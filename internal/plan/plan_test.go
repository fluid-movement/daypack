package plan_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/plan"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func sample() plan.Plan {
	m := plan.PreviewMCP(library.MCPDefinition{Type: "stdio", Command: "npx", Args: []string{"--token", "${DAYPACK_TEST_SECRET}", "two words", ""}, Env: map[string]string{"TOKEN": "${DAYPACK_TEST_SECRET}", "OPTION": "${OPTION:-default}"}})
	return plan.Plan{Operations: []plan.Operation{
		{Target: "agents-skills-dir", Entry: "skill:svelte", Kind: lock.Symlink, Path: ".agents/skills/svelte", Link: "/library/skills/svelte", Action: plan.Create},
		{Target: "claude-mcp-json", Entry: "mcp:docs", Kind: lock.JSONEntry, Path: ".mcp.json", Key: "mcpServers.docs", Action: plan.Update, Gated: true, MCP: &m},
		{Target: "claude-mcp-approval", Entry: "mcp:docs", Kind: lock.JSONArrayElement, Path: ".claude/settings.local.json", Key: "enabledMcpjsonServers", Member: "docs", Action: plan.Adopt, Gated: true},
		{Target: "claude-agents-dir", Entry: "agent:reviewer", Kind: lock.File, Path: ".claude/agents/reviewer.md", Action: plan.Remove},
		{Target: "agents-md", Entry: "fragment:style", Kind: lock.MDSection, Path: "AGENTS.md", Marker: "daypack:style", Action: plan.Conflict, Reason: "existing content"},
		{Target: "agents-md", Entry: "fragment:other", Kind: lock.MDSection, Path: "AGENTS.md", Marker: "daypack:other", Action: plan.Drift},
		{Entry: "skill:missing", Action: plan.Unresolved, Reason: "entry missing"},
		{Harness: "pi", Entry: "agent:reviewer", Action: plan.Unsupported, Reason: "unsupported entry"},
	}, Notes: []contract.Note{{Harness: "pi", Code: "trust", Message: "Review project trust"}, {Harness: "claude", Code: "info", Message: "Shared target appears once"}}}
}
func rendered(t *testing.T, p plan.Plan) (string, string) {
	t.Helper()
	var text, data bytes.Buffer
	if err := plan.RenderText(&text, p); err != nil {
		t.Fatal(err)
	}
	if err := plan.RenderJSON(&data, p); err != nil {
		t.Fatal(err)
	}
	return text.String(), data.String()
}
func TestReports(t *testing.T) {
	p := sample()
	originalText, originalJSON := rendered(t, p)
	testutil.AssertGolden(t, "testdata/plan.txt", []byte(originalText))
	testutil.AssertJSONGolden(t, "testdata/plan.json", []byte(originalJSON))
	r, err := p.Report()
	if err != nil {
		t.Fatal(err)
	}
	if r.Summary != (plan.Summary{Create: 1, Update: 2, Remove: 1, Conflict: 1, Drift: 1, Unresolved: 2}) || r.Outcomes["planned"] != 8 {
		t.Fatalf("unexpected counts: %+v", r)
	}
	for _, o := range r.Operations {
		if o.Outcome != plan.Planned || o.Glyph == "" {
			t.Fatal("missing normalized fields")
		}
	}
	slices.Reverse(p.Operations)
	slices.Reverse(p.Notes)
	text, data := rendered(t, p)
	if text != originalText || data != originalJSON {
		t.Fatal("unstable normalization")
	}
	states := []plan.Outcome{plan.Applied, plan.Skipped, plan.Failed, plan.Unchanged}
	for i := range p.Operations {
		p.Operations[i].Outcome = states[i%len(states)]
		p.Operations[i].Reason = "safe final reason"
	}
	text, data = rendered(t, p)
	testutil.AssertGolden(t, "testdata/result.txt", []byte(text))
	testutil.AssertJSONGolden(t, "testdata/result.json", []byte(data))
	r, err = p.Report()
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if r.Outcomes[string(state)] != 2 {
			t.Fatal("wrong final counts")
		}
	}
	var decoded plan.Report
	dec := json.NewDecoder(strings.NewReader(data))
	if err := dec.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatal("extra document")
	}
	if !reflect.DeepEqual(r, decoded) || !strings.HasSuffix(data, "\n") {
		t.Fatal("JSON projection differs")
	}
	marshaled, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(marshaled)+"\n" != data {
		t.Fatal("marshal bypassed projection")
	}
}
func TestNormalizationIsolation(t *testing.T) {
	p := sample()
	before, _ := rendered(t, p)
	n, err := plan.Normalize(p)
	if err != nil {
		t.Fatal(err)
	}
	n.Notes[0].Message = "changed"
	for i := range n.Operations {
		if n.Operations[i].MCP != nil {
			n.Operations[i].MCP.Args[0] = "changed"
			n.Operations[i].MCP.RequiredVariables[0] = "changed"
		}
	}
	after, _ := rendered(t, p)
	if before != after || p.Operations[0].Outcome != "" {
		t.Fatal("mutated caller")
	}
	d := library.MCPDefinition{Args: []string{"original"}}
	m := plan.PreviewMCP(d)
	m.Args[0] = "changed"
	if d.Args[0] != "original" {
		t.Fatal("preview aliases input")
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("private writer payload") }
func TestInvalidAndWriters(t *testing.T) {
	for _, change := range []func(*plan.Operation){
		func(o *plan.Operation) { o.Action = "private payload" }, func(o *plan.Operation) { o.Outcome = "private payload" }, func(o *plan.Operation) { o.Entry = "private payload" }, func(o *plan.Operation) { o.Kind = "private payload" }, func(o *plan.Operation) { o.Path = "" }, func(o *plan.Operation) { o.Link = "" },
	} {
		p := sample()
		change(&p.Operations[0])
		var out bytes.Buffer
		for _, render := range []func(io.Writer, plan.Plan) error{plan.RenderText, plan.RenderJSON} {
			err := render(&out, p)
			if err == nil || strings.Contains(err.Error(), "private payload") || out.Len() != 0 {
				t.Fatal("unsafe validation")
			}
		}
	}
	for _, render := range []func(io.Writer, plan.Plan) error{plan.RenderText, plan.RenderJSON} {
		if err := render(brokenWriter{}, sample()); err == nil {
			t.Fatal("writer error lost")
		}
	}
	text, data := rendered(t, plan.Plan{})
	testutil.AssertGolden(t, "testdata/empty.txt", []byte(text))
	testutil.AssertJSONGolden(t, "testdata/empty.json", []byte(data))
	if !strings.Contains(text, "No changes.") || !strings.Contains(data, `"operations":[]`) || !strings.Contains(data, `"notes":[]`) {
		t.Fatal("empty report")
	}
	p := sample()
	p.Operations[0].Entry = "skill:bad\n\t\r\x1b"
	p.Operations[0].Path = "bad\npath"
	p.Notes[0].Message = "bad\n\x1b"
	text, _ = rendered(t, p)
	if strings.ContainsAny(text, "\t\r\x1b") || strings.Contains(text, "bad\n") {
		t.Fatal("raw control characters")
	}
}
func TestGatedInlineMCPPlan(t *testing.T) {
	base := library.MCPDefinition{Type: "stdio", Command: "npx", Args: []string{"a", "b"}, Env: map[string]string{"TOKEN": "${TOKEN}"}}
	e := contract.Entry{Inline: true, Artifact: library.Artifact{Type: library.MCP, MCP: &base}}
	if !plan.NeedsConfirmation(e, nil) || plan.NeedsConfirmation(e, &base) {
		t.Fatal("first apply / verified history")
	}
	for _, change := range []func(*library.MCPDefinition){
		func(d *library.MCPDefinition) { d.Command = "other" }, func(d *library.MCPDefinition) { d.Args = []string{"b", "a"} }, func(d *library.MCPDefinition) { d.Args = []string{"a"} }, func(d *library.MCPDefinition) { d.Args = []string{"a", "c"} }, func(d *library.MCPDefinition) { d.URL = "${NEW_URL}" }, func(d *library.MCPDefinition) { d.Type = "http" }, func(d *library.MCPDefinition) { d.Args = []string{"${NEW_REFERENCE}"} },
	} {
		previous := base
		change(&previous)
		if !plan.NeedsConfirmation(e, &previous) {
			t.Fatal("identity change not gated")
		}
	}
	previous := base
	previous.Description = "description"
	previous.Env = map[string]string{"TOKEN": "${OTHER}"}
	previous.Headers = map[string]string{"Authorization": "${HEADER}"}
	if plan.NeedsConfirmation(e, &previous) {
		t.Fatal("non-executable change gated")
	}
	e.Inline = false
	if plan.NeedsConfirmation(e, nil) || plan.NeedsConfirmation(e, &previous) {
		t.Fatal("library trust")
	}
	e.Inline = true
	e.Artifact.Type = library.Skill
	if plan.NeedsConfirmation(e, nil) {
		t.Fatal("non MCP")
	}
	e.Artifact.Type = library.MCP
	empty := base
	empty.Args = nil
	other := empty
	other.Args = []string{}
	e.Artifact.MCP = &empty
	if plan.NeedsConfirmation(e, &other) {
		t.Fatal("nil args differ from empty")
	}
	e.Artifact.MCP = nil
	if !plan.NeedsConfirmation(e, &base) {
		t.Fatal("unverifiable definition must gate")
	}
}
func TestGatePolicy(t *testing.T) {
	for _, gates := range []bool{false, true} {
		for _, interactive := range []bool{false, true} {
			for _, yes := range []bool{false, true} {
				for _, dry := range []bool{false, true} {
					p := sample()
					for i := range p.Operations {
						p.Operations[i].Gated = gates
					}
					want := plan.None
					if gates {
						switch {
						case dry:
							want = plan.Preview
						case yes:
						case interactive:
							want = plan.Prompt
						default:
							want = plan.Skip
						}
					}
					if got := plan.ConfirmationMode(p, interactive, yes, dry); got != want {
						t.Fatalf("mode=%s want=%s", got, want)
					}
				}
			}
		}
	}
	p := sample()
	for i := range p.Operations {
		p.Operations[i].Gated = true
	}
	skipped := plan.SkipGated(p, "safe skip")
	for i, o := range skipped.Operations {
		candidate := o.Action == plan.Create || o.Action == plan.Update || o.Action == plan.Adopt
		if (o.Outcome == plan.Skipped) != candidate || !o.Gated || p.Operations[i].Outcome != "" {
			t.Fatal("incorrect skipping")
		}
	}
	if !reflect.DeepEqual(plan.GatedEntries(sample()), []string{"mcp:docs"}) {
		t.Fatal("duplicate approval prompt")
	}
}
func TestSecretSafetyAndNoWrites(t *testing.T) {
	f := testutil.NewFixture(t, false)
	sentinel := testutil.PlantSentinel(t, f)
	lib, err := library.Load(f.Library)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := lib.Resolve("mcp:docs")
	if err != nil {
		t.Fatal(err)
	}
	projectBefore := testutil.SnapshotTree(t, f.Project)
	libraryBefore := testutil.SnapshotTree(t, f.Library)
	preview := plan.PreviewMCP(*artifact.MCP)
	p := sample()
	p.Operations[1].MCP = &preview
	http := plan.PreviewMCP(library.MCPDefinition{Type: "http", URL: "https://example.test/${HOST:-default}/${DAYPACK_TEST_SECRET}", Headers: map[string]string{"Authorization": "Bearer ${HEADER_SECRET}", "X-Optional": "${OPTIONAL:-fallback}"}})
	p.Operations = append(p.Operations, plan.Operation{Target: "pi-mcp-json", Entry: "mcp:http", Kind: lock.JSONEntry, Path: ".pi/mcp.json", Key: "mcpServers.http", Action: plan.Create, MCP: &http})
	text, data := rendered(t, p)
	if !strings.Contains(data, "${DAYPACK_TEST_SECRET}") || !reflect.DeepEqual(preview.RequiredVariables, []string{"DAYPACK_TEST_SECRET"}) || !reflect.DeepEqual(http.RequiredVariables, []string{"DAYPACK_TEST_SECRET", "HEADER_SECRET"}) {
		t.Fatal("raw reference/variable mismatch")
	}
	if strings.Contains(data, "Authorization") || strings.Contains(data, "Bearer") || strings.Contains(data, `"env"`) {
		t.Fatal("private maps retained")
	}
	testutil.AssertNoSecret(t, f.Project, sentinel, text, data)
	testutil.AssertTree(t, f.Project, projectBefore)
	testutil.AssertTree(t, f.Library, libraryBefore)
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestShortWritesAndNonCandidateGates(t *testing.T) {
	for _, render := range []func(io.Writer, plan.Plan) error{plan.RenderText, plan.RenderJSON} {
		if err := render(shortWriter{}, sample()); !errors.Is(err, io.ErrShortWrite) {
			t.Fatal("short write lost")
		}
	}
	for _, action := range []plan.Action{plan.Remove, plan.Conflict, plan.Drift, plan.Unresolved, plan.Unsupported} {
		p := sample()
		p.Operations = p.Operations[:1]
		p.Operations[0].Action = action
		p.Operations[0].Gated = true
		if plan.ConfirmationMode(p, true, false, false) != plan.None {
			t.Fatal("non-candidate prompted")
		}
	}
	for _, outcome := range []plan.Outcome{plan.Applied, plan.Skipped, plan.Failed, plan.Unchanged} {
		p := sample()
		p.Operations = p.Operations[:1]
		p.Operations[0].Gated = true
		p.Operations[0].Outcome = outcome
		if plan.ConfirmationMode(p, true, false, false) != plan.None {
			t.Fatal("completed operation prompted")
		}
	}
}

func TestTOMLTablePlanSerialization(t *testing.T) {
	p := plan.Plan{Operations: []plan.Operation{{Target: contract.CodexConfigTOML, Entry: "mcp:docs", Kind: lock.TOMLTable, Path: ".codex/config.toml", Key: "mcp_servers.docs", Action: plan.Create}}}
	text, data := rendered(t, p)
	if !strings.Contains(text, ".codex/config.toml") || !strings.Contains(data, `"kind":"toml-table"`) || !strings.Contains(data, `"key":"mcp_servers.docs"`) {
		t.Fatal(text, data)
	}
	p.Operations[0].Key = ""
	if _, err := plan.Normalize(p); err == nil {
		t.Fatal("missing TOML key accepted")
	}
}
