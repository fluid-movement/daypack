package adapters_test

import (
	"reflect"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
)

func fragment(name, body string, harnesses ...string) contract.Entry {
	return contract.Entry{
		Ref:       "fragment:" + name,
		Artifact:  library.Artifact{Type: library.Fragment, Name: name},
		Harnesses: harnesses,
		Content:   []byte(body),
	}
}

func defaultRegistry(t *testing.T) *adapters.Registry {
	t.Helper()
	r, err := adapters.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func itemsFor(items []contract.Item, target string) []contract.Item {
	var out []contract.Item
	for _, i := range items {
		if i.Target == target {
			out = append(out, i)
		}
	}
	return out
}

func TestFragmentsDeduplicateSharedAgentsTarget(t *testing.T) {
	r := defaultRegistry(t)
	entries := []contract.Entry{fragment("zeta", "Z\n"), fragment("alpha", "A\n")}
	got := desired(t, r, entries, "claude-code", "pi")
	sections := itemsFor(got.Items, contract.AgentsMD)
	if len(sections) != 2 {
		t.Fatalf("Claude and Pi must share one item per entry: %+v", sections)
	}
	// Manifest order is preserved, not lexical order.
	if sections[0].Entry != "fragment:zeta" || sections[1].Entry != "fragment:alpha" {
		t.Fatalf("order: %+v", sections)
	}
	if sections[0].Marker != "fragment:zeta" || sections[0].Path != "AGENTS.md" || sections[0].Kind != contract.MarkdownSection || string(sections[0].Bytes) != "\nZ\n" {
		t.Fatalf("item: %+v", sections[0])
	}
	for _, target := range got.Targets {
		if target.ID == contract.AgentsMD && target.Root != "AGENTS.md" {
			t.Fatal(target)
		}
	}
}

func TestFragmentsRespectEffectiveHarnessRestrictions(t *testing.T) {
	r := defaultRegistry(t)
	both := []string{"claude-code", "pi"}
	got := desired(t, r, []contract.Entry{fragment("a", "A\n", "pi"), fragment("b", "B\n", "claude-code"), fragment("c", "C\n")}, both...)
	if sections := itemsFor(got.Items, contract.AgentsMD); len(sections) != 3 {
		t.Fatalf("every selected fragment lands in AGENTS.md once: %+v", sections)
	}
	bridge := itemsFor(got.Items, contract.ClaudeMD)
	if len(bridge) != 1 {
		t.Fatalf("expected one bridge: %+v", bridge)
	}
	// Pi-only selection or Pi-only restriction never creates a bridge.
	for name, entries := range map[string][]contract.Entry{
		"restricted to pi": {fragment("a", "A\n", "pi")},
		"no fragments":     nil,
		"skill only":       {entry("testing")},
	} {
		got := desired(t, r, entries, both...)
		if len(itemsFor(got.Items, contract.ClaudeMD)) != 0 {
			t.Fatalf("%s: unexpected bridge", name)
		}
	}
	got = desired(t, r, []contract.Entry{fragment("a", "A\n")}, "pi")
	if len(itemsFor(got.Items, contract.ClaudeMD)) != 0 || len(itemsFor(got.Items, contract.AgentsMD)) != 1 {
		t.Fatalf("pi-only: %+v", got.Items)
	}
	// A restriction to an unselected harness is rejected, as for other types.
	if _, err := r.Desired([]contract.Entry{fragment("a", "A\n", "claude-code")}, contract.Project{Harnesses: []string{"pi"}}); err == nil {
		t.Fatal("accepted restriction to unselected harness")
	}
}

func TestSingleBridgeRegardlessOfFragmentCountOrOrder(t *testing.T) {
	r := defaultRegistry(t)
	one := desired(t, r, []contract.Entry{fragment("a", "A\n")}, "claude-code")
	many := desired(t, r, []contract.Entry{fragment("c", "C\n"), fragment("b", "B\n"), fragment("a", "A\n")}, "claude-code", "pi")
	reordered := desired(t, r, []contract.Entry{fragment("a", "A\n"), fragment("b", "B\n"), fragment("c", "C\n")}, "claude-code", "pi")
	want := contract.Item{Target: contract.ClaudeMD, Entry: contract.BridgeEntry, Kind: contract.MarkdownSection, Path: "CLAUDE.md", Marker: contract.BridgeMarker, Bytes: []byte("\n@AGENTS.md\n")}
	for name, got := range map[string]adapters.Result{"one": one, "many": many, "reordered": reordered} {
		bridge := itemsFor(got.Items, contract.ClaudeMD)
		if len(bridge) != 1 || !reflect.DeepEqual(bridge[0], want) {
			t.Fatalf("%s: %+v", name, bridge)
		}
	}
}

func TestFragmentRenderingFailsWithoutItems(t *testing.T) {
	r := defaultRegistry(t)
	for name, body := range map[string]string{
		"unterminated frontmatter": "---\nbody\n",
		"embedded marker":          "x <!-- daypack:begin fragment:a --> y\n",
	} {
		if got, err := r.Desired([]contract.Entry{fragment("a", body)}, contract.Project{Harnesses: []string{"claude-code", "pi"}}); err == nil || len(got.Items) != 0 {
			t.Fatalf("%s: accepted: %+v", name, got)
		}
	}
}

// A project adapter may only emit the reserved bridge on a declared target.
type bridgeAdapter struct {
	contract.Adapter
	items []contract.Item
}

func (b bridgeAdapter) DesiredProject([]contract.Entry, contract.Project) ([]contract.Item, error) {
	return b.items, nil
}

func TestProjectItemValidation(t *testing.T) {
	valid := contract.Item{Target: contract.ClaudeMD, Entry: contract.BridgeEntry, Kind: contract.MarkdownSection, Path: "CLAUDE.md", Marker: contract.BridgeMarker, Bytes: []byte("\n@AGENTS.md\n")}
	mutate := map[string]func(*contract.Item){
		"wrong target":   func(i *contract.Item) { i.Target = contract.AgentsMD; i.Path = "AGENTS.md" },
		"unknown target": func(i *contract.Item) { i.Target = "other" },
		"wrong ref":      func(i *contract.Item) { i.Entry = "fragment:real" },
		"wrong marker":   func(i *contract.Item) { i.Marker = "other" },
		"wrong path":     func(i *contract.Item) { i.Path = "OTHER.md" },
		"wrong kind":     func(i *contract.Item) { i.Kind = contract.File },
		"injected body":  func(i *contract.Item) { i.Bytes = []byte("<!-- daypack:end x -->") },
		"link":           func(i *contract.Item) { i.Link = "x" },
	}
	claude, _ := defaultRegistry(t).Lookup("claude-code")
	for name, change := range mutate {
		item := valid
		change(&item)
		r := registry(t, bridgeAdapter{Adapter: claude, items: []contract.Item{item}})
		if _, err := r.Desired([]contract.Entry{fragment("a", "A\n")}, contract.Project{Harnesses: []string{"claude-code"}}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	r := registry(t, bridgeAdapter{Adapter: claude, items: []contract.Item{valid}})
	if got, err := r.Desired([]contract.Entry{fragment("a", "A\n")}, contract.Project{Harnesses: []string{"claude-code"}}); err != nil || len(itemsFor(got.Items, contract.ClaudeMD)) != 1 {
		t.Fatalf("valid bridge rejected: %v", err)
	}
	r = registry(t, bridgeAdapter{Adapter: claude, items: []contract.Item{valid, valid}})
	if _, err := r.Desired([]contract.Entry{fragment("a", "A\n")}, contract.Project{Harnesses: []string{"claude-code"}}); err == nil {
		t.Fatal("accepted two bridges")
	}
}

func TestReadersForInstructionTargets(t *testing.T) {
	if !reflect.DeepEqual(adapters.Readers(contract.ClaudeMD), []string{"claude-code"}) {
		t.Fatal(adapters.Readers(contract.ClaudeMD))
	}
	for _, reader := range adapters.Readers(contract.AgentsMD) {
		if reader == "claude-code" {
			t.Fatal("Claude reads AGENTS.md only through the bridge")
		}
	}
}
