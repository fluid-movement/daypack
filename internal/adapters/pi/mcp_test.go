package pi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/pi"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func mcpEntry(d library.MCPDefinition) contract.Entry {
	return contract.Entry{Ref: "mcp:docs", Artifact: library.Artifact{Type: library.MCP, Name: "docs", MCP: &d}}
}
func TestMCPItems(t *testing.T) {
	f := testutil.NewFixture(t, false)
	sentinel := testutil.PlantSentinel(t, f)
	before := testutil.SnapshotTree(t, f.Project)
	libraryBefore := testutil.SnapshotTree(t, f.Library)
	cases := []library.MCPDefinition{
		{Type: "stdio", Command: "echo", Args: []string{"second", "first"}, Env: map[string]string{"TOKEN": "${DAYPACK_TEST_SECRET}", "MODE": "offline"}, Description: "Local docs"},
		{Type: "http", URL: "https://example.invalid/mcp", Headers: map[string]string{"Authorization": "Bearer ${DAYPACK_TEST_SECRET}"}, Description: "Remote docs"},
		{Type: "stdio", Command: "echo"},
		{Type: "http", URL: "https://example.invalid/mcp"},
	}
	type projection struct {
		Item   contract.Item
		Server json.RawMessage
	}
	var golden []projection
	for _, d := range cases {
		e := mcpEntry(d)
		testadapter.CheckAdapter(t, pi.New(), []testadapter.Case{{Entry: e, Project: contract.Project{Harnesses: []string{"pi"}}, Supported: true}})
		items, err := pi.New().Desired(e, contract.Project{})
		if err != nil || len(items) != 1 {
			t.Fatal("render failed")
		}
		i := items[0]
		if i.Target != contract.PiMCPJSON || i.Entry != e.Ref || i.Kind != contract.JSONEntry || i.Path != ".pi/mcp.json" || i.Key != "mcpServers.docs" || i.Link != "" || i.Marker != "" || len(i.Bytes) != 0 {
			t.Fatal("incorrect item identity")
		}
		data := bytes.Clone(i.Value)
		// Caller mutation after a call cannot change the encoded output.
		if len(d.Args) > 0 {
			d.Args[0] = "changed"
		}
		if d.Env != nil {
			d.Env["MODE"] = "changed"
		}
		if d.Headers != nil {
			d.Headers["Extra"] = "changed"
		}
		if !bytes.Equal(data, i.Value) {
			t.Fatal("output aliases input")
		}
		i.Value = nil
		golden = append(golden, projection{i, data})
	}
	encoded, err := json.Marshal(golden)
	if err != nil {
		t.Fatal(err)
	}
	testutil.AssertNoSecret(t, f.Project, sentinel, string(encoded))
	testutil.AssertJSONGolden(t, "testdata/mcp-items.json", encoded)
	testutil.AssertTree(t, f.Project, before)
	testutil.AssertTree(t, f.Library, libraryBefore)
	unsupported := contract.Entry{Artifact: library.Artifact{Type: library.Agent}}
	if items, err := pi.New().Desired(unsupported, contract.Project{}); err != nil || len(items) != 0 {
		t.Fatal("unsupported type rendered")
	}
}

