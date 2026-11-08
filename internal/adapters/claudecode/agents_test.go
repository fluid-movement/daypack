package claudecode_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/claudecode"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func TestSkillsAndAgentsTogether(t *testing.T) {
	r, err := adapters.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	s := skill("testing")
	native := []byte("---\nname: reviewer\ndescription: Review changes\n---\nReview carefully.\n")
	a := contract.Entry{Ref: "agent:reviewer", Artifact: library.Artifact{Type: library.Agent, Name: "reviewer"}, Content: native}
	p := contract.Project{Root: "/nonexistent/project", Harnesses: []string{"claude-code"}}
	result, err := r.Desired([]contract.Entry{s, a}, p)
	if err != nil {
		t.Fatal(err)
	}
	want := []contract.Item{
		{Target: contract.ClaudeSkillsDir, Entry: s.Ref, Kind: contract.Symlink, Path: ".claude/skills/testing", Link: s.Artifact.Path},
		{Target: contract.ClaudeAgentsDir, Entry: a.Ref, Kind: contract.File, Path: ".claude/agents/reviewer.md", Bytes: native},
	}
	if !reflect.DeepEqual(result.Items, want) || len(result.Unsupported) != 0 {
		t.Fatalf("mixed artifacts: %+v", result)
	}
}

func TestAgentItems(t *testing.T) {
	a := claudecode.New()
	if a.Name() != "claude-code" {
		t.Fatal(a.Name())
	}
	wantTargets := []contract.Target{
		{ID: contract.ClaudeSkillsDir, Root: ".claude/skills", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Skill}},
		{ID: contract.ClaudeAgentsDir, Root: ".claude/agents", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Agent}},
	}
	if !reflect.DeepEqual(a.Targets(), wantTargets) {
		t.Fatal(a.Targets())
	}
	targets := a.Targets()
	targets[1].Types[0] = library.Skill
	targets[1].Root = "changed"
	if !reflect.DeepEqual(a.Targets(), wantTargets) {
		t.Fatal("target alias")
	}
	p := contract.Project{Root: "/nonexistent/project", Harnesses: []string{"claude-code"}}
	var golden []contract.Item
	for _, name := range []string{"reviewer", "planner"} {
		native := "---\nname: " + name + "\ndescription: Review changes\ntools: [Read, Grep]\nmodel: sonnet\npermissionMode: plan\nmcpServers: [docs]\nunknown: {future: true}\n---\nReview ${DAYPACK_TEST_SECRET}.\nSecond line.\n"
		for _, data := range []string{native, strings.TrimSuffix(native, "\n"), strings.ReplaceAll(native, "\n", "\r\n")} {
			e := contract.Entry{Ref: "agent:" + name, Artifact: library.Artifact{Type: library.Agent, Name: name, Path: "/nonexistent/source.md"}, Content: []byte(data)}
			testadapter.CheckAdapter(t, a, []testadapter.Case{{Entry: e, Project: p, Supported: true}})
			items, err := a.Desired(e, p)
			if err != nil {
				t.Fatal(err)
			}
			want := []contract.Item{{Target: contract.ClaudeAgentsDir, Entry: e.Ref, Kind: contract.File, Path: ".claude/agents/" + name + ".md", Bytes: []byte(data)}}
			if !reflect.DeepEqual(items, want) {
				t.Fatalf("items: %#v", items)
			}
			golden = append(golden, items[0])
			// The golden retains independent native bytes before checking output aliasing.
			golden[len(golden)-1].Bytes = bytes.Clone(items[0].Bytes)
			items[0].Bytes[0] = '!'
			if !bytes.Equal(e.Content, []byte(data)) {
				t.Fatal("content alias")
			}
		}
	}
	data, err := json.Marshal(golden)
	if err != nil {
		t.Fatal(err)
	}
	testutil.AssertJSONGolden(t, "testdata/agent-items.json", data)
	for _, typ := range []library.ArtifactType{library.MCP, library.Fragment} {
		e := contract.Entry{Artifact: library.Artifact{Type: typ}}
		testadapter.CheckAdapter(t, a, []testadapter.Case{{Entry: e, Supported: false}})
		items, err := a.Desired(e, p)
		if err != nil || len(items) != 0 {
			t.Fatalf("unsupported: %v %v", items, err)
		}
	}
	if len(a.Notes(p)) != 0 {
		t.Fatal("unexpected notes")
	}
}

func TestInvalidAgentInputs(t *testing.T) {
	for _, name := range []string{"", "../escape", "a/b", "a\\b", "Reviewer", strings.Repeat("a", 65), ".", "a.md"} {
		e := contract.Entry{Ref: "agent:" + name, Artifact: library.Artifact{Type: library.Agent, Name: name}}
		if items, err := claudecode.New().Desired(e, contract.Project{}); err == nil || len(items) != 0 {
			t.Fatalf("accepted %q", name)
		}
	}
	e := contract.Entry{Ref: "agent:other", Artifact: library.Artifact{Type: library.Agent, Name: "reviewer"}}
	if items, err := claudecode.New().Desired(e, contract.Project{}); err == nil || len(items) != 0 {
		t.Fatal("accepted mismatched reference")
	}
}
