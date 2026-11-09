package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fluid-movement/daypack/internal/library"
)

type scanner struct {
	ctx        context.Context
	opts       Options
	candidates []candidate
}

func (s *scanner) invalid(source Source, kind library.ArtifactType, name, reason string) {
	if name == "" {
		name = "source"
	}
	s.candidates = append(s.candidates, candidate{artifact: library.Artifact{Type: kind, Name: name}, sources: []Source{source}, reason: reason})
}
func (s *scanner) native(path, harness string, kind library.ArtifactType) {
	source := Source{Harness: harness, Path: path}
	c, err := readNative(path, kind, s.opts)
	if err != nil {
		s.invalid(source, kind, filepath.Base(path), err.Error())
		return
	}
	c.sources = []Source{source}
	if harness == "pi" && kind == library.Skill {
		c.sources = append(c.sources, Source{Harness: "codex", Path: path})
	}
	s.candidates = append(s.candidates, c)
}
func (s *scanner) skills(root, harness string, recursive bool, seen map[string]bool) {
	if s.ctx.Err() != nil {
		return
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		if !os.IsNotExist(err) {
			s.invalid(Source{Harness: harness, Path: root}, library.Skill, "source", "cannot inspect skill directory")
		}
		return
	}
	if excluded(s.opts, real) || within(canonical(s.opts.LibraryDir), real) {
		s.invalid(Source{Harness: harness, Path: root}, library.Skill, "source", "plugin, synced, package or library source excluded")
		return
	}
	if seen[real] {
		s.invalid(Source{Harness: harness, Path: root}, library.Skill, "source", "cyclic skill directory")
		return
	}
	seen[real] = true
	defer delete(seen, real)
	entries, err := os.ReadDir(root)
	if err != nil {
		s.invalid(Source{Harness: harness, Path: root}, library.Skill, "source", "cannot read skill directory")
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}
		path := filepath.Join(root, name)
		if harness == "claude-code" && name == "synced" {
			s.invalid(Source{Harness: harness, Path: path}, library.Skill, name, "synced skills excluded")
			continue
		}
		st, err := os.Stat(path)
		if err != nil {
			if e.Type()&os.ModeSymlink != 0 {
				s.invalid(Source{Harness: harness, Path: path}, library.Skill, name, "broken or cyclic skill link")
			}
			continue
		}
		if !st.IsDir() {
			continue
		}
		_, err = os.Lstat(filepath.Join(path, "SKILL.md"))
		if err == nil || !recursive {
			s.native(path, harness, library.Skill)
		} else {
			s.skills(path, harness, true, seen)
		}
	}
}
func (s *scanner) agents(root string, seen map[string]bool) {
	if s.ctx.Err() != nil {
		return
	}
	real, err := filepath.EvalSymlinks(root)
	if os.IsNotExist(err) {
		return
	}
	if err != nil || seen[real] || excluded(s.opts, real) || within(canonical(s.opts.LibraryDir), real) {
		s.invalid(Source{Harness: "claude-code", Path: root}, library.Agent, "source", "invalid or excluded agent directory")
		return
	}
	seen[real] = true
	defer delete(seen, real)
	entries, err := os.ReadDir(root)
	if err != nil {
		s.invalid(Source{Harness: "claude-code", Path: root}, library.Agent, "source", "cannot read agent directory")
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(root, e.Name())
		if e.IsDir() {
			s.agents(path, seen)
		} else if filepath.Ext(path) == ".md" {
			s.native(path, "claude-code", library.Agent)
		}
	}
}
func pointer(part string) string {
	return strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1")
}
func object(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &m) != nil || m == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return m, nil
}
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
func (s *scanner) servers(raw json.RawMessage, source Source) {
	m, err := object(raw)
	if err != nil {
		s.invalid(source, library.MCP, "source", "invalid MCP server map")
		return
	}
	for _, name := range sortedKeys(m) {
		src := source
		src.Key += "/" + pointer(name)
		d, values, lossy, err := prepareMCP(name, m[name], source.Harness)
		c := candidate{artifact: library.Artifact{Type: library.MCP, Name: name, MCP: &d}, sources: []Source{src}, values: values, lossy: lossy}
		if err != nil {
			c.reason = err.Error()
		} else {
			c.identity = semanticMCP(d, values)
		}
		s.candidates = append(s.candidates, c)
	}
}
func (s *scanner) config(path, harness string) {
	src := Source{Harness: harness, Path: path}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		s.invalid(src, library.MCP, "source", "cannot read MCP config")
		return
	}
	m, err := object(data)
	if err != nil {
		s.invalid(src, library.MCP, "source", "invalid JSON config")
		return
	}
	if raw, ok := m["mcpServers"]; ok {
		top := src
		top.Key = "/mcpServers"
		s.servers(raw, top)
	}
	if harness != "claude-code" {
		return
	}
	if raw, ok := m["projects"]; ok {
		projects, e := object(raw)
		if e != nil {
			s.invalid(src, library.MCP, "source", "invalid projects map")
			return
		}
		for _, key := range sortedKeys(projects) {
			project, e := object(projects[key])
			source := src
			source.Key = "/projects/" + pointer(key)
			if e != nil {
				s.invalid(source, library.MCP, "source", "invalid project entry")
				continue
			}
			if raw, ok := project["mcpServers"]; ok {
				source.Key += "/mcpServers"
				s.servers(raw, source)
			}
		}
	}
}
func scan(ctx context.Context, opts Options) ([]candidate, error) {
	s := scanner{ctx: ctx, opts: opts}
	s.skills(filepath.Join(opts.ClaudeConfigDir, "skills"), "claude-code", false, map[string]bool{})
	s.agents(filepath.Join(opts.ClaudeConfigDir, "agents"), map[string]bool{})
	s.config(opts.claudeState, "claude-code")
	s.skills(filepath.Join(opts.HomeDir, ".agents", "skills"), "pi", true, map[string]bool{})
	s.config(filepath.Join(opts.PiAgentDir, "mcp.json"), "pi")
	return s.candidates, ctx.Err()
}