func TestMCPCapabilities(t *testing.T) {
	cases := []struct {
		name   string
		d      library.MCPDefinition
		fields []string
	}{
		{"command", library.MCPDefinition{Type: "stdio", Command: "${COMMAND}"}, []string{"command"}},
		{"bare-command", library.MCPDefinition{Type: "stdio", Command: "$COMMAND"}, []string{"command"}},
		{"url", library.MCPDefinition{Type: "http", URL: "https://${HOST}/mcp"}, []string{"url"}},
		{"bare-url", library.MCPDefinition{Type: "http", URL: "https://$HOST/mcp"}, []string{"url"}},
		{"args", library.MCPDefinition{Type: "stdio", Command: "echo", Args: []string{"${ARG}", "$ARG"}}, []string{"args[0]", "args[1]"}},
		{"many", library.MCPDefinition{Type: "stdio", Command: "${CMD}", Args: []string{"$A", "${B:-value}"}, Env: map[string]string{"Z": "${Z:-}", "A": "${A:-fallback}"}}, []string{"args[0]", "args[1]", "command", "env.A", "env.Z"}},
		{"header-default", library.MCPDefinition{Type: "http", URL: "https://example.invalid", Headers: map[string]string{"Authorization": "Bearer ${TOKEN:-fallback}"}}, []string{"headers.Authorization"}},
		{"simple-env", library.MCPDefinition{Type: "stdio", Command: "echo", Env: map[string]string{"TOKEN": "${TOKEN}", "MODE": "$MODE"}}, nil},
		{"simple-header", library.MCPDefinition{Type: "http", URL: "https://example.invalid", Headers: map[string]string{"Authorization": "Bearer ${TOKEN}"}}, nil},
		{"escaped", library.MCPDefinition{Type: "stdio", Command: "$$COMMAND", Args: []string{"$${ARG}", "$$ARG", "$!", "$5", "cost $.", "$"}, Env: map[string]string{"MODE": "$${VAR:-literal}"}}, nil},
		{"escape-then-ref", library.MCPDefinition{Type: "stdio", Command: "echo", Args: []string{"$$${ARG}"}}, []string{"args[0]"}},
		{"literal-helper", library.MCPDefinition{Type: "http", URL: "https://example.invalid", Headers: map[string]string{"X-Mode": "!not-executed"}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, err := pi.New().Desired(mcpEntry(tc.d), contract.Project{})
			if tc.fields == nil {
				if err != nil || len(items) != 1 {
					t.Fatal("supported case rejected")
				}
				// Compare decoded values: JSON escaping must not rewrite references,
				// literal dollars, helpers, maps or ordered arguments.
				var decoded struct {
					Command string
					URL     string
					Args    []string
					Env     map[string]string
					Headers map[string]string
				}
				if err := json.Unmarshal(items[0].Value, &decoded); err != nil {
					t.Fatal(err)
				}
				if decoded.Command != tc.d.Command || decoded.URL != tc.d.URL || !reflect.DeepEqual(decoded.Args, tc.d.Args) || !reflect.DeepEqual(decoded.Env, tc.d.Env) || !reflect.DeepEqual(decoded.Headers, tc.d.Headers) {
					t.Fatal("decoded values changed")
				}
				return
			}
			var unsupported *contract.UnsupportedError
			if !errors.As(err, &unsupported) || !reflect.DeepEqual(unsupported.Fields, tc.fields) || len(items) != 0 {
				t.Fatalf("capability mismatch: %v", err)
			}
			if err.Error() != "unsupported adapter fields" {
				t.Fatal("unsafe diagnostic")
			}
		})
	}
}

func TestInvalidMCP(t *testing.T) {
	valid := mcpEntry(library.MCPDefinition{Type: "stdio", Command: "echo"})
	for _, modify := range []func(*contract.Entry){
		func(e *contract.Entry) { e.Artifact.MCP = nil },
		func(e *contract.Entry) { e.Artifact.Name = "../private" },
		func(e *contract.Entry) { e.Ref = "mcp:other" },
		func(e *contract.Entry) { e.Artifact.MCP.Type = "private-value" },
		func(e *contract.Entry) { e.Artifact.MCP.Command = "" },
		func(e *contract.Entry) { e.Artifact.MCP.URL = "private-value" },
		func(e *contract.Entry) { e.Artifact.MCP.Env = map[string]string{"TOKEN": "private-value"} },
		func(e *contract.Entry) { e.Artifact.MCP.Args = []string{"${broken"} },
	} {
		e := contract.CloneEntry(valid)
		modify(&e)
		items, err := pi.New().Desired(e, contract.Project{})
		var unsupported *contract.UnsupportedError
		if err == nil || errors.As(err, &unsupported) || len(items) != 0 || bytes.Contains([]byte(err.Error()), []byte("private-value")) {
			t.Fatal("invalid input not safely rejected")
		}
	}
	// Warnings alone must not reject a valid definition.
	e := mcpEntry(library.MCPDefinition{Type: "stdio", Command: "echo", Args: []string{"ghp_synthetic-warning"}})
	if _, err := pi.New().Desired(e, contract.Project{}); err != nil {
		t.Fatal("warning rejected")
	}
}
