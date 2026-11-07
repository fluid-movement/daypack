package testutil

import (
	"reflect"
	"testing"
)

func TestFakeAdapterDeclarations(t *testing.T) {
	fake := FakeAdapter{}
	want := []Target{{ID: "agents-skills-dir", Path: ".agents/skills", Shared: true}, {ID: "test-skills-dir", Path: ".test/skills"}}
	if !reflect.DeepEqual(fake.Targets(), want) {
		t.Fatal("incorrect target declarations")
	}
	if fake.Name() != "test" || !fake.Supports(Skill) || fake.Supports("mcp") {
		t.Fatal("incorrect capabilities")
	}
	entry := Entry{Name: "testing", Source: "/synthetic/library/skills/testing", Type: Skill}
	first, err := fake.Desired(entry, Project{Root: "/does-not-exist"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := fake.Desired(entry, Project{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || len(first) != 2 {
		t.Fatal("non-deterministic desired items")
	}
	for i, item := range first {
		if item.Target != want[i].ID || item.Path != want[i].Path+"/testing" || item.Entry != "testing" || item.Kind != "symlink" || item.LinkTarget != entry.Source {
			t.Fatalf("unexpected item: %+v", item)
		}
	}
	unsupported, err := fake.Desired(Entry{Type: "mcp"}, Project{})
	if err != nil || len(unsupported) != 0 {
		t.Fatal("unsupported artifact emitted")
	}
	if len(fake.Notes(Project{})) != 0 {
		t.Fatal("unexpected notes")
	}
}
