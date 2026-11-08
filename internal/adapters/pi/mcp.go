// Package pi renders project-scoped Pi configuration without resolving secrets.
package pi

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/instructions"
	"github.com/fluid-movement/daypack/internal/adapters/skilldir"
	"github.com/fluid-movement/daypack/internal/library"
)

type Adapter struct{}

var _ contract.Adapter = (*Adapter)(nil)

func New() *Adapter           { return &Adapter{} }
func (*Adapter) Name() string { return "pi" }
func (*Adapter) Supports(t contract.ArtifactType) bool {
	return t == library.Skill || t == library.MCP || t == library.Fragment
}
func (*Adapter) Targets() []contract.Target {
	return []contract.Target{
		skilldir.AgentsTarget(),
		instructions.Target(),
		{ID: contract.PiMCPJSON, Root: ".pi/mcp.json", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.MCP}},
	}
}
func (a *Adapter) Desired(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
	switch e.Artifact.Type {
	case library.Skill:
		return skilldir.Desired(e, skilldir.AgentsTarget())
	case library.MCP:
		return mcpItems(e)
	case library.Fragment:
		item, err := instructions.FragmentItem(e)
		if err != nil {
			return nil, err
		}
		return []contract.Item{item}, nil
	}
	return nil, nil
}
func (*Adapter) Notes(p contract.Project) []contract.Note {
	if !slices.Contains(p.Harnesses, "pi") {
		return nil
	}
	return []contract.Note{{Harness: "pi", Code: "project-trust-required", Message: trustMessage}}
}

const trustMessage = "Pi loads project .agents/skills and .pi/mcp.json only after project trust is granted (/trust); untrusted project config may be silently skipped in non-interactive (headless) mode."

// Transport-specific structs keep unrelated fields and policy metadata out.
type stdioServer struct {
	Type        string            `json:"type"`
	Command     string            `json:"command"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Description string            `json:"description"`
}
type httpServer struct {
	Type        string            `json:"type"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers,omitempty"`
	Description string            `json:"description"`
}

func mcpItems(e contract.Entry) ([]contract.Item, error) {
	if library.ValidateName(library.MCP, e.Artifact.Name) != nil || e.Ref != "mcp:"+e.Artifact.Name || e.Artifact.MCP == nil {
		return nil, fmt.Errorf("invalid MCP entry")
	}
	d := *e.Artifact.MCP
	for _, p := range library.ValidateMCP(d, "") {
		if p.Severity == library.Error {
			return nil, fmt.Errorf("invalid MCP definition")
		}
	}
	if fields := unsupportedMCPFields(d); len(fields) > 0 {
		return nil, &contract.UnsupportedError{Fields: fields}
	}
	var value any
	switch d.Type {
	case "stdio":
		value = stdioServer{d.Type, d.Command, d.Args, d.Env, d.Description}
	case "http":
		value = httpServer{d.Type, d.URL, d.Headers, d.Description}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("cannot encode MCP definition")
	}
	return []contract.Item{{Target: contract.PiMCPJSON, Entry: e.Ref, Kind: contract.JSONEntry, Path: ".pi/mcp.json", Key: "mcpServers." + e.Artifact.Name, Value: data}}, nil
}

func unsupportedMCPFields(d library.MCPDefinition) []string {
	var fields []string
	check := func(field, value string, anyReference bool) {
		if unsupportedReference(value, anyReference) {
			fields = append(fields, field)
		}
	}
	check("command", d.Command, true)
	check("url", d.URL, true)
	for i, v := range d.Args {
		check(fmt.Sprintf("args[%d]", i), v, true)
	}
	for k, v := range d.Env {
		check("env."+k, v, false)
	}
	for k, v := range d.Headers {
		check("headers."+k, v, false)
	}
	slices.Sort(fields)
	return slices.Compact(fields)
}

// Pi treats $$ as a literal dollar. Scan reference starts without expansion,
// distinguishing ordinary dollar punctuation from bare variable names.
func unsupportedReference(value string, anyReference bool) bool {
	for i := 0; i < len(value); i++ {
		if value[i] != '$' {
			continue
		}
		if i+1 >= len(value) {
			continue
		}
		next := value[i+1]
		if next == '$' {
			i++
			continue
		}
		if next == '{' {
			end := strings.IndexByte(value[i+2:], '}')
			if end < 0 {
				continue
			} // Schema validation already rejects malformed references.
			body := value[i+2 : i+2+end]
			if anyReference || strings.Contains(body, ":-") {
				return true
			}
			i += end + 2
		} else if anyReference && (next == '_' || next >= 'A' && next <= 'Z' || next >= 'a' && next <= 'z') {
			return true
		}
	}
	return false
}
