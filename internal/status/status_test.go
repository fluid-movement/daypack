package status_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/testutil"
)

const claudeManifest = "version = 1\nharnesses = [\"claude-code\"]\nskills = [\"testing\"]\nagents = [\"reviewer\"]\n"

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

type report struct {
	OK      bool   `json:"ok"`
	Command string `json:"command"`
	Status  struct {
		Entries []struct {
			Entry, Harness, State string
		}
		Checks []struct {
			Code, Entry, Path, Variable, Severity string
		}
		Readers []struct {
			Target       string
			OtherReaders []string `json:"other_readers"`
		}
		Changes struct {
			Notes []struct{ Code string }
		}
	}
}

func run(t *testing.T, f testutil.Fixture, code int, args ...string) report {
	t.Helper()
	res := testutil.RunCLI(t, f, append([]string{"--json", "status"}, args...)...)
	if res.Code != code {
		t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", res.Code, code, res.Stdout, res.Stderr)
	}
	dec := json.NewDecoder(strings.NewReader(res.Stdout))
	var r report
	if err := dec.Decode(&r); err != nil {
		t.Fatalf("%v: %s", err, res.Stdout)
	}
	if dec.More() {
		t.Fatal("more than one JSON document")
	}
	return r
}

func setup(t *testing.T, manifest string) testutil.Fixture {
	t.Helper()
	f := testutil.NewFixture(t, false)
	write(t, filepath.Join(f.Project, "daypack.toml"), manifest)
	return f
}

func applied(t *testing.T) testutil.Fixture {
	f := setup(t, claudeManifest)
	if res := testutil.RunCLI(t, f, "apply", "--yes"); res.Code != 0 {
		t.Fatalf("apply failed: %d %s %s", res.Code, res.Stdout, res.Stderr)
	}
	return f
}

func states(r report) map[string]string {
	m := map[string]string{}
	for _, e := range r.Status.Entries {
		m[e.Entry+"/"+e.Harness] = e.State
	}
	return m
}

func hasCheck(r report, code string) bool {
	for _, c := range r.Status.Checks {
		if c.Code == code {
			return true
		}
	}
	return false
}

func TestStatusAppliedAndPending(t *testing.T) {
	f := setup(t, claudeManifest)
	r := run(t, f, 3)
	if r.Command != "daypack status" || states(r)["skill:testing/claude-code"] != "pending" {
		t.Fatalf("unapplied: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(f.Project, ".daypack")); !os.IsNotExist(err) {
		t.Fatal("status created .daypack")
	}
	if _, err := os.Stat(filepath.Join(f.Project, ".claude")); !os.IsNotExist(err) {
		t.Fatal("status created generated directories")
	}
	f = applied(t)
	r = run(t, f, 0)
	for k, v := range states(r) {
		if v != "ok" {
			t.Errorf("%s = %s", k, v)
		}
	}
	if len(r.Status.Entries) != 2 {
		t.Fatalf("entries: %+v", r.Status.Entries)
	}
	if res := testutil.RunCLI(t, f, "check"); res.Code != 0 || !strings.Contains(res.Stdout, "All ok.") {
		t.Fatalf("check alias: %d %s", res.Code, res.Stdout)
	}
	write(t, filepath.Join(f.Project, "daypack.toml"), "version = 1\nharnesses = [\"claude-code\"]\nskills = [\"testing\"]\n")
	r = run(t, f, 3)
	if states(r)["agent:reviewer/"] != "pending" {
		t.Fatalf("obsolete entry: %+v", r.Status.Entries)
	}
}

func TestStatusOwnershipHealth(t *testing.T) {
	f := applied(t)
	agent := filepath.Join(f.Project, ".claude/agents/reviewer.md")
	write(t, agent, "edited\n")
	r := run(t, f, 3)
	if states(r)["agent:reviewer/claude-code"] != "drift" {
		t.Fatalf("edited agent: %+v", r.Status.Entries)
	}

	f = applied(t)
	if err := os.Remove(filepath.Join(f.Project, ".claude/skills/testing")); err != nil {
		t.Fatal(err)
	}
	r = run(t, f, 3)
	if states(r)["skill:testing/claude-code"] != "pending" || !hasCheck(r, "missing-managed-path") {
		t.Fatalf("deleted link: %+v", r.Status)
	}

	f = applied(t)
	if err := os.RemoveAll(filepath.Join(f.Library, "skills/testing")); err != nil {
		t.Fatal(err)
	}
	r = run(t, f, 3)
	if states(r)["skill:testing/"] != "unresolved" || !hasCheck(r, "dangling-library-link") {
		t.Fatalf("broken library link: %+v", r.Status)
	}

	f = applied(t)
	link := filepath.Join(f.Project, ".claude/skills/testing")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/nonexistent", link); err != nil {
		t.Fatal(err)
	}
	if r = run(t, f, 3); states(r)["skill:testing/claude-code"] != "drift" {
		t.Fatalf("wrong link: %+v", r.Status.Entries)
	}

	f = setup(t, claudeManifest)
	write(t, agent2(f), "hand written\n")
	if r = run(t, f, 3); states(r)["agent:reviewer/claude-code"] != "collision" {
		t.Fatalf("collision: %+v", r.Status.Entries)
	}
	if b, err := os.ReadFile(agent2(f)); err != nil || string(b) != "hand written\n" {
		t.Fatal("handwritten file changed")
	}
}

