package apply_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/apply"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/fluid-movement/daypack/internal/plan"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func setup(t *testing.T, text string, ads ...contract.Adapter) (testutil.Fixture, *manifest.Manifest, *library.Library, *adapters.Registry) {
	t.Helper()
	f := testutil.NewFixture(t, false)
	if err := os.WriteFile(filepath.Join(f.Project, "daypack.toml"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.LoadManifest(filepath.Join(f.Project, "daypack.toml"))
	if err != nil {
		t.Fatal(err)
	}
	lib, err := library.Load(f.Library)
	if err != nil {
		t.Fatal(err)
	}
	r, err := adapters.NewRegistry(ads...)
	if err != nil {
		t.Fatal(err)
	}
	return f, m, lib, r
}

func sharedAdapter(name string, types ...library.ArtifactType) *testadapter.Adapter {
	a := testadapter.New(name, false)
	a.Declarations = []contract.Target{{ID: "shared", Root: ".config/shared.json", Mode: contract.FileRoot, Types: types}}
	return a
}

func TestBuildUsesRestrictedHarnessOnSharedTarget(t *testing.T) {
	target := contract.Target{ID: contract.AgentsSkillsDir, Root: ".agents/skills", Mode: contract.DirectoryRoot, Types: []library.ArtifactType{library.Skill}}
	a := testadapter.New("claude-code", false)
	a.Declarations = []contract.Target{target}
	b := testadapter.New("pi", false)
	b.Declarations = []contract.Target{target}
	a.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
		return []contract.Item{{Target: target.ID, Entry: e.Ref, Kind: contract.File, Path: ".agents/skills/testing.txt", Bytes: []byte("claude-code")}}, nil
	}
	b.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
		return []contract.Item{{Target: target.ID, Entry: e.Ref, Kind: contract.File, Path: ".agents/skills/testing.txt", Bytes: []byte("pi")}}, nil
	}
	f, m, lib, r := setup(t, "version=1\nharnesses=[\"claude-code\",\"pi\"]\nskills=[{name=\"testing\",harnesses=[\"pi\"]}]\n", a, b)
	got, err := apply.Build(context.Background(), f.Project, m, lib, r, apply.Options{})
	if err != nil {
		t.Fatal(err)
	}
	p := got.Plan()
	if len(p.Operations) != 1 || p.Operations[0].Target != target.ID {
		t.Fatalf("restricted shared target plan: %#v", p.Operations)
	}
	if _, err = got.Execute(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	bts, err := os.ReadFile(filepath.Join(f.Project, ".agents/skills/testing.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(bts) != "pi" {
		t.Fatalf("shared producer content=%q, want pi", bts)
	}
}

func TestSharedTargetRejectsDifferentProducers(t *testing.T) {
	target := contract.Target{ID: contract.AgentsSkillsDir, Root: ".agents/skills", Mode: contract.DirectoryRoot, Types: []library.ArtifactType{library.Skill}}
	a := testadapter.New("claude-code", false)
	a.Declarations = []contract.Target{target}
	b := testadapter.New("pi", false)
	b.Declarations = []contract.Target{target}
	a.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
		return []contract.Item{{Target: target.ID, Entry: e.Ref, Kind: contract.File, Path: ".agents/skills/testing.txt", Bytes: []byte("claude-code")}}, nil
	}
	b.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
		return []contract.Item{{Target: target.ID, Entry: e.Ref, Kind: contract.File, Path: ".agents/skills/testing.txt", Bytes: []byte("pi")}}, nil
	}
	f, m, lib, r := setup(t, "version=1\nharnesses=[\"claude-code\",\"pi\"]\nskills=[\"testing\"]\n", a, b)
	if _, err := apply.Build(context.Background(), f.Project, m, lib, r, apply.Options{}); err == nil || !strings.Contains(err.Error(), "disagrees") {
		t.Fatalf("expected producer disagreement, got %v", err)
	}
}

