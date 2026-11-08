package adapters_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/pi"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/library"
)

func TestRealPiSharedProducers(t *testing.T) {
	second := testadapter.New("second", false)
	r := registry(t, pi.New(), second)
	for _, names := range [][]string{{"pi", "second"}, {"pi"}, {"second"}, nil} {
		got := desired(t, r, []contract.Entry{entry("testing")}, names...)
		count := 1
		if len(names) == 0 {
			count = 0
		}
		if len(got.Items) != count || len(got.Targets) != count {
			t.Fatalf("%v: %#v", names, got)
		}
	}
	second.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
		return []contract.Item{{Target: contract.AgentsSkillsDir, Entry: e.Ref, Kind: contract.Symlink, Path: ".agents/skills/" + e.Artifact.Name, Link: "/different"}}, nil
	}
	if _, err := r.Desired([]contract.Entry{entry("testing")}, contract.Project{Harnesses: []string{"pi", "second"}}); err == nil || !strings.Contains(err.Error(), "pi") || !strings.Contains(err.Error(), "second") {
		t.Fatalf("conflict: %v", err)
	}
}

func TestRealPiSelectionAndReaders(t *testing.T) {
	r, err := adapters.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		restriction []string
		targets     []string
	}{
		{nil, []string{contract.ClaudeSkillsDir, contract.AgentsSkillsDir}},
		{[]string{"pi", "claude-code"}, []string{contract.ClaudeSkillsDir, contract.AgentsSkillsDir}},
		{[]string{"pi"}, []string{contract.AgentsSkillsDir}},
		{[]string{"claude-code"}, []string{contract.ClaudeSkillsDir}},
	} {
		e := entry("testing")
		e.Harnesses = tc.restriction
		got := desired(t, r, []contract.Entry{e}, "pi", "claude-code")
		var targets []string
		for _, item := range got.Items {
			targets = append(targets, item.Target)
		}
		if !reflect.DeepEqual(targets, tc.targets) {
			t.Fatalf("%v: %v", tc.restriction, targets)
		}
	}
	for _, restriction := range [][]string{{}, {"unknown"}, {"claude-code"}} {
		e := entry("testing")
		e.Harnesses = restriction
		if _, err := r.Desired([]contract.Entry{e}, contract.Project{Harnesses: []string{"pi"}}); err == nil {
			t.Fatalf("accepted %v", restriction)
		}
	}
	e := entry("testing")
	e.Harnesses = []string{"pi"}
	got := desired(t, r, []contract.Entry{e}, "pi")
	wantReaders := []string{"codex", "cursor", "gemini-cli", "github-copilot", "opencode", "pi"}
	if len(got.Items) != 1 || got.Items[0].Target != contract.AgentsSkillsDir || !reflect.DeepEqual(adapters.Readers(got.Items[0].Target), wantReaders) {
		t.Fatal(got)
	}
	for _, name := range wantReaders[:5] {
		if _, ok := r.Lookup(name); ok {
			t.Fatal("reader registered", name)
		}
	}
	for _, typ := range []library.ArtifactType{library.Agent, library.Fragment} {
		e.Artifact.Type = typ
		e.Ref = string(typ) + ":testing"
		got := desired(t, r, []contract.Entry{e}, "pi")
		if len(got.Items) != 0 || !reflect.DeepEqual(got.Unsupported, []contract.Unsupported{{Harness: "pi", Entry: e.Ref, Type: typ}}) {
			t.Fatal(got)
		}
	}
}
