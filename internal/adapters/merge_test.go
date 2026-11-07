package adapters_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func entry(name string) contract.Entry {
	return contract.Entry{Ref: "skill:" + name, Artifact: library.Artifact{Type: library.Skill, Name: name, Path: "/library/skills/" + name}}
}
func registry(t *testing.T, aa ...contract.Adapter) *adapters.Registry {
	t.Helper()
	r, err := adapters.NewRegistry(aa...)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func desired(t *testing.T, r *adapters.Registry, entries []contract.Entry, names ...string) adapters.Result {
	t.Helper()
	got, err := r.Desired(entries, contract.Project{Harnesses: names})
	if err != nil {
		t.Fatal(err)
	}
	return got
}
func TestSharedTargetOnce(t *testing.T) {
	r := registry(t, testadapter.New("base", false), testadapter.New("pi", false))
	got := desired(t, r, []contract.Entry{entry("svelte")}, "pi", "base")
	if len(got.Items) != 1 || len(got.Targets) != 1 {
		t.Fatalf("not deduplicated: %+v", got)
	}
	b, err := json.Marshal(got.Items)
	if err != nil {
		t.Fatal(err)
	}
	testutil.AssertJSONGolden(t, "testdata/shared-items.json", b)
}
func TestThirdAdapterAndRemoval(t *testing.T) {
	// Adding a package implementing contract plus this single list entry is the
	// entire registration change. No merge branches know these harness names.
	r := registry(t, testadapter.New("base", false), testadapter.New("pi", false), testadapter.New("third", true))
	for _, tc := range []struct {
		names []string
		count int
	}{{[]string{"base", "pi", "third"}, 2}, {[]string{"pi", "third"}, 2}, {[]string{"pi"}, 1}, {nil, 0}} {
		got := desired(t, r, []contract.Entry{entry("svelte")}, tc.names...)
		if len(got.Items) != tc.count {
			t.Fatalf("%v: %d items", tc.names, len(got.Items))
		}
	}
}
func TestRestrictedEntries(t *testing.T) {
	a, b := testadapter.New("base", true), testadapter.New("pi", false)
	r := registry(t, a, b)
	e := entry("svelte")
	e.Harnesses = []string{"pi"}
	got := desired(t, r, []contract.Entry{e}, "base", "pi")
	if len(got.Items) != 1 || got.Items[0].Target != contract.AgentsSkillsDir {
		t.Fatal(got)
	}
	e.Harnesses = []string{"base"}
	if len(desired(t, r, []contract.Entry{e}, "base", "pi").Items) != 2 {
		t.Fatal("restriction lost")
	}
	e.Harnesses = []string{"base", "pi"}
	if len(desired(t, r, []contract.Entry{e}, "base", "pi").Items) != 2 {
		t.Fatal("shared union lost")
	}
	for _, names := range [][]string{{}, {"unknown"}} {
		e.Harnesses = names
		if _, err := r.Desired([]contract.Entry{e}, contract.Project{Harnesses: []string{"base", "pi"}}); err == nil {
			t.Fatal("invalid restriction accepted")
		}
	}
}
func TestRestrictedSharedTargetReaders(t *testing.T) {
	r := registry(t, testadapter.New("pi", false))
	e := entry("svelte")
	e.Harnesses = []string{"pi"}
	got := desired(t, r, []contract.Entry{e}, "pi")
	readers := adapters.Readers(got.Items[0].Target)
	for _, name := range []string{"pi", "codex", "opencode"} {
		if !slices.Contains(readers, name) {
			t.Fatal(readers)
		}
	}
	readers[0] = "corrupted"
	if slices.Contains(adapters.Readers(contract.AgentsSkillsDir), "corrupted") {
		t.Fatal("readers alias")
	}
	if _, ok := r.Lookup("codex"); ok {
		t.Fatal("reader registered")
	}
	if _, err := r.Desired(nil, contract.Project{Harnesses: []string{"codex"}}); err == nil {
		t.Fatal("unknown harness accepted")
	}
	a, ok := r.Lookup("pi")
	if !ok || len(a.Notes(contract.Project{})) != 1 {
		t.Fatal("notes unavailable")
	}
}
func TestUnsupported(t *testing.T) {
	a := testadapter.New("pi", false)
	a.Render = func(contract.Entry, contract.Project) ([]contract.Item, error) {
		t.Fatal("unsupported renderer called")
		return nil, nil
	}
	e := entry("reviewer")
	e.Artifact.Type = library.Agent
	e.Ref = "agent:reviewer"
	got := desired(t, registry(t, a), []contract.Entry{e}, "pi")
	if len(got.Items) != 0 || !reflect.DeepEqual(got.Unsupported, []contract.Unsupported{{Harness: "pi", Entry: e.Ref, Type: library.Agent}}) {
		t.Fatal(got)
	}
}
func jsonAdapter(name string, values ...string) *testadapter.Adapter {
	a := testadapter.New(name, false)
	a.Declarations = []contract.Target{{ID: "json", Root: "config.json", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.Skill}}}
	a.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
		var out []contract.Item
		for i, v := range values {
			out = append(out, contract.Item{Target: "json", Entry: e.Ref, Kind: contract.JSONEntry, Path: "config.json", Key: fmt.Sprintf("servers.k%d", i), Value: []byte(v)})
		}
		return out, nil
	}
	return a
}
func TestMultiItemAgreementAndDisagreement(t *testing.T) {
	a := jsonAdapter("first", `{"b":2,"a":1}`, `[1,2]`)
	b := jsonAdapter("second", `{ "a": 1, "b": 2 }`, `[1,2]`)
	if len(desired(t, registry(t, a, b), []contract.Entry{entry("x")}, "first", "second").Items) != 2 {
		t.Fatal("multi-item merge")
	}
	for _, values := range [][]string{{`{"a":1,"b":2}`, `[2,1]`}, {`{"a":1,"b":2}`}} {
		b = jsonAdapter("second", values...)
		_, err := registry(t, a, b).Desired([]contract.Entry{entry("x")}, contract.Project{Harnesses: []string{"first", "second"}})
		if err == nil || !strings.Contains(err.Error(), "first") || !strings.Contains(err.Error(), "second") || strings.Contains(err.Error(), `"a":`) {
			t.Fatalf("unsafe/missing disagreement: %v", err)
		}
	}
}
func TestCrossEntryConflict(t *testing.T) {
	a := jsonAdapter("base", `1`)
	if _, err := registry(t, a).Desired([]contract.Entry{entry("a"), entry("b")}, contract.Project{Harnesses: []string{"base"}}); err == nil {
		t.Fatal("cross-entry collision accepted")
	}
}
func TestInvalidTargets(t *testing.T) {
	for _, tc := range []struct {
		root string
		mode contract.RootMode
		id   string
	}{{"elsewhere", contract.DirectoryRoot, contract.AgentsSkillsDir}, {".agents/skills/nested", contract.DirectoryRoot, "nested"}, {".agents/skills", contract.FileRoot, "file"}, {"../escape", contract.DirectoryRoot, "bad"}} {
		a, b := testadapter.New("a", false), testadapter.New("b", false)
		b.Declarations[0].Root = tc.root
		b.Declarations[0].Mode = tc.mode
		b.Declarations[0].ID = tc.id
		if _, err := adapters.NewRegistry(a, b); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
	if _, err := adapters.NewRegistry(testadapter.New("a", false), testadapter.New("a", false)); err == nil {
		t.Fatal("duplicate names")
	}
}
func TestInvalidItems(t *testing.T) {
	mutations := []func(*contract.Item){
		func(i *contract.Item) { i.Path = "/absolute" }, func(i *contract.Item) { i.Path = ".agents/skills/../escape" },
		func(i *contract.Item) { i.Path = ".agents/skills-trick/x" }, func(i *contract.Item) { i.Path = ".agents/skills" },
		func(i *contract.Item) { i.Path = ".agents\\skills\\x" }, func(i *contract.Item) { i.Target = "undeclared" },
		func(i *contract.Item) { i.Entry = "skill:wrong" }, func(i *contract.Item) { i.Kind = "unknown" },
		func(i *contract.Item) {
			i.Kind = contract.JSONEntry
			i.Link = ""
			i.Key = "x"
			i.Value = []byte(`{} {}`)
		},
		func(i *contract.Item) {
			i.Kind = contract.JSONArrayElement
			i.Link = ""
			i.Key = "x"
			i.Value = []byte(`42`)
		},
	}
	for n, mutate := range mutations {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			a := testadapter.New("a", false)
			a.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
				i := contract.Item{Target: contract.AgentsSkillsDir, Entry: e.Ref, Kind: contract.Symlink, Path: ".agents/skills/x", Link: "/library/x"}
				mutate(&i)
				return []contract.Item{i}, nil
			}
			if _, err := registry(t, a).Desired([]contract.Entry{entry("x")}, contract.Project{Harnesses: []string{"a"}}); err == nil {
				t.Fatal("invalid item accepted")
			}
		})
	}
}
func TestStableCopiesAndFragmentOrder(t *testing.T) {
	a := jsonAdapter("a", `{"x":1}`)
	r := registry(t, a)
	e := entry("x")
	first := desired(t, r, []contract.Entry{e}, "a")
	if !reflect.DeepEqual(first, desired(t, r, []contract.Entry{e}, "a")) {
		t.Fatal("unstable")
	}
	first.Items[0].Value[0] = '!'
	first.Targets[0].Types[0] = library.Agent
	if !json.Valid(desired(t, r, []contract.Entry{e}, "a").Items[0].Value) || r.Targets()[0].Types[0] != library.Skill {
		t.Fatal("output alias")
	}
	a = testadapter.New("fragments", false)
	a.Declarations[0].Types = []contract.ArtifactType{library.Fragment}
	a.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
		return []contract.Item{{Target: contract.AgentsSkillsDir, Entry: e.Ref, Kind: contract.File, Path: ".agents/skills/" + e.Artifact.Name, Bytes: e.Content}}, nil
	}
	entries := []contract.Entry{entry("z"), entry("a")}
	for i := range entries {
		entries[i].Artifact.Type = library.Fragment
		entries[i].Content = []byte(entries[i].Ref)
	}
	got := desired(t, registry(t, a), entries, "fragments")
	if got.Items[0].Entry != entries[0].Ref {
		t.Fatal("fragment order changed")
	}
	got.Items[0].Bytes[0] = '!'
	if entries[0].Content[0] == '!' {
		t.Fatal("input alias")
	}
}

