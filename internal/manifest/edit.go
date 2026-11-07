package manifest

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/fluid-movement/daypack/internal/library"
)

// AddEntry inserts a library reference without re-encoding the document. Only
// single-line arrays with simple root keys are edited; other layouts fail safely.
func AddEntry(data []byte, kind library.ArtifactType, entry Entry) ([]byte, error) {
	return edit(data, kind, entry, true)
}

// RemoveEntry removes a library reference, preserving unrelated source bytes.
func RemoveEntry(data []byte, kind library.ArtifactType, name string) ([]byte, error) {
	return edit(data, kind, Entry{Name: name}, false)
}

type span struct{ start, end int }

func edit(data []byte, kind library.ArtifactType, entry Entry, add bool) ([]byte, error) {
	m, err := parse(data, "daypack.toml")
	if err != nil {
		return nil, err
	}
	key := ""
	var entries []Entry
	groups := [][]Entry{m.Skills, m.MCP, m.Fragments, m.Agents}
	for i, g := range arrayKinds {
		if g.kind == kind {
			key = g.key
			entries = groups[i]
		}
	}
	if key == "" {
		return nil, fmt.Errorf("unsupported artifact type %q", kind)
	}
	if !namePattern.MatchString(entry.Name) {
		return nil, fmt.Errorf("invalid artifact name")
	}
	if entry.Inline != nil || (entry.Type != "" && entry.Type != kind) {
		return nil, fmt.Errorf("entry must be a %s library reference", kind)
	}
	if add && entry.Harnesses != nil {
		if len(entry.Harnesses) == 0 {
			return nil, fmt.Errorf("entry harness selection must not be empty")
		}
		seen := map[string]bool{}
		for _, h := range entry.Harnesses {
			selected := false
			for _, s := range m.Harnesses {
				if h == s {
					selected = true
				}
			}
			if (h != "claude-code" && h != "pi") || !selected || seen[h] {
				return nil, fmt.Errorf("invalid harness selection for %s:%s", kind, entry.Name)
			}
			seen[h] = true
		}
	}
	rendered := strconv.Quote(entry.Name)
	if entry.Harnesses != nil {
		h := make([]string, len(entry.Harnesses))
		for i, s := range entry.Harnesses {
			h[i] = strconv.Quote(s)
		}
		rendered = "{ name = " + rendered + ", harnesses = [" + strings.Join(h, ", ") + "] }"
	}
	action := "remove"
	if add {
		action = "add"
	}
	manual := func() ([]byte, error) {
		return nil, fmt.Errorf("edit daypack.toml by hand: %s %s in %s = [...]", action, rendered, key)
	}
	index := -1
	for i, e := range entries {
		if e.Name == entry.Name {
			if index != -1 {
				return manual()
			}
			index = i
		}
	}
	if add && index != -1 {
		return nil, fmt.Errorf("%s:%s is already present", kind, entry.Name)
	}
	if !add && index == -1 {
		return bytes.Clone(data), nil
	}
	pattern := regexp.MustCompile(`^[ \t]*` + key + `[ \t]*=[ \t]*\[`)
	offset := 0
	open := -1
	insertion := len(data)
	for _, line := range bytes.SplitAfter(data, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("[")) {
			insertion = offset
			break
		}
		if loc := pattern.FindIndex(line); loc != nil {
			open = offset + loc[1] - 1
			break
		}
		offset += len(line)
	}
	var result []byte
	if open == -1 {
		if len(entries) > 0 {
			return manual()
		}
		// A quoted or otherwise noncanonical empty array must not be duplicated.
		var raw map[string]any
		if _, err := toml.Decode(string(data), &raw); err != nil {
			return nil, err
		}
		if _, exists := raw[key]; exists {
			return manual()
		}
		newline := "\n"
		if bytes.Contains(data, []byte("\r\n")) {
			newline = "\r\n"
		}
		line := key + " = [" + rendered + "]" + newline
		if insertion > 0 && data[insertion-1] != '\n' {
			line = newline + line
		}
		result = splice(data, insertion, insertion, []byte(line))
	} else {
		elements, close, ok := arraySpans(data, open)
		if !ok || len(elements) != len(entries) {
			return manual()
		}
		if add {
			pos := open + 1
			text := rendered
			if len(elements) > 0 {
				pos = elements[len(elements)-1].end
				text = ", " + text
			}
			result = splice(data, pos, pos, []byte(text))
		} else {
			start, end := elements[index].start, elements[index].end
			if index > 0 {
				start = elements[index-1].end
			} else if len(elements) > 1 {
				end = elements[1].start
			} else {
				for i := end; i < close; i++ {
					if data[i] == ',' {
						end = i + 1
						break
					}
				}
			}
			result = splice(data, start, end, nil)
		}
	}
	if _, err := parse(result, "daypack.toml"); err != nil {
		return manual()
	}
	return result, nil
}

func splice(data []byte, start, end int, replacement []byte) []byte {
	out := make([]byte, 0, len(data)+len(replacement)-(end-start))
	out = append(out, data[:start]...)
	out = append(out, replacement...)
	return append(out, data[end:]...)
}

// arraySpans recognizes one-line arrays, tracking quoted strings and nested
// inline tables/arrays so commas inside targeting tables are never separators.
func arraySpans(data []byte, open int) ([]span, int, bool) {
	var elements []span
	start := -1
	last := -1
	depth := 0
	var quote byte
	for i := open + 1; i < len(data); i++ {
		c := data[i]
		if c == '\n' || c == '\r' {
			return nil, 0, false
		}
		if quote != 0 {
			if c == '\\' && quote == '"' {
				i++
				if i >= len(data) || data[i] == '\n' || data[i] == '\r' {
					return nil, 0, false
				}
			} else if c == quote {
				quote = 0
			}
			last = i + 1
			continue
		}
		if c == '#' {
			return nil, 0, false
		}
		if depth == 0 && (c == ',' || c == ']') {
			if start != -1 {
				elements = append(elements, span{start, last})
				start = -1
			}
			if c == ']' {
				return elements, i, true
			}
			continue
		}
		if c == ' ' || c == '\t' {
			continue
		}
		if start == -1 {
			start = i
		}
		if c == '"' || c == '\'' {
			if i+2 < len(data) && data[i+1] == c && data[i+2] == c {
				return nil, 0, false
			}
			quote = c
		}
		if c == '[' || c == '{' {
			depth++
		}
		if c == ']' || c == '}' {
			depth--
		}
		last = i + 1
	}
	return nil, 0, false
}
