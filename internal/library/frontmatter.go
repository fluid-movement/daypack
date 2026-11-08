package library

import (
	"bytes"
	"gopkg.in/yaml.v3"
	"strings"
)

type nativeMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func parseMetadata(data []byte, source string, required bool) (nativeMetadata, []Problem) {
	p := []Problem{}
	add := func(field, msg string) { p = append(p, problem(source, field, msg, "")) }
	lines := bytes.Split(data, []byte("\n"))
	has := len(lines) > 0 && strings.TrimSpace(string(lines[0])) == "---"
	var meta nativeMetadata
	if has {
		end := 1
		for end < len(lines) && strings.TrimSpace(string(lines[end])) != "---" {
			end++
		}
		if end == len(lines) {
			add("frontmatter", "unterminated YAML frontmatter")
		} else if err := yaml.Unmarshal(bytes.Join(lines[1:end], []byte("\n")), &meta); err != nil {
			add("frontmatter", "invalid YAML frontmatter")
		}
	} else if required {
		add("frontmatter", "YAML frontmatter is required")
	}
	return meta, p
}
func metadata(data []byte, source string, kind ArtifactType, name string) (string, []Problem) {
	meta, p := parseMetadata(data, source, kind != Fragment)
	add := func(field, msg string) { p = append(p, problem(source, field, msg, "")) }
	if kind != Fragment {
		if meta.Name != name {
			add("frontmatter.name", "frontmatter name must match the artifact name")
		}
		if strings.TrimSpace(meta.Description) == "" {
			add("frontmatter.description", "non-empty description is required")
		}
	}
	return meta.Description, p
}
