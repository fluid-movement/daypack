package status_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/status"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func TestStatusOnlyDaypackRuleRemoved(t *testing.T) {
	f := applied(t)
	exclude := filepath.Join(f.Project, ".git/info/exclude")
	b, err := os.ReadFile(exclude)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, line := range strings.Split(string(b), "\n") {
		if line != "/.daypack/" {
			kept = append(kept, line)
		}
	}
	if len(kept) == len(strings.Split(string(b), "\n")) {
		t.Fatalf("no .daypack rule in %q", b)
	}
	write(t, exclude, strings.Join(kept, "\n"))
	r := run(t, f, 3)
	var missing []string
	for _, c := range r.Status.Checks {
		if c.Code == "git-exclude-missing" {
			missing = append(missing, c.Path)
		}
	}
	if len(missing) != 1 || missing[0] != ".daypack/" {
		t.Fatalf("missing = %v, want only .daypack/", missing)
	}
}

func gitPath(t *testing.T, project, name string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", project, "rev-parse", "--path-format=absolute", "--git-path", name).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func fileInfo(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		return "absent"
	}
	m := snapshotMeta(t, filepath.Dir(p), false)[filepath.Base(p)]
	return m + "|" + string(b)
}

func TestStatusLinkedWorktreeLeavesSharedGitStateAlone(t *testing.T) {
	f := testutil.NewFixture(t, true)
	main := f
	wt := f
	wt.Project = f.Worktree
	for _, x := range []testutil.Fixture{main, wt} {
		write(t, filepath.Join(x.Project, "daypack.toml"), claudeManifest)
		if res := testutil.RunCLI(t, x, "apply", "--yes"); res.Code != 0 {
			t.Fatalf("apply: %s %s", res.Stdout, res.Stderr)
		}
	}
	exclude := gitPath(t, wt.Project, "info/exclude")
	b, err := os.ReadFile(exclude)
	if err != nil {
		t.Fatal(err)
	}
	write(t, exclude, "# handwritten rule\n*.scratch\n"+string(b))
	if strings.Count(string(b), "# daypack begin ") != 2 {
		t.Fatalf("expected both roots' blocks: %s", b)
	}
	index := gitPath(t, wt.Project, "index")
	mainIndex := gitPath(t, main.Project, "index")
	before := []string{fileInfo(t, exclude), fileInfo(t, index), fileInfo(t, mainIndex)}
	pm := snapshotMeta(t, wt.Project, true)
	if r := run(t, wt, 0); len(r.Status.Entries) != 2 {
		t.Fatalf("entries: %+v", r.Status.Entries)
	}
	after := []string{fileInfo(t, exclude), fileInfo(t, index), fileInfo(t, mainIndex)}
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("shared git file %d changed", i)
		}
	}
	sameMeta(t, "worktree", pm, snapshotMeta(t, wt.Project, true))
	data, err := os.ReadFile(exclude)
	if err != nil || !strings.Contains(string(data), "*.scratch") || strings.Count(string(data), "# daypack begin ") != 2 {
		t.Fatalf("exclude changed: %s", data)
	}
}

func TestStatusInlineMCPReferencesInEveryField(t *testing.T) {
	f := setup(t, "version = 1\nharnesses = [\"claude-code\"]\n"+
		"[inline.mcp.remote]\ntype = \"http\"\nurl = \"https://${DAYPACK_URL_HOST}/mcp\"\n[inline.mcp.remote.headers]\nAuthorization = \"Bearer ${DAYPACK_HEADER_TOKEN}\"\n"+
		"[inline.mcp.runner]\ntype = \"stdio\"\ncommand = \"${DAYPACK_COMMAND}\"\nargs = [\"${DAYPACK_ARG}\"]\n[inline.mcp.runner.env]\nAPI_TOKEN = \"${DAYPACK_ENV_TOKEN}\"\n")
	write(t, filepath.Join(f.Library, "mcp/unrelated.toml"), "type = \"stdio\"\ncommand = \"echo\"\n[env]\nTOKEN = \"${DAYPACK_UNRELATED_LIBRARY}\"\n")
	for _, name := range []string{"DAYPACK_URL_HOST", "DAYPACK_HEADER_TOKEN", "DAYPACK_COMMAND", "DAYPACK_ARG", "DAYPACK_ENV_TOKEN", "DAYPACK_UNRELATED_LIBRARY"} {
		t.Setenv(name, "x")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	r := run(t, f, 3)
	got := map[string]string{}
	for _, c := range r.Status.Checks {
		if c.Code == "unset-variable" {
			got[c.Variable] = c.Entry
		}
	}
	want := map[string]string{"DAYPACK_URL_HOST": "mcp:remote", "DAYPACK_HEADER_TOKEN": "mcp:remote", "DAYPACK_COMMAND": "mcp:runner", "DAYPACK_ARG": "mcp:runner", "DAYPACK_ENV_TOKEN": "mcp:runner"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s attributed to %q, want %q", k, got[k], v)
		}
	}
}

