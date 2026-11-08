package pi_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/pi"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func skill(name string) contract.Entry {
	return contract.Entry{Ref: "skill:" + name, Artifact: library.Artifact{Type: library.Skill, Name: name, Path: filepath.Join(string(filepath.Separator), "nonexistent library", "skills", name)}, Harnesses: []string{"pi"}, Content: []byte("unused")}
}

func TestPureRenderer(t *testing.T) {
	a := pi.New()
	p := contract.Project{Root: filepath.Join(string(filepath.Separator), "nonexistent project"), Harnesses: []string{"pi"}}
	cases := []testadapter.Case{{Entry: skill("testing"), Project: p, Supported: true}}
	for _, typ := range []library.ArtifactType{library.Agent, library.Fragment} {
		e := skill("testing")
		e.Artifact.Type = typ
		cases = append(cases, testadapter.Case{Entry: e, Project: p})
		items, err := a.Desired(e, p)
		if err != nil || len(items) != 0 || a.Supports(typ) {
			t.Fatalf("unsupported %s: %v %v", typ, items, err)
		}
	}
	testadapter.CheckAdapter(t, a, cases)
	items, err := a.Desired(skill("testing"), p)
	if err != nil {
		t.Fatal(err)
	}
	want := []contract.Item{{Target: contract.AgentsSkillsDir, Entry: "skill:testing", Kind: contract.Symlink, Path: ".agents/skills/testing", Link: skill("testing").Artifact.Path}}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("got %#v", items)
	}
	targets := a.Targets()
	expected := []contract.Target{
		{ID: contract.AgentsSkillsDir, Root: ".agents/skills", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Skill}},
		{ID: contract.PiMCPJSON, Root: ".pi/mcp.json", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.MCP}},
	}
	if !reflect.DeepEqual(targets, expected) || a.Name() != "pi" || len(a.Notes(p)) != 1 {
		t.Fatal("declarations")
	}
	targets[0].Types[0] = library.MCP
	targets[0].Root = "changed"
	if !reflect.DeepEqual(a.Targets(), expected) {
		t.Fatal("aliased targets")
	}
	for _, name := range []string{"", "..", "../escape", "a/b", "bad\nname"} {
		e := skill(name)
		if _, err := a.Desired(e, p); err == nil {
			t.Errorf("accepted name %q", name)
		}
	}
	for _, ref := range []string{"", "testing", "skill:other", "mcp:testing"} {
		e := skill("testing")
		e.Ref = ref
		if _, err := a.Desired(e, p); err == nil {
			t.Errorf("accepted ref %q", ref)
		}
	}
	for _, source := range []string{"", "relative", "/absolute\x00secret", "/absolute\rsecret", "/absolute\nsecret"} {
		e := skill("testing")
		e.Artifact.Path = source
		if _, err := a.Desired(e, p); err == nil {
			t.Errorf("accepted source %q", source)
		}
	}
}

func TestSkillsGolden(t *testing.T) {
	f := testutil.NewFixture(t, false)
	r, err := adapters.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	entries := []contract.Entry{skill("testing"), skill("another")}
	for i := range entries {
		entries[i].Artifact.Path = filepath.Join(f.Library, "skills", entries[i].Artifact.Name)
	}
	p := contract.Project{Root: f.Project, Harnesses: []string{"pi"}}
	first, err := r.Desired(entries, p)
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.Desired(entries, p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("nondeterministic output")
	}
	for _, i := range first.Items {
		if !filepath.IsAbs(i.Link) {
			t.Fatal("relative link")
		}
	}
	data, err := json.Marshal(first.Items)
	if err != nil {
		t.Fatal(err)
	}
	testutil.AssertJSONGolden(t, "testdata/skills.golden.json", testutil.NormalizeFixturePaths(data, f.Library, f.Project))
}

func TestTrustAdvice(t *testing.T) {
	a := pi.New()
	p := contract.Project{Root: "/nonexistent project", Harnesses: []string{"pi"}}
	notes := a.Notes(p)
	if len(notes) != 1 || notes[0].Harness != "pi" || notes[0].Code != "project-trust-required" || !strings.Contains(notes[0].Message, "/trust") || !strings.Contains(notes[0].Message, "headless") {
		t.Fatal(notes)
	}
	notes[0].Message = "changed"
	if a.Notes(p)[0].Message == "changed" {
		t.Fatal("notes alias")
	}
}
