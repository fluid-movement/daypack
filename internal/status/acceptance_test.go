package status_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/apply"
	"github.com/fluid-movement/daypack/internal/cli"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/fluid-movement/daypack/internal/status"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func goldenDir() string { return filepath.Join("testdata") }

func TestStatusGoldens(t *testing.T) {
	f := applied(t)
	res := testutil.RunCLI(t, f, "status")
	if res.Code != 0 {
		t.Fatal(res.Stdout, res.Stderr)
	}
	norm := func(s string) []byte { return testutil.NormalizeFixturePaths([]byte(s), f.Project, f.Library, f.Home) }
	testutil.AssertGolden(t, filepath.Join(goldenDir(), "clean.txt"), norm(res.Stdout))
	js := testutil.RunCLI(t, f, "--json", "status")
	testutil.AssertJSONGolden(t, filepath.Join(goldenDir(), "clean.json"), norm(js.Stdout))

	write(t, filepath.Join(f.Project, ".claude/agents/reviewer.md"), "edited\n")
	res = testutil.RunCLI(t, f, "status")
	if res.Code != 3 {
		t.Fatal(res.Code)
	}
	testutil.AssertGolden(t, filepath.Join(goldenDir(), "drift.txt"), norm(res.Stdout))
	js = testutil.RunCLI(t, f, "--json", "status")
	testutil.AssertJSONGolden(t, filepath.Join(goldenDir(), "drift.json"), norm(js.Stdout))
	alias := testutil.RunCLI(t, f, "check")
	if alias.Code != 3 || alias.Stdout != res.Stdout {
		t.Fatal("check alias differs from status")
	}
}

func sharedKindsAdapter() *testadapter.Adapter {
	a := testadapter.New("claude-code", false)
	a.Declarations = []contract.Target{
		{ID: "mcp-json", Root: ".config/mcp.json", Mode: contract.FileRoot, Types: []library.ArtifactType{library.MCP}},
		{ID: "mcp-array", Root: ".config/mcp-array.json", Mode: contract.FileRoot, Types: []library.ArtifactType{library.MCP}},
		{ID: "instructions", Root: "AGENTS.md", Mode: contract.FileRoot, Types: []library.ArtifactType{library.Fragment}},
	}
	a.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
		switch e.Artifact.Type {
		case library.MCP:
			return []contract.Item{
				{Target: "mcp-json", Entry: e.Ref, Kind: contract.JSONEntry, Path: ".config/mcp.json", Key: "mcp.docs", Value: []byte(`{"command":"echo"}`)},
				{Target: "mcp-array", Entry: e.Ref, Kind: contract.JSONArrayElement, Path: ".config/mcp-array.json", Key: "servers", Value: []byte(`"docs"`)},
			}, nil
		case library.Fragment:
			return []contract.Item{{Target: "instructions", Entry: e.Ref, Kind: contract.MarkdownSection, Path: "AGENTS.md", Marker: "fragment", Bytes: []byte("\nfragment\n")}}, nil
		}
		return nil, nil
	}
	return a
}

func build(t *testing.T, f testutil.Fixture, reg *adapters.Registry) *apply.Reconciliation {
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
	return x
}

