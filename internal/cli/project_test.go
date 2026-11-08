package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/testutil"
)

func manifestPath(f testutil.Fixture) string { return filepath.Join(f.Project, "daypack.toml") }

func readManifest(t *testing.T, f testutil.Fixture) string {
	t.Helper()
	b, err := os.ReadFile(manifestPath(f))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func setManifest(t *testing.T, f testutil.Fixture, body string) {
	t.Helper()
	if err := os.WriteFile(manifestPath(f), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const baseManifest = "# top\nversion = 1\nharnesses = [\"claude-code\", \"pi\"]\n\n# skills\nskills = [\"zeta\"] # tail\n\nfragments = [\"conventions\"]\n"

func TestInitCommand(t *testing.T) {
	f := testutil.NewFixture(t, false)
	if err := os.Remove(manifestPath(f)); err != nil {
		t.Fatal(err)
	}
	r := testutil.RunCLI(t, f, "init")
	if r.Code != 0 || !strings.Contains(r.Stdout, "daypack apply") || !strings.Contains(r.Stdout, "daypack add") {
		t.Fatalf("%+v", r)
	}
	first := readManifest(t, f)
	r = testutil.RunCLI(t, f, "init")
	if r.Code != 1 || readManifest(t, f) != first {
		t.Fatalf("%+v", r)
	}
	if r = testutil.RunCLI(t, f, "init", "--harness", "bogus"); r.Code != 2 {
		t.Fatalf("%+v", r)
	}
	if r = testutil.RunCLI(t, f, "init", "extra"); r.Code != 2 {
		t.Fatalf("%+v", r)
	}
}

func TestInitFromAndJSON(t *testing.T) {
	src := testutil.NewFixture(t, false)
	setManifest(t, src, baseManifest)
	dst := testutil.NewFixture(t, false)
	if err := os.Remove(manifestPath(dst)); err != nil {
		t.Fatal(err)
	}
	r := testutil.RunCLI(t, dst, "--json", "init", "--from", src.Project)
	if r.Code != 0 || readManifest(t, dst) != baseManifest {
		t.Fatalf("%+v", r)
	}
	var out struct {
		OK      bool
		Command string
		Items   []map[string]string
	}
	if err := json.Unmarshal([]byte(r.Stdout), &out); err != nil || !out.OK || len(out.Items) != 1 {
		t.Fatalf("%v %s", err, r.Stdout)
	}
}

func TestAddCommand(t *testing.T) {
	f := testutil.NewFixture(t, false)
	setManifest(t, f, baseManifest)
	before := testutil.SnapshotTree(t, f.Project)
	r := testutil.RunCLI(t, f, "add", "skill:testing")
	want := strings.Replace(baseManifest, `["zeta"]`, `["zeta", "testing"]`, 1)
	if r.Code != 0 || readManifest(t, f) != want || !strings.Contains(r.Stdout, "daypack apply") {
		t.Fatalf("%+v\n%s", r, readManifest(t, f))
	}
	r = testutil.RunCLI(t, f, "add", "skill:testing")
	if r.Code != 0 || readManifest(t, f) != want || !strings.Contains(r.Stdout, "Already present") {
		t.Fatalf("%+v", r)
	}
	r = testutil.RunCLI(t, f, "add", "mcp:docs", "--only", "claude-code")
	if r.Code != 0 || !strings.Contains(readManifest(t, f), `mcp = [{ name = "docs", harnesses = ["claude-code"] }]`) {
		t.Fatalf("%+v\n%s", r, readManifest(t, f))
	}
	r = testutil.RunCLI(t, f, "add", "mcp:docs", "--only", "pi")
	got := readManifest(t, f)
	if r.Code != 0 || !strings.Contains(got, `{ name = "docs", harnesses = ["pi"] }`) || strings.Count(got, `"docs"`) != 1 || !strings.Contains(r.Stdout, "Updated") {
		t.Fatalf("%+v\n%s", r, got)
	}
	// Without --only a restricted entry stays as is.
	if r = testutil.RunCLI(t, f, "add", "mcp:docs"); r.Code != 0 || readManifest(t, f) != got {
		t.Fatalf("%+v", r)
	}
	// Widening to the full set writes the plain form.
	if r = testutil.RunCLI(t, f, "add", "mcp:docs", "--only", "pi,claude-code"); r.Code != 0 || !strings.Contains(readManifest(t, f), `mcp = ["docs"]`) {
		t.Fatalf("%+v\n%s", r, readManifest(t, f))
	}
	if !strings.Contains(readManifest(t, f), "# top\n") || !strings.Contains(readManifest(t, f), "# tail\n") {
		t.Fatal("comments lost")
	}
	after := testutil.SnapshotTree(t, f.Project)
	if len(after) != len(before) {
		t.Fatalf("tree changed: %v", after)
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Fatalf("unexpected %s", path)
		}
	}
}

func TestAddErrors(t *testing.T) {
	f := testutil.NewFixture(t, false)
	setManifest(t, f, baseManifest)
	cases := []struct {
		args []string
		code int
		text string
	}{
		{[]string{"add", "skill:testng"}, 1, "did you mean skill:testing"},
		{[]string{"add", "bogus:x"}, 2, ""},
		{[]string{"add", "skill"}, 2, ""},
		{[]string{"add", "skill:testing", "--only", ""}, 2, ""},
		{[]string{"add", "skill:testing", "--only", "bogus"}, 2, ""},
		{[]string{"add", "skill:testing", "--only", "pi,pi"}, 2, ""},
	}
	for _, tc := range cases {
		r := testutil.RunCLI(t, f, tc.args...)
		if r.Code != tc.code || !strings.Contains(r.Stderr, tc.text) {
			t.Fatalf("%v: %+v", tc.args, r)
		}
	}
	setManifest(t, f, "version = 1\nharnesses = [\"pi\"]\n")
	if r := testutil.RunCLI(t, f, "add", "skill:testing", "--only", "claude-code"); r.Code != 1 || !strings.Contains(r.Stderr, "pi") {
		t.Fatalf("%+v", r)
	}
	if err := os.Remove(manifestPath(f)); err != nil {
		t.Fatal(err)
	}
	if r := testutil.RunCLI(t, f, "add", "skill:testing"); r.Code != 1 || !strings.Contains(r.Stderr, "daypack init") {
		t.Fatalf("%+v", r)
	}
	f = testutil.NewFixture(t, false)
	if err := os.WriteFile(filepath.Join(f.Library, "skills", "testing", "SKILL.md"), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := testutil.RunCLI(t, f, "add", "skill:testing"); r.Code != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestRemoveCommand(t *testing.T) {
	f := testutil.NewFixture(t, false)
	setManifest(t, f, baseManifest)
	r := testutil.RunCLI(t, f, "remove", "skill:zeta")
	want := strings.Replace(baseManifest, `["zeta"]`, `[]`, 1)
	if r.Code != 0 || readManifest(t, f) != want || !strings.Contains(r.Stdout, "daypack apply") {
		t.Fatalf("%+v\n%s", r, readManifest(t, f))
	}
	r = testutil.RunCLI(t, f, "remove", "skill:zeta")
	if r.Code != 1 || !strings.Contains(r.Stderr, "Hint:") || readManifest(t, f) != want {
		t.Fatalf("%+v", r)
	}
	// Dangling library reference can still be removed.
	if err := os.RemoveAll(filepath.Join(f.Library, "fragments")); err != nil {
		t.Fatal(err)
	}
	if r = testutil.RunCLI(t, f, "remove", "fragment:conventions"); r.Code != 0 || strings.Contains(readManifest(t, f), "conventions") {
		t.Fatalf("%+v", r)
	}
	setManifest(t, f, baseManifest+"\n[inline.mcp.local]\ntype = \"stdio\"\ncommand = \"echo\"\n")
	before := readManifest(t, f)
	if r = testutil.RunCLI(t, f, "remove", "mcp:local"); r.Code != 1 || !strings.Contains(r.Stderr, "by hand") || readManifest(t, f) != before {
		t.Fatalf("%+v", r)
	}
}

func TestAddRemoveRoundTrip(t *testing.T) {
	f := testutil.NewFixture(t, false)
	setManifest(t, f, baseManifest)
	for _, args := range [][]string{{"skill:testing"}, {"skill:testing", "--only", "pi"}} {
		if r := testutil.RunCLI(t, f, append([]string{"add"}, args...)...); r.Code != 0 {
			t.Fatalf("%+v", r)
		}
		if r := testutil.RunCLI(t, f, "remove", "skill:testing"); r.Code != 0 {
			t.Fatalf("%+v", r)
		}
		if got := readManifest(t, f); got != baseManifest {
			t.Fatalf("not restored:\n%s", got)
		}
	}
}
