package claudecode_test

import (
	"encoding/json"
	"fmt"
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

func mcp(name string, d library.MCPDefinition) contract.Entry {
	return contract.Entry{Ref: "mcp:" + name, Artifact: library.Artifact{Type: library.MCP, Name: name, Path: "/nonexistent/source", Description: "not emitted", MCP: &d}, Content: []byte("not TOML")}
}

func TestMCPItems(t *testing.T) {
	a := claudecode.New()
	p := contract.Project{Root: "/nonexistent/project", Harnesses: []string{"claude-code"}}
	entries := []contract.Entry{
		mcp("docs", library.MCPDefinition{Description: "metadata", Type: "stdio", Command: "${BIN:-echo}", Args: []string{"--token", "${DAYPACK_TEST_SECRET}", "one argument", "${OPTION:-default}"}, Env: map[string]string{"TOKEN": "${DAYPACK_TEST_SECRET}", "PATH": "prefix/${DIR:-bin}"}}),
		mcp("remote", library.MCPDefinition{Type: "http", URL: "${BASE:-https://example.invalid}/${RESOURCE}", Headers: map[string]string{"Authorization": "Bearer ${DAYPACK_TEST_SECRET}", "X-Mode": "${MODE:-test}"}}),
		mcp("minimal", library.MCPDefinition{Type: "stdio", Command: "echo"}),
		mcp("minimal-http", library.MCPDefinition{Type: "http", URL: "https://example.invalid"}),
	}
	var golden []contract.Item
	for _, e := range entries {
		testadapter.CheckAdapter(t, a, []testadapter.Case{{Entry: e, Project: p, Supported: true}})
		before := contract.CloneEntry(e)
		items, err := a.Desired(e, p)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 2 {
			t.Fatal("expected server and approval")
		}
		server, approval := items[0], items[1]
		if server.Target != contract.ClaudeMCPJSON || server.Kind != contract.JSONEntry || server.Key != "mcpServers."+e.Artifact.Name || approval.Target != contract.ClaudeMCPApproval || approval.Kind != contract.JSONArrayElement || approval.Key != "enabledMcpjsonServers" {
			t.Fatal("wrong MCP destinations")
		}
		var name string
		if err := json.Unmarshal(approval.Value, &name); err != nil || name != e.Artifact.Name {
			t.Fatal("invalid approval value")
		}
		var got map[string]any
		if err := json.Unmarshal(server.Value, &got); err != nil {
			t.Fatal(err)
		}
		d := e.Artifact.MCP
		want := map[string]any{}
		if d.Type == "stdio" {
			want["command"] = d.Command
			if len(d.Args) > 0 {
				args := []any{}
				for _, s := range d.Args {
					args = append(args, s)
				}
				want["args"] = args
			}
			if len(d.Env) > 0 {
				env := map[string]any{}
				for k, v := range d.Env {
					env[k] = v
				}
				want["env"] = env
			}
		} else {
			want["type"] = "http"
			want["url"] = d.URL
			if len(d.Headers) > 0 {
				h := map[string]any{}
				for k, v := range d.Headers {
					h[k] = v
				}
				want["headers"] = h
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("transport payload differs")
		}
		golden = append(golden, items...)
		// Returned JSON owns its storage, independently of inputs and later renders.
		again, err := a.Desired(e, p)
		if err != nil {
			t.Fatal(err)
		}
		again[0].Value[0] = '!'
		again[1].Value[0] = '!'
		fresh, err := a.Desired(e, p)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(fresh, items) || !reflect.DeepEqual(e, before) {
			t.Fatal("output aliases input or another render")
		}
	}
	data, err := json.Marshal(golden)
	if err != nil {
		t.Fatal(err)
	}
	testutil.AssertJSONGolden(t, "testdata/mcp-items.json", data)
}

func TestInvalidMCPInputs(t *testing.T) {
	f := testutil.NewFixture(t, false)
	sentinel := testutil.PlantSentinel(t, f)
	valid := mcp("docs", library.MCPDefinition{Type: "stdio", Command: "echo"})
	var cases []contract.Entry
	for _, name := range []string{"", "../escape", "a.b", "a/b", "a\\b", "Upper", strings.Repeat("a", 65), sentinel} {
		e := contract.CloneEntry(valid)
		e.Artifact.Name = name
		e.Ref = "mcp:" + name
		cases = append(cases, e)
	}
	for _, ref := range []string{"", "docs", "mcp:other", "skill:docs", sentinel} {
		e := contract.CloneEntry(valid)
		e.Ref = ref
		cases = append(cases, e)
	}
	e := contract.CloneEntry(valid)
	e.Artifact.MCP = nil
	cases = append(cases, e)
	for _, d := range []library.MCPDefinition{
		{}, {Type: "sse", URL: "https://example.invalid"}, {Type: "stdio", Command: " "}, {Type: "http"},
		{Type: "stdio", Command: "echo", URL: "https://example.invalid"}, {Type: "stdio", Command: "echo", Headers: map[string]string{}},
		{Type: "http", URL: "https://example.invalid", Args: []string{}}, {Type: "http", URL: "https://example.invalid", Env: map[string]string{}}, {Type: "http", URL: "https://example.invalid", Command: "echo"},
		{Type: "stdio", Command: "${BAD"}, {Type: "stdio", Command: "echo", Args: []string{"${BAD:-${NESTED}}"}},
		{Type: "stdio", Command: "echo", Env: map[string]string{"TOKEN": sentinel}},
		{Type: "http", URL: "https://example.invalid", Headers: map[string]string{"Authorization": sentinel}},
		{Type: "stdio", Command: "echo", Env: map[string]string{sentinel: "${BAD"}},
	} {
		cases = append(cases, mcp("docs", d))
	}
	before := testutil.SnapshotTree(t, f.Project)
	for n, e := range cases {
		items, err := claudecode.New().Desired(e, contract.Project{Root: f.Project})
		if err == nil || len(items) != 0 {
			t.Fatalf("case %d accepted", n)
		}
		testutil.AssertNoSecret(t, f.Project, sentinel, fmt.Sprint(err))
	}
	testutil.AssertTree(t, f.Project, before)
	// Warnings do not block valid definitions.
	if _, err := claudecode.New().Desired(mcp("warning", library.MCPDefinition{Type: "http", URL: "https://example.invalid?token=sk-warning"}), contract.Project{}); err != nil {
		t.Fatal("blocked warning")
	}
}

func TestMixedSkillsAgentsMCP(t *testing.T) {
	r, err := adapters.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Names(), []string{"claude-code"}) {
		t.Fatal("duplicate registration")
	}
	entries := []contract.Entry{skill("testing"), {Ref: "agent:reviewer", Artifact: library.Artifact{Type: library.Agent, Name: "reviewer"}, Content: []byte("native")}, mcp("docs", library.MCPDefinition{Type: "stdio", Command: "echo"})}
	got, err := r.Desired(entries, contract.Project{Harnesses: []string{"claude-code"}})
	if err != nil || len(got.Items) != 4 || len(got.Unsupported) != 0 {
		t.Fatalf("mixed: %v", err)
	}
}

func TestMCPEmptyOptionsOmitted(t *testing.T) {
	for _, d := range []library.MCPDefinition{
		{Type: "stdio", Command: "echo", Args: []string{}, Env: map[string]string{}},
		{Type: "http", URL: "https://example.invalid", Headers: map[string]string{}},
	} {
		withEmpty := renderMCP(t, "docs", d)
		d.Args, d.Env, d.Headers = nil, nil, nil
		without := renderMCP(t, "docs", d)
		if !reflect.DeepEqual(withEmpty, without) {
			t.Fatal("empty options rendered inconsistently")
		}
	}
}