func TestSharedFileOwnership(t *testing.T) {
	for _, kind := range []contract.ItemKind{contract.JSONEntry, contract.JSONArrayElement, contract.MarkdownSection} {
		t.Run(string(kind), func(t *testing.T) {
			a := jsonAdapter("a", `1`)
			a.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
				i := contract.Item{Target: "json", Entry: e.Ref, Kind: kind, Path: "config.json"}
				switch kind {
				case contract.JSONEntry:
					i.Key = "servers." + e.Artifact.Name
					i.Value = []byte(`{"b":2,"a":1}`)
				case contract.JSONArrayElement:
					i.Key = "approved"
					i.Value = []byte(`"` + e.Artifact.Name + `"`)
				case contract.MarkdownSection:
					i.Marker = e.Artifact.Name
					i.Bytes = []byte("exact\nbody\n")
				}
				return []contract.Item{i}, nil
			}
			r := registry(t, a)
			if len(desired(t, r, []contract.Entry{entry("a"), entry("b")}, "a").Items) != 2 {
				t.Fatal("distinct shared-file entries lost")
			}
			if _, err := r.Desired([]contract.Entry{entry("a"), entry("a")}, contract.Project{Harnesses: []string{"a"}}); err != nil {
				t.Fatal("identical pair should deduplicate", err)
			}
			partial := a.Render
			a.Render = func(e contract.Entry, p contract.Project) ([]contract.Item, error) {
				if e.Artifact.Name == "whole" {
					return []contract.Item{{Target: "json", Entry: e.Ref, Kind: contract.File, Path: "config.json", Bytes: []byte("whole file")}}, nil
				}
				return partial(e, p)
			}
			if _, err := r.Desired([]contract.Entry{entry("a"), entry("whole")}, contract.Project{Harnesses: []string{"a"}}); err == nil {
				t.Fatal("whole file overlapped a partial owner")
			}
		})
	}
}

