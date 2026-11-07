// Package testadapter supplies memory-only contract fixtures, not production
// harness implementations or registrations.
package testadapter

import (
	"path"
	"slices"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
)

type Adapter struct {
	Harness      string
	Declarations []contract.Target
	Render       func(contract.Entry, contract.Project) ([]contract.Item, error)
}

func New(name string, unique bool) *Adapter {
	targets := []contract.Target{{ID: contract.AgentsSkillsDir, Root: ".agents/skills", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Skill}}}
	if unique {
		targets = append(targets, contract.Target{ID: name + "-skills", Root: "." + name + "/skills", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Skill}})
	}
	return &Adapter{Harness: name, Declarations: targets}
}
func (a *Adapter) Name() string { return a.Harness }
func (a *Adapter) Supports(t contract.ArtifactType) bool {
	for _, target := range a.Declarations {
		if slices.Contains(target.Types, t) {
			return true
		}
	}
	return false
}
func (a *Adapter) Targets() []contract.Target {
	out := slices.Clone(a.Declarations)
	for i := range out {
		out[i].Types = slices.Clone(out[i].Types)
	}
	return out
}
func (a *Adapter) Desired(e contract.Entry, p contract.Project) ([]contract.Item, error) {
	if a.Render != nil {
		return a.Render(e, p)
	}
	var items []contract.Item
	for _, target := range a.Declarations {
		if slices.Contains(target.Types, e.Artifact.Type) {
			items = append(items, contract.Item{Target: target.ID, Entry: e.Ref, Kind: contract.Symlink, Path: path.Join(target.Root, e.Artifact.Name), Link: e.Artifact.Path})
		}
	}
	return items, nil
}
func (a *Adapter) Notes(contract.Project) []contract.Note {
	return []contract.Note{{Harness: a.Harness, Code: "fixture", Message: "Memory-only conformance fixture"}}
}
