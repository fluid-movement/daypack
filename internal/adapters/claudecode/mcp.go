package claudecode

import (
	"encoding/json"
	"fmt"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
)

// mcpItems renders references verbatim. Credential resolution and ownership
// (including tracking and approval adoption) belong to core.
func mcpItems(e contract.Entry) ([]contract.Item, error) {
	if err := library.ValidateName(library.MCP, e.Artifact.Name); err != nil {
		return nil, fmt.Errorf("invalid MCP name")
	}
	if e.Ref != "mcp:"+e.Artifact.Name {
		return nil, fmt.Errorf("invalid MCP reference")
	}
	if e.Artifact.MCP == nil {
		return nil, fmt.Errorf("missing MCP definition")
	}
	d := e.Artifact.MCP
	for _, p := range library.ValidateMCP(*d, "") {
		if p.Severity == library.Error {
			// Even field names may contain credentials in bypassed inputs.
			return nil, fmt.Errorf("invalid MCP definition")
		}
	}
	var payload any
	if d.Type == "stdio" {
		payload = struct {
			Command string            `json:"command"`
			Args    []string          `json:"args,omitempty"`
			Env     map[string]string `json:"env,omitempty"`
		}{d.Command, d.Args, d.Env}
	} else {
		payload = struct {
			Type    string            `json:"type"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers,omitempty"`
		}{"http", d.URL, d.Headers}
	}
	value, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("cannot encode MCP definition")
	}
	name, err := json.Marshal(e.Artifact.Name)
	if err != nil {
		return nil, fmt.Errorf("cannot encode MCP name")
	}
	return []contract.Item{
		{Target: contract.ClaudeMCPJSON, Entry: e.Ref, Kind: contract.JSONEntry, Path: ".mcp.json", Key: "mcpServers." + e.Artifact.Name, Value: value},
		{Target: contract.ClaudeMCPApproval, Entry: e.Ref, Kind: contract.JSONArrayElement, Path: ".claude/settings.local.json", Key: "enabledMcpjsonServers", Value: name},
	}, nil
}
