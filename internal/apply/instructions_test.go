package apply_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/apply"
	"github.com/fluid-movement/daypack/internal/plan"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func fragmentProject(t *testing.T, fragments string) testutil.Fixture {
	t.Helper()
	f := testutil.NewFixture(t, false)
	manifest := "version = 1\nharnesses = [\"claude-code\", \"pi\"]\nfragments = " + fragments + "\n"
	if err := os.WriteFile(filepath.Join(f.Project, "daypack.toml"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	return f
}

func prepare(t *testing.T, f testutil.Fixture) *apply.Reconciliation {
	t.Helper()
	registry, err := adapters.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r, err := apply.Prepare(context.Background(), f.Project, f.Library, registry, apply.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Preparing a plan reports sections and the bridge without touching anything.
func TestFragmentPlanIsReadOnly(t *testing.T) {
	f := fragmentProject(t, `["conventions"]`)
	before := testutil.SnapshotTree(t, f.Project)
	p := prepare(t, f).Plan()
	testutil.AssertTree(t, f.Project, before)
	seen := map[string]plan.Operation{}
	for _, op := range p.Operations {
		seen[op.Path] = op
	}
	if a := seen["AGENTS.md"]; a.Marker != "fragment:conventions" || a.Action != plan.Create || a.Target != "agents-md" {
		t.Fatalf("AGENTS.md operation: %+v", a)
	}
	if c := seen["CLAUDE.md"]; c.Action != plan.Create || c.Entry != "fragment:@agents-md" {
		t.Fatalf("CLAUDE.md operation: %+v", c)
	}
}

// A file edited between preview and execution is skipped, never overwritten.
func TestFragmentExecuteDetectsEditSincePreview(t *testing.T) {
	f := fragmentProject(t, `["conventions"]`)
	r := prepare(t, f)
	edited := "edited after preview\n"
	if err := os.WriteFile(filepath.Join(f.Project, "AGENTS.md"), []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := r.Execute(context.Background(), r.Plan())
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range result.Operations {
		if op.Path == "AGENTS.md" && (op.Outcome != plan.Skipped || !strings.Contains(op.Reason, "changed since preview")) {
			t.Fatalf("operation: %+v", op)
		}
	}
	got, err := os.ReadFile(filepath.Join(f.Project, "AGENTS.md"))
	if err != nil || string(got) != edited {
		t.Fatalf("AGENTS.md overwritten: %q %v", got, err)
	}
}
