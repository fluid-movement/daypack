package library

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func Rename(root string, kind ArtifactType, oldName, newName string) (_ Artifact, err error) {
	old, err := nativePath(root, kind, oldName)
	if err != nil {
		return Artifact{}, err
	}
	dest, err := nativePath(root, kind, newName)
	if err != nil {
		return Artifact{}, err
	}
	a, err := validateArtifact(old, kind, oldName)
	if err != nil {
		return Artifact{}, err
	}
	if oldName == newName {
		return a, nil
	} // Already named as requested.
	if err = absent(dest); err != nil {
		return Artifact{}, err
	}
	stage, clean, err := stagePath(dest)
	if err != nil {
		return Artifact{}, err
	}
	defer func() { err = errors.Join(err, clean()) }()
	if err = copyNative(old, stage, kind == Skill); err != nil {
		return Artifact{}, err
	}
	if kind == Skill || kind == Agent {
		file := contentPath(stage, kind)
		data, err := os.ReadFile(file)
		if err != nil {
			return Artifact{}, err
		}
		data, err = renameMetadata(data, newName)
		if err != nil {
			return Artifact{}, fmt.Errorf("rename %s: %w", old, err)
		}
		if err = os.WriteFile(file, data, 0644); err != nil {
			return Artifact{}, err
		}
	}
	a, err = validateArtifact(stage, kind, newName)
	if err != nil {
		return Artifact{}, fmt.Errorf("validate rename from %s to %s: %w", old, dest, err)
	}
	if err = publish(stage, dest); err != nil {
		return Artifact{}, err
	}
	if err = os.RemoveAll(old); err != nil {
		return Artifact{}, fmt.Errorf("remove old artifact %s after rename to %s: %w", old, dest, err)
	}
	a.Path = dest
	return a, nil
}

// Change only the name value (or add an explicit override of an inherited name).
// YAML node positions locate the value; the scanner retains properties, comments,
// other fields and the Markdown body without re-encoding the document.
func renameMetadata(data []byte, name string) ([]byte, error) {
	// A plain null decodes to an empty name; a lone dash is YAML syntax.
	if name == "null" || name == "-" {
		name = "'" + name + "'"
	}
	lines := bytes.SplitAfter(data, []byte("\n"))
	end := 1
	for end < len(lines) && strings.TrimSpace(string(lines[end])) != "---" {
		end++
	}
	if end >= len(lines) {
		return nil, fmt.Errorf("invalid frontmatter")
	}
	front := bytes.Join(lines[1:end], nil)
	var doc yaml.Node
	if err := yaml.Unmarshal(front, &doc); err != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("invalid frontmatter")
	}
	mapping := doc.Content[0]
	// Convert YAML's rune-based line/column coordinates into Markdown byte offsets.
	offset := func(node *yaml.Node) int {
		n := 0
		for _, line := range lines[:node.Line] {
			n += len(line)
		}
		return n + len(string([]rune(string(lines[node.Line]))[:node.Column-1]))
	}
	replace := func(start, end int, value string) []byte {
		out := append([]byte(nil), data[:start]...)
		out = append(out, value...)
		return append(out, data[end:]...)
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key, value := mapping.Content[i], mapping.Content[i+1]
		var keyName string
		if key.Decode(&keyName) != nil || keyName != "name" {
			continue
		}
		start := offset(value)
		tagStart, tagEnd := -1, -1
		// Node positions include optional tags and anchors. Keep those properties
		// and any whitespace before the scalar itself.
		for start < len(data) && (data[start] == '!' || data[start] == '&') {
			propertyStart := start
			if data[start] == '!' && start+1 < len(data) && data[start+1] == '<' {
				for start < len(data) && data[start] != '>' {
					start++
				}
				start++
			} else {
				for start < len(data) && !strings.ContainsRune(" \t\r\n,{}[]", rune(data[start])) {
					start++
				}
			}
			// Doctor decodes numeric and boolean scalars into strings too. Their
			// explicit tags must become string tags when changing the identity.
			if data[propertyStart] == '!' && strings.HasPrefix(value.Tag, "!!") && value.Tag != "!!str" {
				tagStart, tagEnd = propertyStart, start
			}
			for start < len(data) {
				if strings.ContainsRune(" \t\r\n", rune(data[start])) {
					start++
				} else if data[start] == '#' {
					for start < len(data) && data[start] != '\n' {
						start++
					}
				} else {
					break
				}
			}
		}
		if start >= len(data) {
			return nil, fmt.Errorf("invalid frontmatter name")
		}
		replaceName := func(start, end int, text string) []byte {
			out := replace(start, end, text)
			if tagStart >= 0 {
				out = append(append(append([]byte(nil), out[:tagStart]...), "!!str"...), out[tagEnd:]...)
			}
			return out
		}
		finish := start
		switch data[start] {
		case '|', '>':
			// Replace the block scalar header and its indented content, keeping
			// the header comment and its original line ending.
			headerEnd := bytes.IndexByte(data[start:], '\n') + start
			if headerEnd < start {
				return nil, fmt.Errorf("invalid block scalar name")
			}
			comment := start
			for comment < headerEnd && !strings.ContainsRune(" \t\r#", rune(data[comment])) {
				comment++
			}
			finish = headerEnd + 1
			indent := -1
			for finish < len(data) {
				next := bytes.IndexByte(data[finish:], '\n')
				if next < 0 {
					next = len(data) - finish
				} else {
					next++
				}
				line := data[finish : finish+next]
				spaces := len(line) - len(bytes.TrimLeft(line, " "))
				if len(bytes.TrimSpace(line)) > 0 {
					if indent < 0 {
						indent = spaces
					}
					if spaces < indent || spaces == 0 {
						break
					}
				}
				finish += next
			}
			return replaceName(start, finish, name+string(data[comment:headerEnd+1])), nil
		case '\'', '"':
			quote := data[start]
			finish++
			for finish < len(data) {
				if data[finish] == quote {
					if quote == '\'' && finish+1 < len(data) && data[finish+1] == quote {
						finish += 2
						continue
					}
					text := strings.Trim(name, "'")
					return replaceName(start, finish+1, string(quote)+text+string(quote)), nil
				}
				if quote == '"' && data[finish] == '\\' {
					finish++
				}
				finish++
			}
			return nil, fmt.Errorf("invalid quoted name")
		default:
			// A validated plain name has no whitespace; aliases have the same
			// token boundaries. Replacing an alias leaves its source untouched.
			for finish < len(data) && !strings.ContainsRune(" \t\r\n,}]", rune(data[finish])) {
				finish++
			}
			return replaceName(start, finish, name), nil
		}
	}
	// Doctor also accepts names supplied by YAML merge keys. An explicit name
	// overrides the inherited value without changing the merged mapping.
	if len(mapping.Content) > 0 {
		start := offset(mapping.Content[0])
		if mapping.Style&yaml.FlowStyle != 0 {
			return replace(start, start, "name: "+name+", "), nil
		}
		newline := "\n"
		if bytes.Contains(front, []byte("\r\n")) {
			newline = "\r\n"
		}
		return replace(start, start, "name: "+name+newline+strings.Repeat(" ", mapping.Content[0].Column-1)), nil
	}
	return nil, fmt.Errorf("frontmatter name is required")
}
