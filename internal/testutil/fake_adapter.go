package testutil

// These minimal descriptors stay local until the adapter feature defines its
// production contract. They do not implement reconciliation or restrictions.
type ArtifactType string

const Skill ArtifactType = "skill"

type Entry struct {
	Name, Source string
	Type         ArtifactType
}
type Project struct{ Root string }
type Target struct {
	ID, Path string
	Shared   bool
}
type Item struct{ Target, Entry, Kind, Path, LinkTarget string }
type Note struct{ Message string }
type FakeAdapter struct{}

func (FakeAdapter) Name() string                    { return "test" }
func (FakeAdapter) Supports(kind ArtifactType) bool { return kind == Skill }
func (FakeAdapter) Targets() []Target {
	return []Target{
		{ID: "agents-skills-dir", Path: ".agents/skills", Shared: true},
		{ID: "test-skills-dir", Path: ".test/skills"},
	}
}
func (f FakeAdapter) Desired(entry Entry, _ Project) ([]Item, error) {
	if !f.Supports(entry.Type) {
		return nil, nil
	}
	items := make([]Item, 0, 2)
	for _, target := range f.Targets() {
		items = append(items, Item{Target: target.ID, Entry: entry.Name, Kind: "symlink", Path: target.Path + "/" + entry.Name, LinkTarget: entry.Source})
	}
	return items, nil
}
func (FakeAdapter) Notes(Project) []Note { return nil }