func TestBuildAndExecuteAllSharedLockKinds(t *testing.T) {
	a := testadapter.New("claude-code", false)
	a.Declarations = []contract.Target{
		{ID: "mcp-json", Root: ".config/mcp.json", Mode: contract.FileRoot, Types: []library.ArtifactType{library.MCP}},
		{ID: "mcp-array", Root: ".config/mcp-array.json", Mode: contract.FileRoot, Types: []library.ArtifactType{library.MCP}},
		{ID: "instructions", Root: "AGENTS.md", Mode: contract.FileRoot, Types: []library.ArtifactType{library.Fragment, library.Agent}},
	}
	a.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
		switch e.Artifact.Type {
		case library.MCP:
			return []contract.Item{{Target: "mcp-json", Entry: e.Ref, Kind: contract.JSONEntry, Path: ".config/mcp.json", Key: "mcp.docs", Value: []byte(`{"command":"echo"}`)}, {Target: "mcp-array", Entry: e.Ref, Kind: contract.JSONArrayElement, Path: ".config/mcp-array.json", Key: "servers", Value: []byte(`"docs"`)}}, nil
		case library.Fragment:
			return []contract.Item{{Target: "instructions", Entry: e.Ref, Kind: contract.MarkdownSection, Path: "AGENTS.md", Marker: "fragment", Bytes: []byte("\nfragment\n")}}, nil
		case library.Agent:
			return []contract.Item{{Target: "instructions", Entry: e.Ref, Kind: contract.MarkdownSection, Path: "AGENTS.md", Marker: "agent", Bytes: []byte("\nagent\n")}}, nil
		}
		return nil, nil
	}
	f, m, lib, r := setup(t, "version=1\nharnesses=[\"claude-code\"]\nmcp=[\"docs\"]\nfragments=[\"conventions\"]\nagents=[\"reviewer\"]\n", a)
	x, err := apply.Build(context.Background(), f.Project, m, lib, r, apply.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = x.Execute(context.Background(), x.Plan()); err != nil {
		t.Fatal(err)
	}
	l, err := lock.Load(f.Project)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[lock.Kind]int{}
	for _, i := range l.Items() {
		kinds[i.Kind]++
	}
	for _, k := range []lock.Kind{lock.JSONEntry, lock.JSONArrayElement, lock.MDSection} {
		if kinds[k] == 0 {
			t.Errorf("lock missing kind %s: %#v", k, kinds)
		}
	}
}

const inlineBase = "version=1\nharnesses=[\"claude-code\"]\n[inline.mcp.local]\ntype=\"stdio\"\ncommand=\"%s\"\nargs=[\"--safe\"]\n[inline.mcp.local.env]\nTOKEN=\"%s\"\n"

func inlineAdapter() *testadapter.Adapter {
	a := sharedAdapter("claude-code", library.MCP)
	a.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
		return []contract.Item{{Target: "shared", Entry: e.Ref, Kind: contract.JSONEntry, Path: ".config/shared.json", Key: "mcp.local", Value: []byte(`{"managed":true}`)}}, nil
	}
	return a
}
func buildInline(t *testing.T, f testutil.Fixture, command, env string) (*apply.Reconciliation, error) {
	t.Helper()
	m, err := manifest.LoadManifest(filepath.Join(f.Project, "daypack.toml"))
	if err != nil {
		return nil, err
	}
	lib, err := library.Load(f.Library)
	if err != nil {
		return nil, err
	}
	r, err := adapters.NewRegistry(inlineAdapter())
	if err != nil {
		return nil, err
	}
	return apply.Build(context.Background(), f.Project, m, lib, r, apply.Options{})
}
func executeInline(t *testing.T, f testutil.Fixture, cmd, env string) plan.Plan {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.Project, "daypack.toml"), []byte(fmt.Sprintf(inlineBase, cmd, env)), 0600); err != nil {
		t.Fatal(err)
	}
	x, err := buildInline(t, f, cmd, env)
	if err != nil {
		t.Fatal(err)
	}
	p, err := x.Execute(context.Background(), x.Plan())
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func gated(p plan.Plan) bool {
	for _, o := range p.Operations {
		if o.Entry == "mcp:local" {
			return o.Gated
		}
	}
	return false
}

