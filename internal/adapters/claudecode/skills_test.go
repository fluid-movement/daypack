package claudecode_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/claudecode"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func skill(name string) contract.Entry {
	return contract.Entry{Ref: "skill:" + name, Artifact: library.Artifact{Type: library.Skill, Name: name, Path: filepath.Join(string(filepath.Separator), "nonexistent library", "skills", name)}, Harnesses: []string{"claude-code"}, Content: []byte("unused")}
}

func TestPureRenderer(t *testing.T) {
	a := claudecode.New()
	p := contract.Project{Root: filepath.Join(string(filepath.Separator), "nonexistent project"), Harnesses: []string{"claude-code"}}
	cases := []testadapter.Case{{Entry: skill("testing"), Project: p, Supported: true}}
	for _, typ := range []library.ArtifactType{library.Fragment} {
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
	want := []contract.Item{{Target: contract.ClaudeSkillsDir, Entry: "skill:testing", Kind: contract.Symlink, Path: ".claude/skills/testing", Link: skill("testing").Artifact.Path}}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("got %#v", items)
	}
	targets := a.Targets()
	expected := []contract.Target{
		{ID: contract.ClaudeSkillsDir, Root: ".claude/skills", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Skill}},
		{ID: contract.ClaudeAgentsDir, Root: ".claude/agents", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Agent}},
		{ID: contract.ClaudeMCPJSON, Root: ".mcp.json", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.MCP}},
		{ID: contract.ClaudeMCPApproval, Root: ".claude/settings.local.json", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.MCP}},
	}
	if !reflect.DeepEqual(targets, expected) || a.Name() != "claude-code" || len(a.Notes(p)) != 0 {
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

func TestRegistrySelection(t *testing.T) {
	r, err := adapters.NewRegistry(claudecode.New(), testadapter.New("pi", false))
	if err != nil {
		t.Fatal(err)
	}
	p := contract.Project{Harnesses: []string{"claude-code", "pi"}}
	for _, tc := range []struct {
		name       string
		harnesses  []string
		claude, pi int
	}{
		{"inherited", nil, 1, 1}, {"claude", []string{"claude-code"}, 1, 0}, {"pi", []string{"pi"}, 0, 1}, {"both", []string{"pi", "claude-code"}, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := skill("testing")
			e.Harnesses = tc.harnesses
			got, err := r.Desired([]contract.Entry{e}, p)
			if err != nil {
				t.Fatal(err)
			}
			counts := map[string]int{}
			for _, i := range got.Items {
				counts[i.Target]++
			}
			if counts[contract.ClaudeSkillsDir] != tc.claude || counts[contract.AgentsSkillsDir] != tc.pi {
				t.Fatal(got)
			}
		})
	}
	if got, err := r.Desired(nil, p); err != nil || len(got.Items) != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}
	e := skill("testing")
	e.Harnesses = []string{}
	if _, err := r.Desired([]contract.Entry{e}, p); err == nil {
		t.Fatal("accepted empty selection")
	}
	for _, typ := range []library.ArtifactType{library.Fragment} {
		e := skill("testing")
		e.Artifact.Type = typ
		got, err := r.Desired([]contract.Entry{e}, p)
		want := []contract.Unsupported{{Harness: "claude-code", Entry: e.Ref, Type: typ}}
		if err != nil || len(got.Items) != 0 || !reflect.DeepEqual(got.Unsupported, want) {
			t.Fatalf("unsupported: %v %v", got, err)
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
	p := contract.Project{Root: f.Project, Harnesses: []string{"claude-code"}}
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
