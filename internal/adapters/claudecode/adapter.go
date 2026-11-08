// Package claudecode renders project-scoped Claude Code configuration.
package claudecode

import (
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
)

type Adapter struct{}

var _ contract.Adapter = (*Adapter)(nil)

func New() *Adapter                                    { return &Adapter{} }
func (*Adapter) Name() string                          { return "claude-code" }
func (*Adapter) Supports(t contract.ArtifactType) bool { return t == library.Agent }
func (*Adapter) Targets() []contract.Target {
	return []contract.Target{{ID: contract.ClaudeAgentsDir, Root: ".claude/agents", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Agent}}}
}
func (*Adapter) Desired(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
	switch e.Artifact.Type {
	case library.Agent:
		return agentItems(e)
	default:
		return nil, nil
	}
}
func (*Adapter) Notes(contract.Project) []contract.Note { return nil }