func inspectWith(t *testing.T, f testutil.Fixture, reg *adapters.Registry, env func(string) (string, bool)) status.Report {
	t.Helper()
	r, err := status.Inspect(context.Background(), build(t, f, reg).Inspection(), status.Options{Registry: reg, HomeDir: f.Home, LookupEnv: env})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func setEnv(string) (string, bool) { return "x", true }

func codes(r status.Report) map[string]bool {
	m := map[string]bool{}
	for _, c := range r.Checks {
		m[c.Code] = true
	}
	return m
}

func rowState(r status.Report, entry string) status.State {
	for _, e := range r.Entries {
		if e.Entry == entry {
			return e.State
		}
	}
	return ""
}

func TestStatusSharedOwnershipKinds(t *testing.T) {
	reg, err := adapters.NewRegistry(sharedKindsAdapter())
	if err != nil {
		t.Fatal(err)
	}
	f := setup(t, "version=1\nharnesses=[\"claude-code\"]\nmcp=[\"docs\"]\nfragments=[\"conventions\"]\n")
	write(t, filepath.Join(f.Project, "AGENTS.md"), "# handwritten\n")
	x := build(t, f, reg)
	if _, err = x.Execute(context.Background(), x.Plan()); err != nil {
		t.Fatal(err)
	}
	r := inspectWith(t, f, reg, setEnv)
	if r.NeedsAttention() {
		t.Fatalf("applied shared kinds need attention: %+v %+v", r.Entries, r.Checks)
	}
	// Remove the JSON key and the array member.
	write(t, filepath.Join(f.Project, ".config/mcp.json"), "{}\n")
	write(t, filepath.Join(f.Project, ".config/mcp-array.json"), "{\"servers\":[]}\n")
	r = inspectWith(t, f, reg, setEnv)
	if rowState(r, "mcp:docs") != status.Pending || !codes(r)["missing-managed-path"] {
		t.Fatalf("deleted members: %+v %+v", r.Entries, r.Checks)
	}
	// Edit the Markdown section.
	b, err := os.ReadFile(filepath.Join(f.Project, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	edited := bytes.Replace(b, []byte("\nfragment\n"), []byte("\nedited\n"), 1)
	if bytes.Equal(edited, b) {
		t.Fatalf("section text not found: %q", b)
	}
	write(t, filepath.Join(f.Project, "AGENTS.md"), string(edited))
	r = inspectWith(t, f, reg, setEnv)
	if rowState(r, "fragment:conventions") != status.Drift {
		t.Fatalf("edited section: %+v", r.Entries)
	}
	if !strings.Contains(string(edited), "# handwritten") {
		t.Fatal("handwritten content lost")
	}
}

func TestStatusUnknownTargetNotObserved(t *testing.T) {
	f := setup(t, claudeManifest)
	l := lock.New()
	if err := l.Put(lock.Item{Target: "ghost-target", Entry: "skill:testing", Kind: lock.File, Path: "ghost/file", Hash: "h"}); err != nil {
		t.Fatal(err)
	}
	if err := l.Save(f.Project); err != nil {
		t.Fatal(err)
	}
	r := run(t, f, 3)
	for _, c := range r.Status.Checks {
		if c.Path == "ghost/file" {
			t.Fatalf("unknown target path was observed: %+v", c)
		}
	}
}

type meta map[string]string

func snapshotMeta(t *testing.T, root string, skipGit bool) meta {
	t.Helper()
	m := meta{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if skipGit && rel == ".git" {
			return filepath.SkipDir
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		st := info.Sys().(*syscall.Stat_t)
		m[rel] = fmt.Sprintf("%v %d %d", info.Mode(), info.ModTime().UnixNano(), st.Ino)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func gitMeta(t *testing.T, project string) meta {
	t.Helper()
	m := meta{}
	for _, p := range []string{".git/info/exclude", ".git/index"} {
		info, err := os.Lstat(filepath.Join(project, p))
		if err != nil {
			m[p] = "absent"
			continue
		}
		m[p] = fmt.Sprintf("%d %d %d", info.ModTime().UnixNano(), info.Size(), info.Sys().(*syscall.Stat_t).Ino)
	}
	return m
}

func sameMeta(t *testing.T, what string, a, b meta) {
	t.Helper()
	for k, v := range a {
		if b[k] != v {
			t.Errorf("%s: %s changed: %q -> %q", what, k, v, b[k])
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			t.Errorf("%s: %s created", what, k)
		}
	}
}

func TestStatusReadOnlyProof(t *testing.T) {
	scenarios := map[string]struct {
		prepare func(testutil.Fixture)
		code    int
	}{
		"success": {func(f testutil.Fixture) {}, 0},
		"drift":   {func(f testutil.Fixture) { write(t, filepath.Join(f.Project, ".claude/agents/reviewer.md"), "edited\n") }, 3},
		"malformed manifest": {func(f testutil.Fixture) {
			write(t, filepath.Join(f.Project, "daypack.toml"), "version = [\n")
		}, 1},
		"unsafe daypack dir": {func(f testutil.Fixture) {
			if err := os.RemoveAll(filepath.Join(f.Project, ".daypack")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(f.Library, filepath.Join(f.Project, ".daypack")); err != nil {
				t.Fatal(err)
			}
		}, 1},
	}
	for name, sc := range scenarios {
		t.Run(name, func(t *testing.T) {
			f := applied(t)
			sentinel := testutil.PlantSentinel(t, f)
			write(t, filepath.Join(f.Home, ".pi/agent/trust.json"), "{}\n")
			gitIn(t, f.Project, "add", "daypack.toml")
			sc.prepare(f)
			p0, l0, h0, g0 := snapshotMeta(t, f.Project, true), snapshotMeta(t, f.Library, false), snapshotMeta(t, f.Home, false), gitMeta(t, f.Project)
			res := testutil.RunCLI(t, f, "status")
			if res.Code != sc.code {
				t.Fatalf("exit %d, want %d: %s %s", res.Code, sc.code, res.Stdout, res.Stderr)
			}
			sameMeta(t, "project", p0, snapshotMeta(t, f.Project, true))
			sameMeta(t, "library", l0, snapshotMeta(t, f.Library, false))
			sameMeta(t, "home", h0, snapshotMeta(t, f.Home, false))
			sameMeta(t, "git", g0, gitMeta(t, f.Project))
			testutil.AssertNoSecret(t, f.Project, sentinel, res.Stdout, res.Stderr)
		})
	}
}

func TestStatusInlineMCP(t *testing.T) {
	f := setup(t, "version = 1\nharnesses = [\"claude-code\"]\nmcp = [\"docs\"]\n[inline.mcp.local]\ntype = \"stdio\"\ncommand = \"runner\"\nargs = [\"--x\", \"${DAYPACK_INLINE_ARG}\", \"${DAYPACK_DEFAULTED:-d}\"]\n[inline.mcp.local.env]\nAPI_TOKEN = \"${DAYPACK_INLINE_TOKEN}\"\n")
	t.Setenv("DAYPACK_TEST_SECRET", "present")
	if err := os.Unsetenv("DAYPACK_INLINE_TOKEN"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DAYPACK_INLINE_ARG", "")
	var out, errb bytes.Buffer
	code := cli.Run(context.Background(), []string{"--json", "--library", f.Library, "--project", f.Project, "status"}, cli.IO{In: failingReader{t}, Out: &out, Err: &errb, IsTerminal: func(io.Reader) bool { return true }})
	if code != 3 {
		t.Fatalf("exit %d: %s %s", code, out.String(), errb.String())
	}
	var doc struct {
		Status struct {
			Changes struct {
				Operations []struct {
					Entry   string
					Gated   bool
					Outcome string
					MCP     *struct {
						RequiredVariables []string `json:"required_variables"`
					}
				}
			}
			Checks []struct{ Code, Entry, Variable string }
		}
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	gated := false
	for _, o := range doc.Status.Changes.Operations {
		if o.Entry == "mcp:local" && o.Gated && o.MCP != nil && len(o.MCP.RequiredVariables) == 2 && o.Outcome != "skipped" {
			gated = true
		}
	}
	if !gated {
		t.Fatalf("no gated inline metadata: %s", out.String())
	}
	var unset []string
	for _, c := range doc.Status.Checks {
		if c.Code == "unset-variable" {
			unset = append(unset, c.Entry+":"+c.Variable)
		}
	}
	if len(unset) != 1 || unset[0] != "mcp:local:DAYPACK_INLINE_TOKEN" {
		t.Fatalf("unset = %v (empty counts as set, defaults and unrelated library servers do not)", unset)
	}
	if strings.Contains(out.String(), "present") {
		t.Fatal("value disclosed")
	}
}

type failingReader struct{ t *testing.T }

func (r failingReader) Read([]byte) (int, error) {
	r.t.Helper()
	r.t.Fatal("status read stdin")
	return 0, io.EOF
}

type failingWriter struct{ short bool }

func (w failingWriter) Write(p []byte) (int, error) {
	if w.short {
		return len(p) / 2, nil
	}
	return 0, errors.New("disk full")
}

func TestStatusWriterFailures(t *testing.T) {
	f := applied(t)
	for _, json := range []bool{false, true} {
		for _, short := range []bool{false, true} {
			args := []string{"--library", f.Library, "--project", f.Project, "status"}
			if json {
				args = append([]string{"--json"}, args...)
			}
			var errb bytes.Buffer
			code := cli.Run(context.Background(), args, cli.IO{In: failingReader{t}, Out: failingWriter{short}, Err: &errb})
			if code != cli.Error || !strings.Contains(errb.String(), "could not write result") {
				t.Errorf("json=%v short=%v: code %d stderr %q", json, short, code, errb.String())
			}
		}
	}
	code := cli.Run(context.Background(), []string{"--quiet", "--library", f.Library, "--project", f.Project, "status"}, cli.IO{In: failingReader{t}, Out: failingWriter{}, Err: failingWriter{}})
	if code != cli.Success && code != cli.Error {
		t.Fatalf("unexpected code %d", code)
	}
}

func TestStatusEscapedAndGitRules(t *testing.T) {
	a := testadapter.New("claude-code", false)
	a.Declarations = []contract.Target{
		{ID: "odd", Root: ".odd", Mode: contract.DirectoryRoot, Types: []library.ArtifactType{library.Skill}},
		{ID: "shared", Root: ".config/shared.json", Mode: contract.FileRoot, Types: []library.ArtifactType{library.MCP}},
	}
	a.Render = func(e contract.Entry, _ contract.Project) ([]contract.Item, error) {
		if e.Artifact.Type == library.MCP {
			return []contract.Item{{Target: "shared", Entry: e.Ref, Kind: contract.JSONEntry, Path: ".config/shared.json", Key: "mcp.docs", Value: []byte(`{"a":1}`)}}, nil
		}
		return []contract.Item{{Target: "odd", Entry: e.Ref, Kind: contract.File, Path: ".odd/we ird#[x]!*.txt", Bytes: []byte("x")}}, nil
	}
	reg, err := adapters.NewRegistry(a)
	if err != nil {
		t.Fatal(err)
	}
	f := setup(t, "version=1\nharnesses=[\"claude-code\"]\nskills=[\"testing\"]\nmcp=[\"docs\"]\n")
	// An intentionally tracked shared container.
	write(t, filepath.Join(f.Project, ".config/shared.json"), "{}\n")
	gitIn(t, f.Project, "add", "-f", ".config/shared.json")
	x := build(t, f, reg)
	if _, err = x.Execute(context.Background(), x.Plan()); err != nil {
		t.Fatal(err)
	}
	r := inspectWith(t, f, reg, setEnv)
	if r.NeedsAttention() {
		t.Fatalf("healthy escaped/tracked project: %+v %+v", r.Entries, r.Checks)
	}
	exclude := filepath.Join(f.Project, ".git/info/exclude")
	good, err := os.ReadFile(exclude)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(good), `/.odd/we\ ird\#\[x\]\!\*.txt`) || strings.Contains(string(good), "shared.json") {
		t.Fatalf("unexpected block: %s", good)
	}
	// Globally ignored path does not satisfy the exact rule.
	global := filepath.Join(t.TempDir(), "ignore")
	write(t, global, ".odd/\n")
	gitIn(t, f.Project, "config", "core.excludesFile", global)
	if err = os.WriteFile(exclude, nil, 0600); err != nil {
		t.Fatal(err)
	}
	r = inspectWith(t, f, reg, setEnv)
	missing := ""
	for _, c := range r.Checks {
		if c.Code == "git-exclude-missing" && c.Path == ".odd/we ird#[x]!*.txt" {
			missing = c.Path
		}
		if c.Path == ".config/shared.json" {
			t.Fatalf("tracked shared container flagged: %+v", c)
		}
	}
	if missing == "" {
		t.Fatalf("globally ignored path not reported: %+v", r.Checks)
	}
	// Duplicate block and malformed block.
	if err = os.WriteFile(exclude, append(append([]byte{}, good...), good...), 0600); err != nil {
		t.Fatal(err)
	}
	if r = inspectWith(t, f, reg, setEnv); !codes(r)["git-exclude-malformed"] {
		t.Fatalf("duplicate block: %+v", r.Checks)
	}
	if err = os.WriteFile(exclude, []byte(strings.Split(string(good), "# daypack end")[0]), 0600); err != nil {
		t.Fatal(err)
	}
	if r = inspectWith(t, f, reg, setEnv); !codes(r)["git-exclude-malformed"] {
		t.Fatalf("unclosed block: %+v", r.Checks)
	}
}

func TestStatusOtherReadersForRestrictedAndOpenEntries(t *testing.T) {
	shared := contract.Target{ID: contract.AgentsSkillsDir, Root: ".agents/skills", Mode: contract.DirectoryRoot, Types: []library.ArtifactType{library.Skill}}
	a := testadapter.New("claude-code", false)
	a.Declarations = []contract.Target{shared}
	b := testadapter.New("pi", false)
	b.Declarations = []contract.Target{shared}
	reg, err := adapters.NewRegistry(a, b)
	if err != nil {
		t.Fatal(err)
	}
	f := setup(t, "version=1\nharnesses=[\"claude-code\",\"pi\"]\nskills=[\"testing\"]\n")
	r := inspectWith(t, f, reg, setEnv)
	if len(r.Readers) != 1 || strings.Join(r.Readers[0].Selected, ",") != "claude-code,pi" {
		t.Fatalf("open entry: %+v", r.Readers)
	}
	for _, o := range r.Readers[0].OtherReaders {
		if o == "pi" {
			t.Fatal("selected producer listed as other reader")
		}
	}
	if len(r.Entries) != 2 {
		t.Fatalf("per-harness rows: %+v", r.Entries)
	}
	if len(r.Changes.Operations) != 1 {
		t.Fatalf("shared change duplicated: %+v", r.Changes.Operations)
	}
}