func TestInlineConsentHistoryTracksExecutableIdentityOnly(t *testing.T) {
	f := testutil.NewFixture(t, false)
	if p := executeInline(t, f, "runner", "${TOKEN}"); !gated(p) {
		t.Fatal("first apply should be gated")
	}
	if p := executeInline(t, f, "runner", "${OTHER_TOKEN}"); gated(p) {
		t.Fatal("environment-only change should retain consent")
	}
	if p := executeInline(t, f, "runner-v2", "${OTHER_TOKEN}"); !gated(p) {
		t.Fatal("command change should require consent")
	}
	// Legacy lock entries without consent identity must be gated again.
	l, err := lock.Load(f.Project)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range l.Items() {
		i.MCPIdentity = ""
		if err = l.Put(i); err != nil {
			t.Fatal(err)
		}
	}
	if err = l.Save(f.Project); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(f.Project, "daypack.toml"), []byte(fmt.Sprintf(inlineBase, "runner-v2", "${OTHER_TOKEN}")), 0600); err != nil {
		t.Fatal(err)
	}
	x, err := buildInline(t, f, "runner-v2", "${OTHER_TOKEN}")
	if err != nil {
		t.Fatal(err)
	}
	if !gated(x.Plan()) {
		t.Fatal("legacy history should be gated")
	}
	_ = executeInline(t, f, "runner-v2", "${OTHER_TOKEN}")
	if err = os.Remove(filepath.Join(f.Project, ".daypack", "lock.toml")); err != nil {
		t.Fatal(err)
	}
	x, err = buildInline(t, f, "runner-v2", "${OTHER_TOKEN}")
	if err != nil {
		t.Fatal(err)
	}
	if !gated(x.Plan()) {
		t.Fatal("missing ownership history should be gated")
	}
}

func TestPlanDoesNotExposeSecretAndBuildIsReadOnly(t *testing.T) {
	f := testutil.NewFixture(t, false)
	secret := "must-never-appear-829173"
	t.Setenv("TOKEN", secret)
	if err := os.WriteFile(filepath.Join(f.Project, "daypack.toml"), []byte(fmt.Sprintf(inlineBase, "runner", "${TOKEN}")), 0600); err != nil {
		t.Fatal(err)
	}
	before := testutil.SnapshotTree(t, f.Project)
	libraryBefore := testutil.SnapshotTree(t, f.Library)
	x, err := buildInline(t, f, "runner", "${TOKEN}")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(x.Plan())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), secret) {
		t.Fatal("plan exposed secret")
	}
	testutil.AssertTree(t, f.Project, before)
	testutil.AssertTree(t, f.Library, libraryBefore)
	if _, err = os.Stat(filepath.Join(f.Project, ".daypack", "lock.toml")); !os.IsNotExist(err) {
		t.Fatal("Build created lockfile")
	}
}

