package library

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func Load(libraryPath string) (*Library, error) {
	var err error
	if libraryPath == "" {
		libraryPath, err = DefaultPath()
	} else {
		libraryPath, err = filepath.Abs(libraryPath)
	}
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(libraryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("library %s does not exist; run daypack library init", libraryPath)
		}
		return nil, fmt.Errorf("read library %s: %w", libraryPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("library %s is not a directory; run daypack library init", libraryPath)
	}
	l := &Library{root: libraryPath}
	entries, err := os.ReadDir(libraryPath)
	if err != nil {
		return nil, fmt.Errorf("read library %s: %w", libraryPath, err)
	}
	dirs := map[string]ArtifactType{"skills": Skill, "mcp": MCP, "fragments": Fragment, "agents": Agent}
	for _, e := range entries {
		if e.Name() == "AGENTS.md" || housekeeping(e.Name()) {
			continue
		}
		path := filepath.Join(libraryPath, e.Name())
		kind, ok := dirs[e.Name()]
		if !ok || !e.IsDir() {
			l.unknown(path)
			continue
		}
		children, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		for _, child := range children {
			if housekeeping(child.Name()) {
				continue
			}
			source := filepath.Join(path, child.Name())
			name := child.Name()
			if kind == Skill {
				if !child.IsDir() {
					l.unknown(source)
					continue
				}
			} else {
				ext := ".md"
				if kind == MCP {
					ext = ".toml"
				}
				if child.IsDir() || filepath.Ext(name) != ext {
					l.unknown(source)
					continue
				}
				name = strings.TrimSuffix(name, ext)
			}
			a := Artifact{Type: kind, Name: name, Path: source}
			if kind == Skill {
				source = filepath.Join(source, "SKILL.md")
			}
			data, err := os.ReadFile(source)
			if err != nil {
				if kind == Skill && os.IsNotExist(err) {
					l.problems = append(l.problems, problem(source, "frontmatter", "SKILL.md is required", ""))
					l.artifacts = append(l.artifacts, a)
					continue
				}
				return nil, fmt.Errorf("read %s: %w", source, err)
			}
			var p []Problem
			if kind == MCP {
				d, problems := ParseMCP(data, source)
				a.MCP = &d
				a.Description = d.Description
				p = problems
			} else {
				a.Description, p = metadata(data, source, kind, name)
			}
			l.problems = append(l.problems, p...)
			l.artifacts = append(l.artifacts, a)
		}
	}
	l.buildIndex()
	return l, nil
}
func housekeeping(name string) bool {
	return name == ".git" || name == ".gitignore" || name == "secrets.env"
}
func (l *Library) unknown(path string) {
	l.problems = append(l.problems, Problem{Severity: Warning, File: path, Message: "unknown library entry ignored"})
}
func (l *Library) buildIndex() {
	l.index = map[string]Artifact{}
	l.invalid = map[string]bool{}
	sort.SliceStable(l.artifacts, func(i, j int) bool {
		a, b := l.artifacts[i], l.artifacts[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Path < b.Path
	})
	for _, a := range l.artifacts {
		file := a.Path
		if a.Type == Skill {
			file = filepath.Join(file, "SKILL.md")
		}
		ref := string(a.Type) + ":" + a.Name
		if !namePattern.MatchString(a.Name) {
			l.problems = append(l.problems, problem(file, "name", "name must contain 1-64 lowercase letters, digits or hyphens", ""))
		}
		if previous, ok := l.index[ref]; ok {
			l.problems = append(l.problems, problem(file, "name", "duplicate name within artifact type", ""))
			prevFile := previous.Path
			if previous.Type == Skill {
				prevFile = filepath.Join(prevFile, "SKILL.md")
			}
			l.problems = append(l.problems, problem(prevFile, "name", "duplicate name within artifact type", ""))
			l.invalid[ref] = true
		} else {
			l.index[ref] = a
		}
		for _, p := range l.problems {
			if p.File == file && p.Severity == Error {
				l.invalid[ref] = true
			}
		}
	}
	sortProblems(l.problems)
}
