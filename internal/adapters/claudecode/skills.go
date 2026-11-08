// Package claudecode renders project-scoped Claude Code configuration.
package claudecode

import (
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/skilldir"
	"github.com/fluid-movement/daypack/internal/library"
)

type Adapter struct{}

var _ contract.Adapter = (*Adapter)(nil)

func New() *Adapter { return &Adapter{} }

func (*Adapter) Name() string { return "claude-code" }

func (*Adapter) Supports(t contract.ArtifactType) bool {
	return t == library.Skill || t == library.Agent || t == library.MCP
}

func (*Adapter) Targets() []contract.Target {
	return []contract.Target{
		{ID: contract.ClaudeSkillsDir, Root: ".claude/skills", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Skill}},
		{ID: contract.ClaudeAgentsDir, Root: ".claude/agents", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Agent}},
		{ID: contract.ClaudeMCPJSON, Root: ".mcp.json", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.MCP}},
		{ID: contract.ClaudeMCPApproval, Root: ".claude/settings.local.json", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.MCP}},
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
	return skilldir.Desired(e, a.Targets()[0])
}

func (*Adapter) Notes(contract.Project) []contract.Note { return nil }
