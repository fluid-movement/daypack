// Package codex renders native project configuration without resolving credentials.
package codex

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/instructions"
	"github.com/fluid-movement/daypack/internal/adapters/skilldir"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/secrets"
	"gopkg.in/yaml.v3"
)

type Adapter struct{}

var _ contract.Adapter = (*Adapter)(nil)

func New() *Adapter           { return &Adapter{} }
func (*Adapter) Name() string { return "codex" }
func (*Adapter) Supports(t contract.ArtifactType) bool {
	return t == library.Skill || t == library.MCP || t == library.Fragment || t == library.Agent
}
func (*Adapter) Targets() []contract.Target {
	return []contract.Target{skilldir.AgentsTarget(), instructions.Target(),
		{ID: contract.CodexConfigTOML, Root: ".codex/config.toml", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.MCP}},
		{ID: contract.CodexAgentsDir, Root: ".codex/agents", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Agent}},
	}
}
func (*Adapter) Desired(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
	switch e.Artifact.Type {
	case library.Skill:
		return skilldir.Desired(e, skilldir.AgentsTarget())
	case library.Fragment:
		item, err := instructions.FragmentItem(e)
		if err != nil {
			return nil, err
		}
		return []contract.Item{item}, nil
	case library.MCP:
		return mcpItems(e)
	case library.Agent:
		return agentItems(e)
	}
	return nil, nil
}
func (*Adapter) Notes(p contract.Project) []contract.Note {
	if !slices.Contains(p.Harnesses, "codex") {
		return nil
	}
	return []contract.Note{{Harness: "codex", Code: "project-trust-required", Message: "Codex loads project .codex/config.toml only for trusted projects; untrusted projects ignore project config. Grant trust in Codex; daypack does not change trust settings."}}
}
func encode(value any) ([]byte, error) {
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(value); err != nil {
		return nil, fmt.Errorf("cannot encode Codex configuration")
	}
	return b.Bytes(), nil
}

// reference classifies native env-name mappings without inspecting the process
// environment. Everything reference-like outside the exact grammar is rejected.
func reference(value string) (name string, literal bool, supported bool) {
	refs, err := secrets.ParseReferences(value)
	if err != nil {
		return "", false, false
	}
	if len(refs) == 1 && refs[0].Start == 0 && refs[0].End == len(value) && refs[0].Default == nil {
		return refs[0].Name, false, true
	}
	if len(refs) > 0 {
		return "", false, false
	}
	for i := 0; i+1 < len(value); i++ {
		c := value[i+1]
		if value[i] == '$' && (c == '{' || c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return "", false, false
		}
	}
	return "", true, true
}
func mcpItems(e contract.Entry) ([]contract.Item, error) {
	if library.ValidateName(library.MCP, e.Artifact.Name) != nil || e.Ref != "mcp:"+e.Artifact.Name || e.Artifact.MCP == nil {
		return nil, fmt.Errorf("invalid MCP entry")
	}
	d := *e.Artifact.MCP
	invalid := false
	for _, p := range library.ValidateMCP(d, "") {
		invalid = invalid || p.Severity == library.Error
	}
	var fields []string
	static := func(field, value string) {
		_, literal, ok := reference(value)
		if !ok || !literal {
			fields = append(fields, field)
		}
	}
	static("command", d.Command)
	static("url", d.URL)
	for n, v := range d.Args {
		static(fmt.Sprintf("args[%d]", n), v)
	}
	value := map[string]any{}
	if d.Type == "stdio" {
		value["command"] = d.Command
		if len(d.Args) > 0 {
			value["args"] = d.Args
		}
		env := map[string]string{}
		var vars []string
		for k, v := range d.Env {
			name, literal, ok := reference(v)
			switch {
			case !ok || !literal && name != k:
				fields = append(fields, "env."+k)
			case literal:
				env[k] = v
			default:
				vars = append(vars, name)
			}
		}
		slices.Sort(vars)
		if len(env) > 0 {
			value["env"] = env
		}
		if len(vars) > 0 {
			value["env_vars"] = vars
		}
	} else {
		value["url"] = d.URL
		headers, envHeaders := map[string]string{}, map[string]string{}
		auth := []string{}
		for k := range d.Headers {
			if strings.EqualFold(k, "Authorization") {
				auth = append(auth, k)
			}
		}
		if len(auth) > 1 {
			for _, k := range auth {
				fields = append(fields, "headers."+k)
			}
		}
		for k, v := range d.Headers {
			isBearer := strings.EqualFold(k, "Authorization") && strings.HasPrefix(v, "Bearer ")
			raw := v
			if isBearer {
				raw = strings.TrimPrefix(v, "Bearer ")
			}
			name, literal, ok := reference(raw)
			switch {
			case !ok:
				fields = append(fields, "headers."+k)
			case literal:
				headers[k] = v
			case isBearer:
				value["bearer_token_env_var"] = name
			default:
				envHeaders[k] = name
			}
		}
		if len(headers) > 0 {
			value["http_headers"] = headers
		}
		if len(envHeaders) > 0 {
			value["env_http_headers"] = envHeaders
		}
	}
	if len(fields) > 0 {
		slices.Sort(fields)
		return nil, &contract.UnsupportedError{Fields: slices.Compact(fields)}
	}
	if invalid {
		return nil, fmt.Errorf("invalid MCP definition")
	}
	data, err := encode(value)
	if err != nil {
		return nil, err
	}
	return []contract.Item{{Target: contract.CodexConfigTOML, Entry: e.Ref, Kind: contract.TOMLTable, Path: ".codex/config.toml", Key: "mcp_servers." + e.Artifact.Name, Value: data}}, nil
}

