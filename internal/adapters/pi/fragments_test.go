package pi_test

import (
	"reflect"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/instructions"
	"github.com/fluid-movement/daypack/internal/adapters/pi"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/library"
)

func fragment(name, body string) contract.Entry {
	return contract.Entry{Ref: "fragment:" + name, Artifact: library.Artifact{Type: library.Fragment, Name: name}, Harnesses: []string{"pi"}, Content: []byte(body)}
}

func TestFragmentRendering(t *testing.T) {
	a := pi.New()
	p := contract.Project{Root: "/nonexistent/project", Harnesses: []string{"pi"}}
	if !a.Supports(library.Fragment) {
		t.Fatal("fragments unsupported")
	}
	testadapter.CheckAdapter(t, a, []testadapter.Case{{Entry: fragment("conventions", "Body.\n"), Project: p, Supported: true}})
	items, err := a.Desired(fragment("conventions", "Body.\n"), p)
	if err != nil {
		t.Fatal(err)
	}
	want := []contract.Item{{Target: contract.AgentsMD, Entry: "fragment:conventions", Kind: contract.MarkdownSection, Path: "AGENTS.md", Marker: "fragment:conventions", Bytes: []byte("\nBody.\n")}}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("got %#v", items)
	}
	if _, err := a.Desired(fragment("conventions", "---\nunterminated"), p); err == nil {
		t.Fatal("accepted malformed frontmatter")
	}
}

func TestSharedAgentsTargetAndNoBridge(t *testing.T) {
	var found bool
	for _, target := range pi.New().Targets() {
		if target.ID == contract.AgentsMD {
			found = reflect.DeepEqual(target, instructions.Target())
		}
		if target.ID == contract.ClaudeMD {
			t.Fatal("Pi reads AGENTS.md directly and must not declare a CLAUDE.md bridge")
		}
	}
	if !found {
		t.Fatal("Pi must declare the shared AGENTS.md target identically")
	}
	if _, ok := any(pi.New()).(contract.ProjectAdapter); ok {
		t.Fatal("Pi has no project-level items")
	}
}