func TestGroupOrderingAndExactBytes(t *testing.T) {
	a := jsonAdapter("a", `1`, `2`)
	b := jsonAdapter("b", `1`, `2`)
	render := b.Render
	b.Render = func(e contract.Entry, p contract.Project) ([]contract.Item, error) {
		items, err := render(e, p)
		slices.Reverse(items)
		return items, err
	}
	if len(desired(t, registry(t, a, b), []contract.Entry{entry("x")}, "a", "b").Items) != 2 {
		t.Fatal("item order is semantic")
	}
	for _, kind := range []contract.ItemKind{contract.File, contract.MarkdownSection} {
		a, b = testadapter.New("a", false), testadapter.New("b", false)
		renderer := func(body string) func(contract.Entry, contract.Project) ([]contract.Item, error) {
			return func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
				i := contract.Item{Target: contract.AgentsSkillsDir, Entry: e.Ref, Kind: kind, Path: ".agents/skills/x", Bytes: []byte(body)}
				if kind == contract.MarkdownSection {
					i.Marker = "managed"
				}
				return []contract.Item{i}, nil
			}
		}
		a.Render = renderer("body\n")
		b.Render = renderer("body")
		if _, err := registry(t, a, b).Desired([]contract.Entry{entry("x")}, contract.Project{Harnesses: []string{"a", "b"}}); err == nil {
			t.Fatal("exact bytes ignored")
		}
	}
}

func TestAdapterInputsIsolated(t *testing.T) {
	a, b := testadapter.New("a", false), testadapter.New("b", false)
	a.Render = func(e contract.Entry, p contract.Project) ([]contract.Item, error) {
		e.Content[0] = '!'
		e.Harnesses[0] = "mutated"
		p.Harnesses[0] = "mutated"
		e.Artifact.MCP.Env["TOKEN"] = "mutated"
		return nil, nil
	}
	b.Render = func(e contract.Entry, p contract.Project) ([]contract.Item, error) {
		if string(e.Content) != "source" || e.Harnesses[0] != "a" || p.Harnesses[0] != "a" || e.Artifact.MCP.Env["TOKEN"] != "${TOKEN}" {
			t.Fatal("one adapter mutated another's input")
		}
		return nil, nil
	}
	e := entry("x")
	e.Content = []byte("source")
	e.Harnesses = []string{"a", "b"}
	e.Artifact.MCP = &library.MCPDefinition{Env: map[string]string{"TOKEN": "${TOKEN}"}}
	desired(t, registry(t, a, b), []contract.Entry{e}, "a", "b")
	if string(e.Content) != "source" || e.Artifact.MCP.Env["TOKEN"] != "${TOKEN}" || e.Harnesses[0] != "a" {
		t.Fatal("caller input mutated")
	}
}
