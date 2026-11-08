// Package claudecode renders project-scoped Claude Code configuration.
package claudecode

import (
	"slices"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/instructions"
	"github.com/fluid-movement/daypack/internal/adapters/skilldir"
	"github.com/fluid-movement/daypack/internal/library"
)

type Adapter struct{}

var _ contract.Adapter = (*Adapter)(nil)

func New() *Adapter { return &Adapter{} }

func (*Adapter) Name() string { return "claude-code" }

func (*Adapter) Supports(t contract.ArtifactType) bool {
	return t == library.Skill || t == library.Agent || t == library.MCP || t == library.Fragment
}

func (*Adapter) Targets() []contract.Target {
	return []contract.Target{
		{ID: contract.ClaudeSkillsDir, Root: ".claude/skills", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Skill}},
		{ID: contract.ClaudeAgentsDir, Root: ".claude/agents", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Agent}},
		{ID: contract.ClaudeMCPJSON, Root: ".mcp.json", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.MCP}},
		{ID: contract.ClaudeMCPApproval, Root: ".claude/settings.local.json", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.MCP}},
		instructions.Target(),
		{ID: contract.ClaudeMD, Root: "CLAUDE.md", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.Fragment}},
	}
}

func (a *Adapter) Desired(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
	if !a.Supports(e.Artifact.Type) {
		return nil, nil
	}
	if e.Artifact.Type == library.Agent {
		return agentItems(e)
	}
	if e.Artifact.Type == library.MCP {
		return mcpItems(e)
	}
	if e.Artifact.Type == library.Fragment {
		item, err := instructions.FragmentItem(e)
		if err != nil {
			return nil, err
		}
		return []contract.Item{item}, nil
	}
	return skilldir.Desired(e, a.Targets()[0])
}

func (*Adapter) Notes(p contract.Project) []contract.Note {
	if !slices.Contains(p.Harnesses, "claude-code") {
		return nil
	}
	return []contract.Note{{Harness: "claude-code", Code: "mcp-approval", Message: mcpApprovalMessage}}
}

// Static advisory: the adapter cannot observe Claude Code's approval state.
const mcpApprovalMessage = "Claude Code may require approving project MCP servers on first use; approve them via /mcp (or list them in enabledMcpjsonServers in .claude/settings.local.json)."

var _ contract.ProjectAdapter = (*Adapter)(nil)

// DesiredProject adds exactly one owned AGENTS.md import for CLAUDE.md when at
// least one fragment selects Claude Code, because Claude Code reads CLAUDE.md,
// not AGENTS.md. Core decides whether the item becomes an owned stub file or a
// managed section of a handwritten CLAUDE.md.
func (*Adapter) DesiredProject(entries []contract.Entry, _ contract.Project) ([]contract.Item, error) {
	for _, e := range entries {
		if e.Artifact.Type == library.Fragment {
			return []contract.Item{{Target: contract.ClaudeMD, Entry: contract.BridgeEntry, Kind: contract.MarkdownSection, Path: "CLAUDE.md", Marker: contract.BridgeMarker, Bytes: []byte("\n@AGENTS.md\n")}}, nil
		}
	}
	return nil, nil
}
