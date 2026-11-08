// Package claudecode renders project-scoped Claude Code configuration.
package claudecode

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
)

type Adapter struct{}

var _ contract.Adapter = (*Adapter)(nil)

func New() *Adapter { return &Adapter{} }

func (*Adapter) Name() string { return "claude-code" }

func (*Adapter) Supports(t contract.ArtifactType) bool {
	return t == library.Skill || t == library.Agent
}

func (*Adapter) Targets() []contract.Target {
	return []contract.Target{
		{ID: contract.ClaudeSkillsDir, Root: ".claude/skills", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Skill}},
		{ID: contract.ClaudeAgentsDir, Root: ".claude/agents", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Agent}},
	}
}

func (a *Adapter) Desired(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
	if !a.Supports(e.Artifact.Type) {
		return nil, nil
	}
	if e.Artifact.Type == library.Agent {
		return agentItems(e)
	}
	if err := library.ValidateName(library.Skill, e.Artifact.Name); err != nil {
		return nil, fmt.Errorf("invalid skill name")
	}
	if e.Ref != "skill:"+e.Artifact.Name {
		return nil, fmt.Errorf("invalid skill reference")
	}
	if !filepath.IsAbs(e.Artifact.Path) || strings.ContainsAny(e.Artifact.Path, "\x00\r\n") {
		return nil, fmt.Errorf("invalid skill source path")
	}
	return []contract.Item{{Target: contract.ClaudeSkillsDir, Entry: e.Ref, Kind: contract.Symlink, Path: path.Join(".claude/skills", e.Artifact.Name), Link: e.Artifact.Path}}, nil
}

func (*Adapter) Notes(contract.Project) []contract.Note { return nil }