func TestStatusPiNotesAndTrustStoreAccess(t *testing.T) {
	f := setup(t, "version = 1\nharnesses = [\"pi\"]\n")
	r := run(t, f, 3)
	found := false
	for _, n := range r.Status.Changes.Notes {
		found = found || n.Code == "project-trust-required"
	}
	if !found || !hasCheck(r, "pi-trust") || len(r.Status.Entries) != 0 {
		t.Fatalf("empty selected Pi: %+v", r.Status)
	}
	// An unselected Pi never opens an unreadable trust store.
	g := setup(t, claudeManifest)
	if err := os.MkdirAll(filepath.Join(g.Home, ".pi/agent/trust.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if res := testutil.RunCLI(t, g, "apply", "--yes"); res.Code != 0 {
		t.Fatal(res.Stdout, res.Stderr)
	}
	r = run(t, g, 0)
	if hasCheck(r, "pi-trust") {
		t.Fatal("unselected Pi produced a trust check")
	}
	// A selected Pi with an unreadable store is an operational error.
	h := setup(t, "version = 1\nharnesses = [\"pi\"]\n")
	if err := os.MkdirAll(filepath.Join(h.Home, ".pi/agent/trust.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if res := testutil.RunCLI(t, h, "status"); res.Code != 1 {
		t.Fatalf("unreadable store: exit %d", res.Code)
	}
}

func TestStatusTextEscapesControlCharacters(t *testing.T) {
	a := testadapter.New("claude-code", false)
	a.Declarations = []contract.Target{{ID: "odd", Root: ".odd", Mode: contract.DirectoryRoot, Types: []library.ArtifactType{library.Skill}}}
	a.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
		return []contract.Item{{Target: "odd", Entry: e.Ref, Kind: contract.File, Path: ".odd/a\tb\x1b[31mc d.txt", Bytes: []byte("x")}}, nil
	}
	reg, err := adapters.NewRegistry(a)
	if err != nil {
		t.Fatal(err)
	}
	f := setup(t, "version=1\nharnesses=[\"claude-code\"]\nskills=[\"testing\"]\n")
	r := inspectWith(t, f, reg, setEnv)
	r.Checks = append(r.Checks, status.Check{Code: "x\ny", Entry: "skill:a\rb", Path: "p\x07q", Message: "m\nn", Severity: library.Error})
	for name, render := range map[string]func(*bytes.Buffer) error{
		"text":      func(b *bytes.Buffer) error { return status.RenderText(b, r) },
		"attention": func(b *bytes.Buffer) error { return status.RenderAttention(b, r) },
	} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		for _, c := range b.String() {
			if c != '\n' && (c < 0x20 || c == 0x7f || c == 0x2028) {
				t.Errorf("%s output has raw control %U", name, c)
			}
		}
		if name == "text" && !strings.Contains(b.String(), `\x1b`) {
			t.Errorf("escape not rendered: %q", b.String())
		}
		if lines := strings.Count(b.String(), "\n"); lines > 40 {
			t.Errorf("%s: injected newlines produced %d lines", name, lines)
		}
	}
}

