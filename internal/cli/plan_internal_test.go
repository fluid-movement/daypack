package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/plan"
	"github.com/spf13/cobra"
)

func testPlanCommand(streams IO) func(*cobra.Command) {
	return func(root *cobra.Command) {
		cmd := &cobra.Command{Use: "report-test"}
		var yes, dry bool
		cmd.Flags().BoolVar(&yes, "yes", false, "")
		cmd.Flags().BoolVar(&dry, "dry-run", false, "")
		SetHandler(cmd, func(_ context.Context, options Options, _ []string) (Result, error) {
			p := plan.Plan{Operations: []plan.Operation{
				{Target: "mcp", Entry: "mcp:docs", Kind: lock.JSONEntry, Path: ".mcp.json", Key: "mcpServers.docs", Action: plan.Create, Gated: true},
				{Target: "skills", Entry: "skill:test", Kind: lock.Symlink, Path: ".agents/skills/test", Link: "/library/test", Action: plan.Create},
			}}
			p, err := ConfirmPlan(p, options, yes, dry, streams)
			if err != nil {
				return Result{}, err
			}
			code := Success
			for _, o := range p.Operations {
				if o.Outcome == plan.Skipped {
					code = Attention
				}
			}
			return PlanResult(p, code)
		})
		root.AddCommand(cmd)
	}
}
func TestPlanEnvelopeAndDispatch(t *testing.T) {
	environment(t)
	for _, tc := range []struct {
		args        []string
		code        int
		json, quiet bool
	}{
		{[]string{"report-test", "--json"}, Attention, true, false},
		{[]string{"report-test", "--json", "--quiet"}, Attention, true, true},
		{[]string{"report-test", "--yes", "--json"}, Success, true, false},
		{[]string{"report-test", "--dry-run", "--json"}, Success, true, false},
		{[]string{"report-test", "--no-color"}, Attention, false, false},
		{[]string{"report-test", "--quiet"}, Attention, false, true},
	} {
		var out, stderr bytes.Buffer
		streams := IO{In: unreadableInput{t}, Out: &out, Err: &stderr, IsTerminal: func(io.Reader) bool { return tc.json }}
		code := run(context.Background(), tc.args, streams, testPlanCommand(streams))
		if code != tc.code || stderr.Len() != 0 {
			t.Fatalf("code %d: %s", code, stderr.String())
		}
		if tc.json {
			value := object(t, out.String(), code, "daypack report-test")
			operations, ok := value["operations"].([]any)
			if !ok || len(operations) != 2 {
				t.Fatal("nested or missing operations")
			}
			if value["summary"] == nil || value["outcomes"] == nil || value["notes"] == nil {
				t.Fatal("missing typed report fields")
			}
			row := operations[0].(map[string]any)
			if row["gated"] != true || row["marker"] != "+" {
				t.Fatal("lost gate/marker")
			}
		} else if tc.quiet {
			if out.Len() != 0 {
				t.Fatal("quiet plan chatter")
			}
		} else {
			if !strings.Contains(out.String(), "Summary:") || !strings.Contains(out.String(), "skipped") || strings.Contains(out.String(), "\x1b") {
				t.Fatal("missing text dispatch")
			}
		}
	}
}

type unreadableInput struct{ t *testing.T }

func (r unreadableInput) Read([]byte) (int, error) {
	r.t.Fatal("unexpected input read")
	return 0, io.EOF
}
func TestPlanIOFailures(t *testing.T) {
	environment(t)
	for _, tc := range []struct {
		in          io.Reader
		out, stderr io.Writer
		terminal    bool
	}{
		{strings.NewReader(""), badWriter{}, io.Discard, false},
		{badReader{}, io.Discard, io.Discard, true},
		{unreadableInput{t}, io.Discard, badWriter{}, true},
	} {
		streams := IO{In: tc.in, Out: tc.out, Err: tc.stderr, IsTerminal: func(io.Reader) bool { return tc.terminal }}
		if code := run(context.Background(), []string{"report-test"}, streams, testPlanCommand(streams)); code != Error {
			t.Fatalf("code %d", code)
		}
	}
}

type badWriter struct{}

func (badWriter) Write([]byte) (int, error) { return 0, errors.New("private writer payload") }

type badReader struct{}

func (badReader) Read([]byte) (int, error) { return 0, errors.New("private reader payload") }

func TestPlanOutputErrorsAreSafe(t *testing.T) {
	environment(t)
	for _, jsonMode := range []bool{false, true} {
		var stderr bytes.Buffer
		streams := IO{In: strings.NewReader(""), Out: badWriter{}, Err: &stderr, IsTerminal: func(io.Reader) bool { return false }}
		args := []string{"report-test"}
		if jsonMode {
			args = append(args, "--json")
		}
		if code := run(context.Background(), args, streams, testPlanCommand(streams)); code != Error || strings.Contains(stderr.String(), "private writer payload") {
			t.Fatal("unsafe output failure")
		}
	}
}
