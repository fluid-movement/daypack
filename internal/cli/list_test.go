package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/testutil"
)

type listEnvelope struct {
	OK      bool   `json:"ok"`
	Command string `json:"command"`
	Items   []struct {
		Type, Name, Description, Source string
		Used                            bool
		Harnesses                       []string
	} `json:"items"`
	Problems []struct{ Message, Hint string } `json:"Problems"`
}

func parseList(t *testing.T, out string) listEnvelope {
	t.Helper()
	var e listEnvelope
	if err := json.Unmarshal([]byte(out), &e); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return e
}

func TestList(t *testing.T) {
	f := testutil.NewFixture(t, false)
	r := testutil.RunCLI(t, f, "list")
	if r.Code != 0 {
		t.Fatalf("%+v", r)
	}
	for _, want := range []string{"skill\n", "mcp\n", "fragment\n", "agent\n", "[x] testing  Offline test skill  (claude-code, pi)"} {
		if !strings.Contains(r.Stdout, want) {
			t.Fatalf("missing %q in\n%s", want, r.Stdout)
		}
	}
	r = testutil.RunCLI(t, f, "list", "skills")
	if r.Code != 0 || strings.Contains(r.Stdout, "mcp") || !strings.Contains(r.Stdout, "testing") {
		t.Fatalf("%+v", r)
	}
	r = testutil.RunCLI(t, f, "list", "bogus")
	if r.Code != 2 || !strings.Contains(r.Stderr, "types: skill, mcp, fragment, agent") {
		t.Fatalf("%+v", r)
	}
	r = testutil.RunCLI(t, f, "list", "--quiet")
	if r.Code != 0 || r.Stdout != "skill:testing\nmcp:docs\nfragment:conventions\nagent:reviewer\n" {
		t.Fatalf("%+v", r)
	}
}

func TestListJSON(t *testing.T) {
	f := testutil.NewFixture(t, false)
	r := testutil.RunCLI(t, f, "list", "--json")
	e := parseList(t, r.Stdout)
	if r.Code != 0 || !e.OK || e.Command != "daypack list" || len(e.Items) != 4 {
		t.Fatalf("%+v", r)
	}
	if e.Items[0].Name != "testing" || e.Items[0].Source != "library" || !e.Items[0].Used || len(e.Items[0].Harnesses) != 2 || e.Items[0].Description != "Offline test skill" {
		t.Fatalf("%+v", e.Items[0])
	}
}

func TestListOutsideProject(t *testing.T) {
	f := testutil.NewFixture(t, false)
	if err := os.Remove(filepath.Join(f.Project, "daypack.toml")); err != nil {
		t.Fatal(err)
	}
	r := testutil.RunCLI(t, f, "list", "--json")
	e := parseList(t, r.Stdout)
	if r.Code != 0 || len(e.Items) != 4 {
		t.Fatalf("%+v", r)
	}
	for _, item := range e.Items {
		if item.Used || len(item.Harnesses) != 0 {
			t.Fatalf("%+v", item)
		}
	}
	if r = testutil.RunCLI(t, f, "list"); strings.Contains(r.Stdout, "[x]") {
		t.Fatal(r.Stdout)
	}
}

func TestListMissingLibrary(t *testing.T) {
	f := testutil.NewFixture(t, false)
	f.Library = filepath.Join(f.Library, "absent")
	r := testutil.RunCLI(t, f, "list")
	if r.Code != 1 || !strings.Contains(r.Stderr, "daypack library init") {
		t.Fatalf("%+v", r)
	}
}

func TestListInvalidManifest(t *testing.T) {
	f := testutil.NewFixture(t, false)
	text := "version = 1\nharnesses = [\"claude-code\"]\nskills = [\"testin\"]\n[inline.mcp.local]\ntype = \"stdio\"\ncommand = \"echo\"\n"
	if err := os.WriteFile(filepath.Join(f.Project, "daypack.toml"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	r := testutil.RunCLI(t, f, "list", "--json")
	e := parseList(t, r.Stdout)
	if r.Code != 3 || e.OK || len(e.Items) != 5 || len(e.Problems) == 0 || e.Problems[0].Hint != "did you mean skill:testing?" {
		t.Fatalf("%+v", r)
	}
	if r = testutil.RunCLI(t, f, "list", "mcp"); r.Code != 3 || !strings.Contains(r.Stdout, "local  (project-local)  (claude-code)") {
		t.Fatalf("%+v", r)
	}
}
