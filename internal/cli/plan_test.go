package cli_test

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/cli"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/plan"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func gatedPlan() plan.Plan {
	m := plan.PreviewMCP(library.MCPDefinition{Type: "stdio", Command: "server", Args: []string{"${DAYPACK_TEST_SECRET}"}, Env: map[string]string{"TOKEN": "${DAYPACK_TEST_SECRET}"}})
	return plan.Plan{Operations: []plan.Operation{
		{Target: "mcp", Entry: "mcp:docs", Kind: lock.JSONEntry, Path: ".mcp.json", Key: "mcpServers.docs", Action: plan.Create, Gated: true, MCP: &m},
		{Target: "approval", Entry: "mcp:docs", Kind: lock.JSONArrayElement, Path: ".claude/settings.local.json", Key: "enabledMcpjsonServers", Member: "docs", Action: plan.Create, Gated: true},
		{Target: "skills", Entry: "skill:example", Kind: lock.Symlink, Path: ".agents/skills/example", Link: "/library/example", Action: plan.Create},
	}}
}

type failingInput struct{}

func (failingInput) Read([]byte) (int, error) { return 0, errors.New("private read payload") }

type failingOutput struct{}

func (failingOutput) Write([]byte) (int, error) { return 0, errors.New("private write payload") }

type unreadable struct{ t *testing.T }

func (r unreadable) Read([]byte) (int, error) { r.t.Fatal("unexpected stdin read"); return 0, io.EOF }
func TestConfirmation(t *testing.T) {
	cases := []struct {
		name, input                                  string
		interactive, json, yes, dry, approve, prompt bool
	}{
		{name: "approve", input: "y\n", interactive: true, approve: true, prompt: true},
		{name: "case insensitive", input: "YES\n", interactive: true, approve: true, prompt: true},
		{name: "refuse", input: "no\n", interactive: true, prompt: true},
		{name: "empty", input: "\n", interactive: true, prompt: true},
		{name: "EOF", interactive: true, prompt: true},
		{name: "unterminated approval defaults no", input: "yes", interactive: true, prompt: true},
		{name: "yes", interactive: true, yes: true, approve: true},
		{name: "non TTY"},
		{name: "JSON overrides TTY", interactive: true, json: true},
		{name: "dry run", interactive: true, dry: true, approve: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			var input io.Reader = unreadable{t}
			if tc.prompt {
				input = strings.NewReader(tc.input)
			}
			p := gatedPlan()
			got, err := cli.ConfirmPlan(p, cli.Options{Interactive: tc.interactive, JSON: tc.json}, tc.yes, tc.dry, cli.IO{In: input, Err: &stderr})
			if err != nil {
				t.Fatal(err)
			}
			if (stderr.Len() > 0) != tc.prompt || (tc.prompt && strings.Count(stderr.String(), "mcp:docs") != 1) {
				t.Fatal("incorrect prompt")
			}
			for _, o := range got.Operations {
				if o.Gated && ((o.Outcome == plan.Skipped) == tc.approve) {
					t.Fatal("incorrect consent result")
				}
				if !o.Gated && o.Outcome != plan.Planned {
					t.Fatal("non-gated operation lost")
				}
			}
			if p.Operations[0].Outcome != "" {
				t.Fatal("caller mutated")
			}
		})
	}
	p := gatedPlan()
	for i := range p.Operations {
		p.Operations[i].Gated = false
	}
	if _, err := cli.ConfirmPlan(p, cli.Options{Interactive: true}, false, false, cli.IO{In: unreadable{t}, Err: failingOutput{}}); err != nil {
		t.Fatal(err)
	}
	for _, streams := range []cli.IO{{In: failingInput{}, Err: io.Discard}, {In: unreadable{t}, Err: failingOutput{}}} {
		_, err := cli.ConfirmPlan(gatedPlan(), cli.Options{Interactive: true}, false, false, streams)
		if err == nil || strings.Contains(err.Error(), "private") {
			t.Fatal("unsafe or missing I/O error")
		}
	}
	p = gatedPlan()
	p.Operations[0].Action = "invalid"
	if _, err := cli.ConfirmPlan(p, cli.Options{}, false, false, cli.IO{}); err == nil {
		t.Fatal("invalid plan accepted")
	}
	if _, err := cli.PlanResult(p, cli.Success); err == nil {
		t.Fatal("invalid result accepted")
	}
}
func TestProjectionAndConfirmationNoSecrets(t *testing.T) {
	f := testutil.NewFixture(t, false)
	sentinel := testutil.PlantSentinel(t, f)
	beforeProject := testutil.SnapshotTree(t, f.Project)
	beforeLibrary := testutil.SnapshotTree(t, f.Library)
	for _, input := range []io.Reader{strings.NewReader("yes\n"), strings.NewReader("no\n"), failingInput{}} {
		var prompt bytes.Buffer
		p, err := cli.ConfirmPlan(gatedPlan(), cli.Options{Interactive: true}, false, false, cli.IO{In: input, Err: &prompt})
		safeErr := ""
		if err != nil {
			safeErr = err.Error()
		}
		var text, data bytes.Buffer
		if err == nil {
			if err := plan.RenderText(&text, p); err != nil {
				t.Fatal(err)
			}
			if err := plan.RenderJSON(&data, p); err != nil {
				t.Fatal(err)
			}
			report, err := p.Report()
			if err != nil {
				t.Fatal(err)
			}
			result, err := cli.PlanResult(p, cli.Attention)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result.Operations, report.Operations) || result.Code != cli.Attention {
				t.Fatal("different CLI projection")
			}
		}
		testutil.AssertNoSecret(t, f.Project, sentinel, prompt.String(), text.String(), data.String(), safeErr)
	}
	testutil.AssertTree(t, f.Project, beforeProject)
	testutil.AssertTree(t, f.Library, beforeLibrary)
}