func agentItems(e contract.Entry) ([]contract.Item, error) {
	if library.ValidateName(library.Agent, e.Artifact.Name) != nil || e.Ref != "agent:"+e.Artifact.Name {
		return nil, fmt.Errorf("invalid agent entry")
	}
	lines := bytes.Split(e.Content, []byte("\n"))
	if len(lines) == 0 || strings.TrimSpace(string(lines[0])) != "---" {
		return nil, fmt.Errorf("agent frontmatter required")
	}
	end := 1
	for end < len(lines) && strings.TrimSpace(string(lines[end])) != "---" {
		end++
	}
	if end == len(lines) {
		return nil, fmt.Errorf("invalid agent frontmatter")
	}
	var meta map[string]any
	if err := yaml.Unmarshal(bytes.Join(lines[1:end], []byte("\n")), &meta); err != nil {
		return nil, fmt.Errorf("invalid agent frontmatter")
	}
	var fields []string
	for k, v := range meta {
		if k == "name" || k == "description" || k == "model" {
			continue
		}
		if nonempty(v) {
			fields = append(fields, "frontmatter."+k)
		}
	}
	if len(fields) > 0 {
		slices.Sort(fields)
		return nil, &contract.UnsupportedError{Fields: fields}
	}
	name, _ := meta["name"].(string)
	description, _ := meta["description"].(string)
	if name != e.Artifact.Name || strings.TrimSpace(description) == "" {
		return nil, fmt.Errorf("invalid agent metadata")
	}
	value := map[string]any{"name": name, "description": description, "developer_instructions": string(bytes.Join(lines[end+1:], []byte("\n")))}
	if model, present := meta["model"]; present && nonempty(model) {
		s, ok := model.(string)
		if !ok {
			return nil, &contract.UnsupportedError{Fields: []string{"frontmatter.model"}}
		}
		value["model"] = s
	}
	data, err := encode(value)
	if err != nil {
		return nil, err
	}
	return []contract.Item{{Target: contract.CodexAgentsDir, Entry: e.Ref, Kind: contract.File, Path: ".codex/agents/" + e.Artifact.Name + ".toml", Bytes: data}}, nil
}
func nonempty(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(x) != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	default:
		return true
	}
}
