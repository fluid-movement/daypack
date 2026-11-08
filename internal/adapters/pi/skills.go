// Package pi renders project-scoped Pi configuration.
package pi

import (
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/skilldir"
	"github.com/fluid-movement/daypack/internal/library"
)

type Adapter struct{}

var _ contract.Adapter = (*Adapter)(nil)

func New() *Adapter                                    { return &Adapter{} }
func (*Adapter) Name() string                          { return "pi" }
func (*Adapter) Supports(t contract.ArtifactType) bool { return t == library.Skill }
func (*Adapter) Targets() []contract.Target            { return []contract.Target{skilldir.AgentsTarget()} }
func (*Adapter) Desired(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
	return skilldir.Desired(e, skilldir.AgentsTarget())
}
func (*Adapter) Notes(contract.Project) []contract.Note {
	return []contract.Note{{Harness: "pi", Code: "project-trust", Message: "Pi loads project .agents/skills only after project trust is granted with /trust; skills may be silently skipped in headless use."}}
}