func agent2(f testutil.Fixture) string { return filepath.Join(f.Project, ".claude/agents/reviewer.md") }

func TestStatusGitHygiene(t *testing.T) {
	f := applied(t)
	exclude := filepath.Join(f.Project, ".git/info/exclude")
	good, err := os.ReadFile(exclude)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(exclude, nil, 0600); err != nil {
		t.Fatal(err)
	}
	r := run(t, f, 3)
	if !hasCheck(r, "git-exclude-missing") {
		t.Fatalf("missing block: %+v", r.Status.Checks)
	}
	if err = os.WriteFile(exclude, append([]byte("# daypack begin x\n"), good...), 0600); err != nil {
		t.Fatal(err)
	}
	if r = run(t, f, 3); !hasCheck(r, "git-exclude-malformed") {
		t.Fatalf("malformed: %+v", r.Status.Checks)
	}
	if err = os.WriteFile(exclude, good, 0600); err != nil {
		t.Fatal(err)
	}
	run(t, f, 0)
	cmd := exec.Command("git", "-C", f.Project, "add", "-f", ".claude/agents/reviewer.md")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if r = run(t, f, 3); !hasCheck(r, "git-tracked-managed-path") {
		t.Fatalf("tracked: %+v", r.Status.Checks)
	}
}

func TestStatusEnvironment(t *testing.T) {
	f := setup(t, "version = 1\nharnesses = [\"claude-code\"]\nmcp = [\"docs\"]\n")
	sentinel := testutil.PlantSentinel(t, f)
	t.Setenv("DAYPACK_TEST_SECRET", "")
	if r := run(t, f, 3); hasCheck(r, "unset-variable") {
		t.Fatal("explicit empty value must count as set")
	}
	if err := os.Unsetenv("DAYPACK_TEST_SECRET"); err != nil {
		t.Fatal(err)
	}
	res := testutil.RunCLI(t, f, "--json", "status")
	var r report
	if err := json.Unmarshal([]byte(res.Stdout), &r); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range r.Status.Checks {
		if c.Code == "unset-variable" && c.Variable == "DAYPACK_TEST_SECRET" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no unset-variable check: %s", res.Stdout)
	}
	testutil.AssertNoSecret(t, f.Project, sentinel, res.Stdout, res.Stderr)
	t.Setenv("DAYPACK_TEST_SECRET", sentinel)
	res = testutil.RunCLI(t, f, "status")
	if strings.Contains(res.Stdout, "unset-variable") {
		t.Fatalf("variable is set: %s", res.Stdout)
	}
	testutil.AssertNoSecret(t, f.Project, sentinel, res.Stdout, res.Stderr)
}

func TestStatusPiTrust(t *testing.T) {
	f := setup(t, "version = 1\nharnesses = [\"pi\"]\nskills = [\"testing\"]\n")
	r := run(t, f, 3)
	if !hasCheck(r, "pi-trust") || len(r.Status.Changes.Notes) == 0 {
		t.Fatalf("pi: %+v", r.Status)
	}
	store := filepath.Join(f.Home, ".pi/agent/trust.json")
	root, err := filepath.EvalSymlinks(f.Project)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, body string
		trusted    bool
	}{
		{"exact true", `{"` + root + `": true}`, true},
		{"ancestor true", `{"` + filepath.Dir(root) + `": true}`, true},
		{"bom", "\xef\xbb\xbf" + `{"` + root + `": true}`, true},
		{"child false", `{"` + filepath.Dir(root) + `": true, "` + root + `": false}`, false},
		{"null falls through", `{"` + filepath.Dir(root) + `": true, "` + root + `": null}`, true},
		{"malformed", `{`, false},
		{"array", `[]`, false},
		{"bad value", `{"` + root + `": "yes"}`, false},
		{"empty", `{}`, false},
	}
	for _, c := range cases {
		write(t, store, c.body)
		res := testutil.RunCLI(t, f, "--json", "status")
		var out report
		if err := json.Unmarshal([]byte(res.Stdout), &out); err != nil {
			t.Fatal(err)
		}
		var trustErr bool
		for _, ch := range out.Status.Checks {
			if ch.Code == "pi-trust" && ch.Severity == "error" {
				trustErr = true
			}
		}
		if trustErr == c.trusted {
			t.Errorf("%s: trust attention = %v", c.name, trustErr)
		}
	}
	if _, err := os.Stat(store + ".lock"); !os.IsNotExist(err) {
		t.Fatal("trust lock created")
	}
}

func TestStatusReadOnlyAndQuiet(t *testing.T) {
	f := applied(t)
	before := testutil.SnapshotTree(t, f.Project)
	lib := testutil.SnapshotTree(t, f.Library)
	res := testutil.RunCLI(t, f, "status")
	if res.Code != 0 {
		t.Fatalf("%d %s", res.Code, res.Stdout)
	}
	testutil.AssertTree(t, f.Project, before)
	testutil.AssertTree(t, f.Library, lib)
	write(t, filepath.Join(f.Project, ".claude/agents/reviewer.md"), "edited\n")
	res = testutil.RunCLI(t, f, "--quiet", "status")
	if res.Code != 3 || !strings.Contains(res.Stdout, "drift") {
		t.Fatalf("quiet attention: %d %q", res.Code, res.Stdout)
	}
	if res = testutil.RunCLI(t, f, "status", "extra"); res.Code != 2 {
		t.Fatalf("usage = %d", res.Code)
	}
}
