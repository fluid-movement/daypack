package codex_test

import (
	"bytes"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/claudecode"
	"github.com/fluid-movement/daypack/internal/adapters/codex"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func mcp(d library.MCPDefinition) contract.Entry {
	return contract.Entry{Ref: "mcp:docs", Artifact: library.Artifact{Type: library.MCP, Name: "docs", MCP: &d}}
}
func agent(content string) contract.Entry {
	return contract.Entry{Ref: "agent:reviewer", Artifact: library.Artifact{Type: library.Agent, Name: "reviewer"}, Content: []byte(content)}
}
func decode(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if _, err := toml.Decode(string(data), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestNativeMCPGoldensAndConformance(t *testing.T) {
	p := contract.Project{Harnesses: []string{"codex"}}
	cases := []struct {
		name       string
		definition library.MCPDefinition
		want       map[string]any
	}{
		{"stdio", library.MCPDefinition{Type: "stdio", Description: "library only", Command: "echo", Args: []string{"hello", "quoted \" value"}, Env: map[string]string{"TOKEN": "${TOKEN}", "MODE": "offline"}}, map[string]any{"command": "echo", "args": []any{"hello", "quoted \" value"}, "env_vars": []any{"TOKEN"}, "env": map[string]any{"MODE": "offline"}}},
		{"http", library.MCPDefinition{Type: "http", Description: "library only", URL: "https://example.invalid/mcp", Headers: map[string]string{"Authorization": "Bearer ${TOKEN}", "X-Region": "eu", "X-Custom": "${CUSTOM}"}}, map[string]any{"url": "https://example.invalid/mcp", "bearer_token_env_var": "TOKEN", "http_headers": map[string]any{"X-Region": "eu"}, "env_http_headers": map[string]any{"X-Custom": "CUSTOM"}}},
		{"whole-authorization", library.MCPDefinition{Type: "http", URL: "https://example.invalid", Headers: map[string]string{"authorization": "${AUTH}"}}, map[string]any{"url": "https://example.invalid", "env_http_headers": map[string]any{"authorization": "AUTH"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := mcp(tc.definition)
			testadapter.CheckAdapter(t, codex.New(), []testadapter.Case{{Entry: e, Project: p, Supported: true}})
			items, err := codex.New().Desired(e, p)
			if err != nil || len(items) != 1 {
				t.Fatal(items, err)
			}
			i := items[0]
			if i.Kind != contract.TOMLTable || i.Key != "mcp_servers.docs" || i.Path != ".codex/config.toml" {
				t.Fatal(i)
			}
			if got := decode(t, i.Value); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("native mapping: %#v", got)
			}
			native, err := lock.SetTOML(nil, lock.Item{Kind: lock.TOMLTable, Key: i.Key}, i.Value)
			if err != nil {
				t.Fatal(err)
			}
			servers := decode(t, native)["mcp_servers"].(map[string]any)
			if !reflect.DeepEqual(servers["docs"], tc.want) {
				t.Fatal("native table mismatch")
			}
			testutil.AssertGolden(t, filepath.Join("testdata", tc.name+".toml"), native)
		})
	}
}
func TestUnsupportedReferencesAreFieldOnlyAndAtomic(t *testing.T) {
	cases := []struct {
		name   string
		d      library.MCPDefinition
		fields []string
	}{
		{"rename", library.MCPDefinition{Type: "stdio", Command: "echo", Env: map[string]string{"TOKEN": "${OTHER}"}}, []string{"env.TOKEN"}},
		{"defaults", library.MCPDefinition{Type: "stdio", Command: "echo", Env: map[string]string{"TOKEN": "${TOKEN:-secret-default}"}}, []string{"env.TOKEN"}},
		{"composition", library.MCPDefinition{Type: "stdio", Command: "echo", Env: map[string]string{"MODE": "prefix-${MODE}"}}, []string{"env.MODE"}},
		{"bare-secret-key", library.MCPDefinition{Type: "stdio", Command: "echo", Env: map[string]string{"TOKEN": "$TOKEN"}}, []string{"env.TOKEN"}},
		{"bare-authorization", library.MCPDefinition{Type: "http", URL: "https://example.invalid", Headers: map[string]string{"Authorization": "Bearer $TOKEN"}}, []string{"headers.Authorization"}},
		{"bare", library.MCPDefinition{Type: "stdio", Command: "echo", Env: map[string]string{"MODE": "$MODE"}}, []string{"env.MODE"}},
		{"command-args", library.MCPDefinition{Type: "stdio", Command: "${COMMAND}", Args: []string{"$ARG", "${ARG}", "prefix-${ARG}"}}, []string{"args[0]", "args[1]", "args[2]", "command"}},
		{"url", library.MCPDefinition{Type: "http", URL: "https://${HOST}/mcp"}, []string{"url"}},
		{"header-default", library.MCPDefinition{Type: "http", URL: "https://example.invalid", Headers: map[string]string{"Authorization": "Bearer ${TOKEN:-secret-default}"}}, []string{"headers.Authorization"}},
		{"header-compose", library.MCPDefinition{Type: "http", URL: "https://example.invalid", Headers: map[string]string{"X-Custom": "Basic ${TOKEN}", "X-Other": "$OTHER"}}, []string{"headers.X-Custom", "headers.X-Other"}},
		{"ambiguous-auth", library.MCPDefinition{Type: "http", URL: "https://example.invalid", Headers: map[string]string{"Authorization": "Bearer ${TOKEN}", "authorization": "${AUTH}"}}, []string{"headers.Authorization", "headers.authorization"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, err := codex.New().Desired(mcp(tc.d), contract.Project{})
			var u *contract.UnsupportedError
			if len(items) != 0 || !errors.As(err, &u) || !reflect.DeepEqual(u.Fields, tc.fields) {
				t.Fatalf("%v %#v", err, u)
			}
			if strings.Contains(err.Error(), "secret-default") {
				t.Fatal("value in diagnostic")
			}
			r, err := adapters.DefaultRegistry()
			if err != nil {
				t.Fatal(err)
			}
			result, err := r.Desired([]contract.Entry{mcp(tc.d)}, contract.Project{Harnesses: []string{"codex"}})
			if err != nil || len(result.Items) != 0 || len(result.Unsupported) != 1 || !reflect.DeepEqual(result.Unsupported[0].Fields, tc.fields) {
				t.Fatal(result, err)
			}
		})
	}
}
func TestAgentTranslationAndMetadata(t *testing.T) {
	content := "---\nname: reviewer\ndescription: 'Review \"quotes\" and Unicode ☃'\nmodel: explicit-model\n---\nLine one.\n\"\"\"\nline two \\ path\n"
	e := agent(content)
	testadapter.CheckAdapter(t, codex.New(), []testadapter.Case{{Entry: e, Supported: true}})
	items, err := codex.New().Desired(e, contract.Project{})
	if err != nil || len(items) != 1 {
		t.Fatal(err)
	}
	value := decode(t, items[0].Bytes)
	want := map[string]any{"name": "reviewer", "description": "Review \"quotes\" and Unicode ☃", "model": "explicit-model", "developer_instructions": "Line one.\n\"\"\"\nline two \\ path\n"}
	if !reflect.DeepEqual(value, want) {
		t.Fatal(value)
	}
	if items[0].Path != ".codex/agents/reviewer.toml" || items[0].Kind != contract.File {
		t.Fatal(items[0])
	}
	testutil.AssertGolden(t, "testdata/agent.toml", items[0].Bytes)
	claude, err := claudecode.New().Desired(e, contract.Project{})
	if err != nil || !bytes.Equal(claude[0].Bytes, e.Content) {
		t.Fatal("Claude Markdown changed", err)
	}
	e = agent(strings.ReplaceAll(content, "model: explicit-model\n", "tools: []\npermission: ''\n"))
	items, err = codex.New().Desired(e, contract.Project{})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := decode(t, items[0].Bytes)["model"]; exists {
		t.Fatal("absent model should inherit")
	}
	e = agent(strings.ReplaceAll(content, "model: explicit-model", "tools: [Read]\npermission: parent"))
	items, err = codex.New().Desired(e, contract.Project{})
	var unsupported *contract.UnsupportedError
	if len(items) != 0 || !errors.As(err, &unsupported) || !reflect.DeepEqual(unsupported.Fields, []string{"frontmatter.permission", "frontmatter.tools"}) {
		t.Fatal(err, unsupported)
	}
}
func TestSharedResourcesNoBridge(t *testing.T) {
	r, err := adapters.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	entries := []contract.Entry{
		{Ref: "skill:testing", Artifact: library.Artifact{Type: library.Skill, Name: "testing", Path: "/nonexistent/library/skills/testing"}},
		{Ref: "fragment:conventions", Artifact: library.Artifact{Type: library.Fragment, Name: "conventions"}, Content: []byte("Use tests.\n")},
	}
	for _, selection := range [][]string{{"codex"}, {"codex", "pi"}, {"claude-code", "codex", "pi"}} {
		result, err := r.Desired(entries, contract.Project{Harnesses: selection})
		if err != nil {
			t.Fatal(err)
		}
		counts := map[string]int{}
		for _, i := range result.Items {
			counts[i.Target]++
		}
		if counts[contract.AgentsMD] != 1 || counts[contract.AgentsSkillsDir] != 1 {
			t.Fatal(counts)
		}
		claude := slicesContains(selection, "claude-code")
		if (counts[contract.ClaudeMD] > 0) != claude {
			t.Fatal("unexpected bridge", counts)
		}
	}
}
func slicesContains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