func TestStatusReadOnlyProofExtended(t *testing.T) {
	type scenario struct {
		code    int
		prepare func(t *testing.T, f testutil.Fixture) testutil.Fixture
	}
	piManifest := "version = 1\nharnesses = [\"claude-code\", \"pi\"]\nskills = [\"testing\"]\n"
	scenarios := map[string]scenario{
		"unapplied": {3, func(t *testing.T, f testutil.Fixture) testutil.Fixture {
			write(t, filepath.Join(f.Project, "daypack.toml"), claudeManifest)
			return f
		}},
		"pi selected trusted": {0, func(t *testing.T, f testutil.Fixture) testutil.Fixture {
			write(t, filepath.Join(f.Project, "daypack.toml"), piManifest)
			if res := testutil.RunCLI(t, f, "apply", "--yes"); res.Code != 0 {
				t.Fatal(res.Stdout, res.Stderr)
			}
			root, err := filepath.EvalSymlinks(f.Project)
			if err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(f.Home, ".pi/agent/trust.json"), `{"`+root+`": true}`)
			return f
		}},
		"pi malformed trust": {3, func(t *testing.T, f testutil.Fixture) testutil.Fixture {
			write(t, filepath.Join(f.Project, "daypack.toml"), piManifest)
			write(t, filepath.Join(f.Home, ".pi/agent/trust.json"), "{")
			return f
		}},
		"pi unreadable trust": {1, func(t *testing.T, f testutil.Fixture) testutil.Fixture {
			write(t, filepath.Join(f.Project, "daypack.toml"), piManifest)
			if err := os.MkdirAll(filepath.Join(f.Home, ".pi/agent/trust.json"), 0700); err != nil {
				t.Fatal(err)
			}
			return f
		}},
		"malformed lock": {1, func(t *testing.T, f testutil.Fixture) testutil.Fixture {
			write(t, filepath.Join(f.Project, "daypack.toml"), claudeManifest)
			write(t, filepath.Join(f.Project, ".daypack/lock.toml"), "version = [\n")
			return f
		}},
		"unsafe lock symlink": {1, func(t *testing.T, f testutil.Fixture) testutil.Fixture {
			write(t, filepath.Join(f.Project, "daypack.toml"), claudeManifest)
			write(t, filepath.Join(f.Library, "elsewhere.toml"), "version = 1\n")
			if err := os.MkdirAll(filepath.Join(f.Project, ".daypack"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(f.Library, "elsewhere.toml"), filepath.Join(f.Project, ".daypack/lock.toml")); err != nil {
				t.Fatal(err)
			}
			return f
		}},
		"unset variable": {3, func(t *testing.T, f testutil.Fixture) testutil.Fixture {
			write(t, filepath.Join(f.Project, "daypack.toml"), "version = 1\nharnesses = [\"claude-code\"]\nmcp = [\"docs\"]\n")
			if err := os.Unsetenv("DAYPACK_TEST_SECRET"); err != nil {
				t.Fatal(err)
			}
			return f
		}},
		"linked worktree": {0, func(t *testing.T, f testutil.Fixture) testutil.Fixture {
			f.Project = f.Worktree
			write(t, filepath.Join(f.Project, "daypack.toml"), claudeManifest)
			if res := testutil.RunCLI(t, f, "apply", "--yes"); res.Code != 0 {
				t.Fatal(res.Stdout, res.Stderr)
			}
			return f
		}},
	}
	for name, sc := range scenarios {
		t.Run(name, func(t *testing.T) {
			f := testutil.NewFixture(t, name == "linked worktree")
			sentinel := testutil.PlantSentinel(t, f)
			f = sc.prepare(t, f)
			exclude, index := gitPath(t, f.Project, "info/exclude"), gitPath(t, f.Project, "index")
			treeBefore := testutil.SnapshotTree(t, f.Project)
			libTree := testutil.SnapshotTree(t, f.Library)
			p0, l0, h0 := snapshotMeta(t, f.Project, true), snapshotMeta(t, f.Library, false), snapshotMeta(t, f.Home, false)
			g0 := []string{fileInfo(t, exclude), fileInfo(t, index)}
			res := testutil.RunCLI(t, f, "status")
			if res.Code != sc.code {
				t.Fatalf("exit %d, want %d: %s %s", res.Code, sc.code, res.Stdout, res.Stderr)
			}
			sameMeta(t, "project", p0, snapshotMeta(t, f.Project, true))
			sameMeta(t, "library", l0, snapshotMeta(t, f.Library, false))
			sameMeta(t, "home", h0, snapshotMeta(t, f.Home, false))
			if g1 := []string{fileInfo(t, exclude), fileInfo(t, index)}; g0[0] != g1[0] || g0[1] != g1[1] {
				t.Error("git exclude or index changed")
			}
			if name != "linked worktree" {
				testutil.AssertTree(t, f.Project, treeBefore)
			}
			testutil.AssertTree(t, f.Library, libTree)
			if name == "unapplied" {
				for _, p := range []string{".daypack", ".claude"} {
					if _, err := os.Lstat(filepath.Join(f.Project, p)); !os.IsNotExist(err) {
						t.Errorf("%s created", p)
					}
				}
			}
			testutil.AssertNoSecret(t, f.Project, sentinel, res.Stdout, res.Stderr)
		})
	}
}

func TestStatusUnknownTargetFileAndSymlinkNeverFollowed(t *testing.T) {
	f := setup(t, claudeManifest)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(f.Project, "ghost/file"), "real file\n")
	if err := os.Symlink(outside, filepath.Join(f.Project, "ghost/link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/nonexistent/daypack-target", filepath.Join(f.Project, "ghost/dangling")); err != nil {
		t.Fatal(err)
	}
	l := lock.New()
	for _, i := range []lock.Item{
		{Target: "ghost-target", Entry: "skill:testing", Kind: lock.File, Path: "ghost/file", Hash: "h"},
		{Target: "ghost-target", Entry: "skill:testing", Kind: lock.Symlink, Path: "ghost/link", Link: outside},
		{Target: "ghost-target", Entry: "skill:testing", Kind: lock.Symlink, Path: "ghost/dangling", Link: "/nonexistent/daypack-target"},
	} {
		if err := l.Put(i); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Save(f.Project); err != nil {
		t.Fatal(err)
	}
	before := snapshotMeta(t, f.Project, true)
	outMeta := snapshotMeta(t, filepath.Dir(outside), false)
	r := run(t, f, 3)
	for _, c := range r.Status.Checks {
		if strings.HasPrefix(c.Path, "ghost/") {
			t.Fatalf("unknown target observed: %+v", c)
		}
	}
	sameMeta(t, "project", before, snapshotMeta(t, f.Project, true))
	sameMeta(t, "outside", outMeta, snapshotMeta(t, filepath.Dir(outside), false))
	b, err := os.ReadFile(filepath.Join(f.Project, "ghost/file"))
	if err != nil || string(b) != "real file\n" {
		t.Fatal("unknown-target file changed")
	}
}
