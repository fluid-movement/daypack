package claudecode_test

import (
	"reflect"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters/claudecode"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/instructions"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/library"
)

func fragment(name, body string) contract.Entry {
	return contract.Entry{Ref: "fragment:" + name, Artifact: library.Artifact{Type: library.Fragment, Name: name}, Harnesses: []string{"claude-code"}, Content: []byte(body)}
}

func TestFragmentRendering(t *testing.T) {
	a := claudecode.New()
	p := contract.Project{Root: "/nonexistent/project", Harnesses: []string{"claude-code"}}
	if !a.Supports(library.Fragment) {
		t.Fatal("fragments unsupported")
	}
	testadapter.CheckAdapter(t, a, []testadapter.Case{{Entry: fragment("conventions", "---\ndescription: x\n---\nBody.\n"), Project: p, Supported: true}})
	items, err := a.Desired(fragment("conventions", "---\ndescription: x\n---\nBody.\n"), p)
	if err != nil {
		t.Fatal(err)
	}
	want := []contract.Item{{Target: contract.AgentsMD, Entry: "fragment:conventions", Kind: contract.MarkdownSection, Path: "AGENTS.md", Marker: "fragment:conventions", Bytes: []byte("\nBody.\n")}}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("got %#v", items)
	}
	if _, err := a.Desired(fragment("conventions", "x <!-- daypack:end y -->"), p); err == nil {
		t.Fatal("accepted embedded marker")
	}
}

func TestSharedAgentsTargetMatchesInstructionsPackage(t *testing.T) {
	var found bool
	for _, target := range claudecode.New().Targets() {
		if target.ID == contract.AgentsMD {
			found = reflect.DeepEqual(target, instructions.Target())
		}
	}
	if !found {
		t.Fatal("Claude must declare the shared AGENTS.md target identically")
	}
}

func TestBridgeItem(t *testing.T) {
	a := claudecode.New()
	p := contract.Project{Harnesses: []string{"claude-code"}}
	want := []contract.Item{{Target: contract.ClaudeMD, Entry: contract.BridgeEntry, Kind: contract.MarkdownSection, Path: "CLAUDE.md", Marker: contract.BridgeMarker, Bytes: []byte("\n@AGENTS.md\n")}}
	entries := []contract.Entry{fragment("b", "B"), fragment("a", "A")}
	before := []contract.Entry{contract.CloneEntry(entries[0]), contract.CloneEntry(entries[1])}
	got, err := a.DesiredProject(entries, p)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
	if !reflect.DeepEqual(entries, before) {
		t.Fatal("entries mutated")
	}
	// Reordering never changes the one bridge.
	swapped, err := a.DesiredProject([]contract.Entry{entries[1], entries[0]}, p)
	if err != nil || !reflect.DeepEqual(swapped, want) {
		t.Fatal("bridge depends on fragment order")
	}
	for name, in := range map[string][]contract.Entry{"none": nil, "skill only": {skill("testing")}} {
		if got, err := a.DesiredProject(in, p); err != nil || len(got) != 0 {
			t.Fatalf("%s: %v %v", name, got, err)
		}
	}
}
