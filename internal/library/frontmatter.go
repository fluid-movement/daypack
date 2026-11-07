package library

import (
	"bytes"
	"gopkg.in/yaml.v3"
	"strings"
)

func metadata(data []byte, source string, kind ArtifactType, name string) (string, []Problem) {
	p := []Problem{}
	add := func(field, msg string) { p = append(p, problem(source, field, msg, "")) }
	lines := bytes.Split(data, []byte("\n"))
	has := len(lines) > 0 && strings.TrimSpace(string(lines[0])) == "---"
	var meta struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
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
	} else if kind != Fragment {
		add("frontmatter", "YAML frontmatter is required")
	}
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
