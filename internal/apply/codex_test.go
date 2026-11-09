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

func TestCodexTOMLSnapshotRecheck(t *testing.T) {
	f := testutil.NewFixture(t, false)
	if err := os.WriteFile(filepath.Join(f.Project, "daypack.toml"), []byte("version=1\nharnesses=['codex']\nmcp=['docs']\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.Library, "mcp/docs.toml"), []byte("type='stdio'\ncommand='echo'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := adapters.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := apply.Prepare(context.Background(), f.Project, f.Library, r, apply.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(f.Project, ".codex"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(f.Project, ".codex/config.toml")
	const changed = "# written since preview\nmodel='mine'\n"
	if err := os.WriteFile(cfg, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Execute(context.Background(), prepared.Plan())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 1 || result.Operations[0].Outcome != plan.Skipped || !strings.Contains(result.Operations[0].Reason, "changed") {
		t.Fatal(result)
	}
	data, err := os.ReadFile(cfg)
	if err != nil || string(data) != changed {
		t.Fatal(string(data), err)
	}
}