func TestAdoptAndForceAreDistinct(t *testing.T) {
	a := sharedAdapter("claude-code", library.Skill)
	a.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
		return []contract.Item{{Target: "shared", Entry: e.Ref, Kind: contract.File, Path: ".config/shared.json", Bytes: []byte("managed")}}, nil
	}
	f, m, lib, r := setup(t, "version=1\nharnesses=[\"claude-code\"]\nskills=[\"testing\"]\n", a)
	p := filepath.Join(f.Project, ".config/shared.json")
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("handwritten"), 0600); err != nil {
		t.Fatal(err)
	}
	x, err := apply.Build(context.Background(), f.Project, m, lib, r, apply.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if x.Plan().Operations[0].Action != plan.Conflict {
		t.Fatal("unowned content should conflict without Adopt")
	}
	x, err = apply.Build(context.Background(), f.Project, m, lib, r, apply.Options{Adopt: true})
	if err != nil {
		t.Fatal(err)
	}
	if x.Plan().Operations[0].Action != plan.Update {
		t.Fatalf("expected planned takeover update, got %#v", x.Plan().Operations)
	}
	// Execute takeover, then create managed drift. Force alone may replace it.
	if _, err = x.Execute(context.Background(), x.Plan()); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(p, []byte("user edit"), 0600); err != nil {
		t.Fatal(err)
	}
	x, err = apply.Build(context.Background(), f.Project, m, lib, r, apply.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if x.Plan().Operations[0].Action != plan.Drift {
		t.Fatal("drift should be skipped without Force")
	}
	x, err = apply.Build(context.Background(), f.Project, m, lib, r, apply.Options{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if x.Plan().Operations[0].Action != plan.Update {
		t.Fatalf("Force should permit update, got %#v", x.Plan().Operations)
	}
}

func TestUnresolvedManifestEntryRetainsExistingManagedItem(t *testing.T) {
	a := testadapter.New("claude-code", false)
	f, m, lib, r := setup(t, "version=1\nharnesses=[\"claude-code\"]\nskills=[\"testing\"]\n", a)
	x, err := apply.Build(context.Background(), f.Project, m, lib, r, apply.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = x.Execute(context.Background(), x.Plan()); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(f.Project, ".agents/skills/testing")
	if _, err = os.Lstat(old); err != nil {
		t.Fatal(err)
	}
	if err = os.RemoveAll(filepath.Join(f.Library, "skills/testing")); err != nil {
		t.Fatal(err)
	}
	// The manifest still names testing, but the library entry is unresolved.
	lib, err = library.Load(f.Library)
	if err != nil {
		t.Fatal(err)
	}
	x, err = apply.Build(context.Background(), f.Project, m, lib, r, apply.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range x.Plan().Operations {
		if o.Action == plan.Remove {
			t.Fatal("unresolved entry planned removal")
		}
	}
	if _, err = os.Lstat(old); err != nil {
		t.Fatal("unresolved apply removed prior item:", err)
	}
}

func TestSharedTargetLastProducerRemoval(t *testing.T) {
	a, b := testadapter.New("pi", false), testadapter.New("claude-code", true)
	f, m, lib, registry := setup(t, "version=1\nharnesses=[\"pi\",\"claude-code\"]\nskills=[\"testing\"]\n", a, b)
	run := func() plan.Plan {
		t.Helper()
		r, err := apply.Build(context.Background(), f.Project, m, lib, registry, apply.Options{})
		if err != nil {
			t.Fatal(err)
		}
		p, err := r.Execute(context.Background(), r.Plan())
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	if p := run(); len(p.Operations) != 2 {
		t.Fatalf("shared target produced more than once: %v", p.Operations)
	}
	shared := filepath.Join(f.Project, ".agents/skills/testing")
	before, err := os.Lstat(shared)
	if err != nil {
		t.Fatal(err)
	}
	m.Harnesses = []string{"pi"}
	p := run()
	if len(p.Operations) != 1 || p.Operations[0].Target == contract.AgentsSkillsDir || p.Operations[0].Action != plan.Remove {
		t.Fatalf("wrong last harness removal: %v", p.Operations)
	}
	after, err := os.Lstat(shared)
	if err != nil || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("shared item rewritten while still used")
	}
	m.Skills = nil
	p = run()
	if len(p.Operations) != 1 || p.Operations[0].Target != contract.AgentsSkillsDir || p.Operations[0].Action != plan.Remove {
		t.Fatalf("last producer did not remove shared item: %v", p.Operations)
	}
	if _, err = os.Lstat(shared); !os.IsNotExist(err) {
		t.Fatal("last producer retained link")
	}
}

func TestHandwrittenSharedContainersAndPreservedMembership(t *testing.T) {
	a := sharedAdapter("pi", library.MCP)
	a.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
		return []contract.Item{
			{Target: "shared", Entry: e.Ref, Kind: contract.JSONEntry, Path: ".config/shared.json", Key: "servers.docs", Value: []byte(`{"b":2,"a":1}`)},
			{Target: "shared", Entry: e.Ref, Kind: contract.JSONArrayElement, Path: ".config/shared.json", Key: "approved", Value: []byte(`"docs"`)},
		}, nil
	}
	f, m, lib, registry := setup(t, "version=1\nharnesses=[\"pi\"]\nmcp=[\"docs\"]\n", a)
	path := filepath.Join(f.Project, ".config/shared.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"approved":["docs","handwritten"],"other":{"keep":true}}`), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := apply.Build(context.Background(), f.Project, m, lib, registry, apply.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Execute(context.Background(), r.Plan()); err != nil {
		t.Fatal(err)
	}
	owned, err := lock.Load(f.Project)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range owned.Items() {
		if i.Kind == lock.JSONArrayElement && (i.Value != "docs" || !i.Preserve) {
			t.Fatal("preexisting membership not decoded/preserved")
		}
		if i.Kind == lock.JSONEntry {
			expected, e := lock.HashJSON([]byte(`{"a":1,"b":2}`))
			if e != nil {
				t.Fatal(e)
			}
			if i.Hash != expected {
				t.Fatal("noncanonical JSON hash")
			}
		}
	}
	m.MCP = nil
	r, err = apply.Build(context.Background(), f.Project, m, lib, registry, apply.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Execute(context.Background(), r.Plan()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	members := value["approved"].([]any)
	if len(members) != 2 || members[0] != "docs" || value["other"] == nil {
		t.Fatalf("handwritten data removed: %s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("shared container mode changed")
	}
}

func TestInlineUnchangedRequiredMemberIsRechecked(t *testing.T) {
	a := inlineAdapter()
	a.Declarations = append(a.Declarations, contract.Target{ID: "approval", Root: ".config/approval.json", Mode: contract.FileRoot, Types: []library.ArtifactType{library.MCP}})
	a.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
		value, err := json.Marshal(e.Artifact.MCP.Env)
		if err != nil {
			return nil, err
		}
		return []contract.Item{
			{Target: "shared", Entry: e.Ref, Kind: contract.JSONEntry, Path: ".config/shared.json", Key: "mcp.local", Value: value},
			{Target: "approval", Entry: e.Ref, Kind: contract.JSONArrayElement, Path: ".config/approval.json", Key: "approved", Value: []byte(`"local"`)},
		}, nil
	}
	f, m, lib, registry := setup(t, fmt.Sprintf(inlineBase, "runner", "${TOKEN}"), a)
	r, err := apply.Build(context.Background(), f.Project, m, lib, registry, apply.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Execute(context.Background(), r.Plan()); err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(filepath.Join(f.Project, ".config/shared.json"))
	if err != nil {
		t.Fatal(err)
	}
	m.InlineMCP[0].Inline.Env["TOKEN"] = "${OTHER}"
	r, err = apply.Build(context.Background(), f.Project, m, lib, registry, apply.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plan().Operations) != 1 || r.Plan().Operations[0].Gated {
		t.Fatal("env-only change should update without regating")
	}
	if err = os.WriteFile(filepath.Join(f.Project, ".config/approval.json"), []byte(`{"approved":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	p, err := r.Execute(context.Background(), r.Plan())
	if err != nil {
		t.Fatal(err)
	}
	if p.Operations[0].Outcome != plan.Skipped {
		t.Fatal("required unchanged member was not rechecked")
	}
	now, err := os.ReadFile(filepath.Join(f.Project, ".config/shared.json"))
	if err != nil || string(now) != string(old) {
		t.Fatal("partial inline update escaped preflight")
	}
}
