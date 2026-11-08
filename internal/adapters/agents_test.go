package adapters_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/claudecode"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func TestDefaultClaudeAgents(t *testing.T) {
	r, err := adapters.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	a, ok := r.Lookup("claude-code")
	if !ok || !a.Supports(library.Agent) || !a.Supports(library.Skill) {
		t.Fatal("Claude agent adapter absent")
	}
	if !reflect.DeepEqual(r.Names(), []string{"claude-code", "pi"}) {
		t.Fatal(r.Names())
	}
	want := []contract.Target{
		{ID: contract.AgentsMD, Root: "AGENTS.md", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.Fragment}},
		{ID: contract.AgentsSkillsDir, Root: ".agents/skills", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Skill}},
		{ID: contract.ClaudeAgentsDir, Root: ".claude/agents", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Agent}},
		{ID: contract.ClaudeMCPApproval, Root: ".claude/settings.local.json", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.MCP}},
		{ID: contract.ClaudeMCPJSON, Root: ".mcp.json", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.MCP}},
		{ID: contract.ClaudeMD, Root: "CLAUDE.md", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.Fragment}},
		{ID: contract.ClaudeSkillsDir, Root: ".claude/skills", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Skill}},
		{ID: contract.PiMCPJSON, Root: ".pi/mcp.json", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.MCP}},
	}
	if !reflect.DeepEqual(r.Targets(), want) {
		t.Fatal(r.Targets())
	}
}

func TestAgentFixturePipeline(t *testing.T) {
	cases := []struct {
		name, harnesses, agents string
		claude, pi              bool
	}{
		{"claude", `["claude-code"]`, `["reviewer"]`, true, false},
		{"inherited-both", `["claude-code", "pi"]`, `["reviewer"]`, true, true},
		{"pi-only", `["pi"]`, `["reviewer"]`, false, true},
		{"pi-restricted", `["claude-code", "pi"]`, `[{ name = "reviewer", harnesses = ["pi"] }]`, false, true},
		{"claude-restricted", `["claude-code", "pi"]`, `[{ name = "reviewer", harnesses = ["claude-code"] }]`, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := testutil.NewFixture(t, false)
			sentinel := testutil.PlantSentinel(t, f)
			native := "---\nname: reviewer\ndescription: Review changes\ntools: [Read]\nmodel: sonnet\npermissionMode: plan\nmcpServers: [docs]\nfuture: true\n---\nKeep ${DAYPACK_TEST_SECRET} literal.\n"
			// Exercise distinct newline variants through the real loader and resolver.
			if tc.name == "claude-restricted" {
				native = strings.TrimSuffix(native, "\n")
			}
			if tc.name == "inherited-both" {
				native = strings.ReplaceAll(native, "\n", "\r\n")
			}
			if err := os.WriteFile(filepath.Join(f.Library, "agents/reviewer.md"), []byte(native), 0600); err != nil {
				t.Fatal(err)
			}
			body := "version = 1\nharnesses = " + tc.harnesses + "\nagents = " + tc.agents + "\n"
			if err := os.WriteFile(filepath.Join(f.Project, "daypack.toml"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			before := testutil.SnapshotTree(t, f.Project)
			lib, err := library.Load(f.Library)
			if err != nil {
				t.Fatal(err)
			}
			m, err := manifest.LoadManifest(filepath.Join(f.Project, "daypack.toml"))
			if err != nil {
				t.Fatal(err)
			}
			if problems := m.Validate(lib); len(problems) != 0 {
				t.Fatal(problems)
			}
			var entries []contract.Entry
			for _, e := range m.Entries() {
				if e.Type != library.Agent {
					continue
				}
				a, err := lib.Resolve("agent:" + e.Name)
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(a.Path)
				if err != nil {
					t.Fatal(err)
				}
				entries = append(entries, contract.FromManifest(e, a, data))
			}
			if len(entries) != 1 {
				t.Fatal(entries)
			}
			r, err := adapters.NewRegistry(claudecode.New(), testadapter.New("pi", false))
			if err != nil {
				t.Fatal(err)
			}
			result, err := r.Desired(entries, contract.Project{Root: f.Project, Harnesses: m.Harnesses})
			if err != nil {
				t.Fatal(err)
			}
			if tc.claude {
				want := []contract.Item{{Target: contract.ClaudeAgentsDir, Entry: "agent:reviewer", Kind: contract.File, Path: ".claude/agents/reviewer.md", Bytes: []byte(native)}}
				if !reflect.DeepEqual(result.Items, want) || !bytes.Equal(result.Items[0].Bytes, []byte(native)) {
					t.Fatalf("items: %#v", result.Items)
				}
			} else if len(result.Items) != 0 {
				t.Fatal(result.Items)
			}
			if tc.pi {
				want := []contract.Unsupported{{Harness: "pi", Entry: "agent:reviewer", Type: library.Agent}}
				if !reflect.DeepEqual(result.Unsupported, want) {
					t.Fatal(result.Unsupported)
				}
			} else if len(result.Unsupported) != 0 {
				t.Fatal(result.Unsupported)
			}
			testutil.AssertTree(t, f.Project, before)
			testutil.AssertNoSecret(t, f.Project, sentinel)
		})
	}
}
