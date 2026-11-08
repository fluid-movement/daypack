package status_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/apply"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/fluid-movement/daypack/internal/status"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func inspect(t *testing.T, f testutil.Fixture, reg *adapters.Registry) status.Report {
	t.Helper()
	m, err := manifest.LoadManifest(filepath.Join(f.Project, "daypack.toml"))
	if err != nil {
		t.Fatal(err)
	}
	lib, err := library.Load(f.Library)
	if err != nil {
		t.Fatal(err)
	}
	x, err := apply.Build(context.Background(), f.Project, m, lib, reg, apply.Options{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := status.Inspect(context.Background(), x.Inspection(), status.Options{Registry: reg, HomeDir: f.Home, LookupEnv: func(string) (string, bool) { return "", false }})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestStatusReadersAndNotes(t *testing.T) {
	shared := contract.Target{ID: contract.AgentsSkillsDir, Root: ".agents/skills", Mode: contract.DirectoryRoot, Types: []library.ArtifactType{library.Skill}}
	a := testadapter.New("claude-code", false)
	a.Declarations = []contract.Target{shared}
	b := testadapter.New("pi", false)
	b.Declarations = []contract.Target{shared}
	reg, err := adapters.NewRegistry(a, b)
	if err != nil {
		t.Fatal(err)
	}
	f := setup(t, "version=1\nharnesses=[\"claude-code\",\"pi\"]\nskills=[{name=\"testing\",harnesses=[\"pi\"]}]\n")
	r := inspect(t, f, reg)
	if len(r.Changes.Operations) != 1 {
		t.Fatalf("shared change must appear once: %+v", r.Changes.Operations)
	}
	if len(r.Entries) != 1 || r.Entries[0].Harness != "pi" || r.Entries[0].State != status.Pending {
		t.Fatalf("entries: %+v", r.Entries)
	}
	if len(r.Readers) != 1 || !reflect.DeepEqual(r.Readers[0].Selected, []string{"pi"}) || !strings.Contains(strings.Join(r.Readers[0].OtherReaders, ","), "codex") {
		t.Fatalf("readers: %+v", r.Readers)
	}
	if len(r.Changes.Notes) != 2 {
		t.Fatalf("notes: %+v", r.Changes.Notes)
	}
	var out bytes.Buffer
	if err = status.RenderText(&out, r); err != nil || strings.Count(out.String(), "note pi fixture") != 1 {
		t.Fatalf("render: %v %s", err, out.String())
	}
}

func TestStatusRequiresDependencies(t *testing.T) {
	if _, err := status.Inspect(context.Background(), apply.Inspection{}, status.Options{}); err == nil {
		t.Fatal("expected input error")
	}
}

func TestStatusWorktree(t *testing.T) {
	f := testutil.NewFixture(t, true)
	f.Project = f.Worktree
	write(t, filepath.Join(f.Project, "daypack.toml"), claudeManifest)
	if res := testutil.RunCLI(t, f, "apply", "--yes"); res.Code != 0 {
		t.Fatalf("apply: %s %s", res.Stdout, res.Stderr)
	}
	run(t, f, 0)
	out, err := exec.Command("git", "-C", f.Project, "rev-parse", "--git-path", "info/exclude").Output()
	if err != nil {
		t.Fatal(err)
	}
	exclude := strings.TrimSpace(string(out))
	if !filepath.IsAbs(exclude) {
		exclude = filepath.Join(f.Project, exclude)
	}
	if err = os.WriteFile(exclude, []byte("# handwritten\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if r := run(t, f, 3); !hasCheck(r, "git-exclude-missing") {
		t.Fatalf("common exclude not consulted: %+v", r.Status.Checks)
	}
	if b, err := os.ReadFile(exclude); err != nil || string(b) != "# handwritten\n" {
		t.Fatal("status rewrote the exclude file")
	}
}
