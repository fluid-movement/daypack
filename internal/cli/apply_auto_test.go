package cli_test

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/cli"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func applyJSON(t *testing.T, r testutil.CLIResult, code int, command string) map[string]any {
	t.Helper()
	if r.Code != code {
		t.Fatalf("exit %d, want %d: %s %s", r.Code, code, r.Stdout, r.Stderr)
	}
	dec := json.NewDecoder(strings.NewReader(r.Stdout))
	var v map[string]any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("json %q: %v", r.Stdout, err)
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		t.Fatal("more than one JSON document")
	}
	if v["command"] != command || v["ok"] != (code == cli.Success) {
		t.Fatalf("envelope %v", v)
	}
	return v
}

func TestApplyAutoLibraryEntries(t *testing.T) {
	f := testutil.NewFixture(t, false)
	sentinel := testutil.PlantSentinel(t, f)
	writeApplyManifest(t, f.Project, supportedApplyManifest)
	libBefore := testutil.SnapshotTree(t, f.Library)
	r := testutil.RunCLIWithInput(t, f, unreadable{t}, true, "apply", "--auto")
	if r.Code != cli.Success {
		t.Fatalf("auto: %+v", r)
	}
	if link, err := os.Readlink(filepath.Join(f.Project, ".claude/skills/testing")); err != nil || link != filepath.Join(f.Library, "skills/testing") {
		t.Fatalf("skill not applied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.Project, ".claude/agents/reviewer.md")); err != nil {
		t.Fatal(err)
	}
	testutil.AssertTree(t, f.Library, libBefore)
	before := testutil.SnapshotTree(t, f.Project)
	again := testutil.RunCLIWithInput(t, f, unreadable{t}, true, "apply", "--auto", "--yes", "--json")
	applyJSON(t, again, cli.Success, "daypack apply")
	testutil.AssertTree(t, f.Project, before)
	testutil.AssertNoSecret(t, f.Project, sentinel, r.Stdout, r.Stderr, again.Stdout, again.Stderr)
}

func TestApplyAutoRejectsForceAdopt(t *testing.T) {
	f := testutil.NewFixture(t, false)
	writeApplyManifest(t, f.Project, supportedApplyManifest)
	before := testutil.SnapshotTree(t, f.Project)
	for _, flag := range []string{"--force", "--adopt"} {
		r := testutil.RunCLIWithInput(t, f, unreadable{t}, true, "apply", "--auto", flag, "--json")
		applyJSON(t, r, cli.Usage, "daypack apply")
		testutil.AssertTree(t, f.Project, before)
	}
}

func TestApplyAutoDryRunWritesNothing(t *testing.T) {
	f := testutil.NewFixture(t, false)
	writeApplyManifest(t, f.Project, supportedApplyManifest)
	before := testutil.SnapshotTree(t, f.Project)
	libBefore := testutil.SnapshotTree(t, f.Library)
	r := testutil.RunCLIWithInput(t, f, unreadable{t}, true, "apply", "--auto", "--dry-run", "--json")
	v := applyJSON(t, r, cli.Success, "daypack apply")
	if len(v["operations"].([]any)) != 2 {
		t.Fatalf("expected planned operations: %s", r.Stdout)
	}
	testutil.AssertTree(t, f.Project, before)
	testutil.AssertTree(t, f.Library, libBefore)
}

func TestApplyAutoNoManifestSilent(t *testing.T) {
	f := testutil.NewFixture(t, false)
	if err := os.Remove(filepath.Join(f.Project, "daypack.toml")); err != nil {
		t.Fatal(err)
	}
	// A missing library proves the library is never opened.
	f.Library = filepath.Join(t.TempDir(), "absent")
	before := testutil.SnapshotTree(t, f.Project)
	r := testutil.RunCLIWithInput(t, f, unreadable{t}, true, "apply", "--auto")
	if r.Code != cli.Success || r.Stdout != "" || r.Stderr != "" {
		t.Fatalf("not silent: %+v", r)
	}
	testutil.AssertTree(t, f.Project, before)
	if _, err := os.Lstat(f.Library); err == nil {
		t.Fatal("library created")
	}
	if _, err := os.Lstat(filepath.Join(f.Project, ".daypack")); err == nil {
		t.Fatal(".daypack created")
	}
	applyJSON(t, testutil.RunCLI(t, f, "apply", "--auto", "--json"), cli.Success, "daypack apply")
	// The manual command still reports the missing manifest.
	if m := testutil.RunCLI(t, f, "apply"); m.Code != cli.Error {
		t.Fatalf("manual apply: %+v", m)
	}
}

func TestApplyAutoFailuresExitZero(t *testing.T) {
	cases := map[string]func(t *testing.T, f testutil.Fixture){
		"malformed manifest": func(t *testing.T, f testutil.Fixture) { writeApplyManifest(t, f.Project, "not valid [") },
		"missing library": func(t *testing.T, f testutil.Fixture) {
			if err := os.RemoveAll(f.Library); err != nil {
				t.Fatal(err)
			}
		},
		"unresolved": func(t *testing.T, f testutil.Fixture) {
			writeApplyManifest(t, f.Project, "version = 1\nharnesses = [\"claude-code\"]\nskills = [\"missing\"]\n")
		},
		"corrupt lock": func(t *testing.T, f testutil.Fixture) {
			if err := os.MkdirAll(filepath.Join(f.Project, ".daypack"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(f.Project, ".daypack/lock.toml"), []byte("invalid ["), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"collision": func(t *testing.T, f testutil.Fixture) {
			p := filepath.Join(f.Project, ".claude/agents/reviewer.md")
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("Handwritten.\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			f := testutil.NewFixture(t, false)
			sentinel := testutil.PlantSentinel(t, f)
			writeApplyManifest(t, f.Project, supportedApplyManifest)
			setup(t, f)
			for _, args := range [][]string{{"apply", "--auto"}, {"apply", "--auto", "--json"}} {
				r := testutil.RunCLIWithInput(t, f, unreadable{t}, true, args...)
				if r.Code != cli.Success {
					t.Fatalf("%v: exit %d: %s %s", args, r.Code, r.Stdout, r.Stderr)
				}
				if r.Stdout == "" && r.Stderr == "" {
					t.Fatalf("%v: failure produced no diagnostic", args)
				}
				testutil.AssertNoSecret(t, f.Project, sentinel, r.Stdout, r.Stderr)
			}
			if name == "collision" {
				if data := readFile(t, filepath.Join(f.Project, ".claude/agents/reviewer.md")); string(data) != "Handwritten.\n" {
					t.Fatal("collision overwritten")
				}
			}
			// The same condition keeps manual apply's non-zero status.
			if m := testutil.RunCLI(t, f, "apply", "--json"); m.Code == cli.Success {
				t.Fatalf("manual apply unexpectedly succeeded: %s", m.Stdout)
			}
		})
	}
}

func TestApplyAutoDriftExitsZero(t *testing.T) {
	f := testutil.NewFixture(t, false)
	writeApplyManifest(t, f.Project, supportedApplyManifest)
	applyJSON(t, testutil.RunCLI(t, f, "apply", "--json"), cli.Success, "daypack apply")
	agent := filepath.Join(f.Project, ".claude/agents/reviewer.md")
	if err := os.WriteFile(agent, []byte("Edited.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := testutil.RunCLIWithInput(t, f, unreadable{t}, true, "apply", "--auto", "--json")
	applyJSON(t, r, cli.Success, "daypack apply")
	if !strings.Contains(r.Stdout, "drift") {
		t.Fatalf("drift not reported: %s", r.Stdout)
	}
	if data := readFile(t, agent); string(data) != "Edited.\n" {
		t.Fatal("auto overwrote drifted content")
	}
}
