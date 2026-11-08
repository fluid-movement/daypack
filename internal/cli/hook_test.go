package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fluid-movement/daypack/internal/cli"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func hookGitPath(t *testing.T, dir string) string {
	t.Helper()
	return filepath.Join(strings.TrimSpace(gitOut(t, dir, "rev-parse", "--path-format=absolute", "--git-path", "hooks")), "post-checkout")
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestHookInstallUninstallCLI(t *testing.T) {
	f := testutil.NewFixture(t, false)
	sentinel := testutil.PlantSentinel(t, f)
	path := hookGitPath(t, f.Project)

	r := testutil.RunCLI(t, f, "hook", "install")
	if r.Code != cli.Success || !strings.Contains(r.Stdout, path) {
		t.Fatalf("install: %+v", r)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "' apply --auto </dev/null || :\nexit 0\n") || !strings.HasPrefix(string(data), "#!/bin/sh\n# daypack post-checkout hook v1") {
		t.Fatalf("hook content %q: %v", data, err)
	}
	if strings.Contains(string(data), f.Project) || strings.Contains(string(data), f.Library) {
		t.Fatal("hook embeds project or library root")
	}
	info := stat(t, path)
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"hook", "install"}, {"hook", "install", "--json"}} {
		again := testutil.RunCLI(t, f, args...)
		if again.Code != cli.Success {
			t.Fatalf("repeat install: %+v", again)
		}
		after := stat(t, path)
		if after.ModTime().Sub(old) > time.Second || after.Mode() != info.Mode() {
			t.Fatal("repeat install modified hook")
		}
	}
	js := applyJSON(t, testutil.RunCLI(t, f, "hook", "install", "--json"), cli.Success, "daypack hook install")
	if item := js["items"].(map[string]any); item["conflict"] != false || item["path"] != path {
		t.Fatalf("json item %v", item)
	}

	u := testutil.RunCLI(t, f, "hook", "uninstall")
	if u.Code != cli.Success {
		t.Fatalf("uninstall: %+v", u)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("hook not removed")
	}
	if u = testutil.RunCLI(t, f, "hook", "uninstall", "--json"); u.Code != cli.Success {
		t.Fatalf("missing uninstall: %+v", u)
	}
	testutil.AssertNoSecret(t, f.Project, sentinel, r.Stdout, r.Stderr, u.Stdout, u.Stderr, string(data))
}

func TestHookForeignRefusal(t *testing.T) {
	f := testutil.NewFixture(t, false)
	path := hookGitPath(t, f.Project)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := "#!/bin/sh\necho mine\n"
	if err := os.WriteFile(path, []byte(foreign), 0o755); err != nil {
		t.Fatal(err)
	}
	before := testutil.SnapshotTree(t, filepath.Dir(path))

	text := testutil.RunCLI(t, f, "hook", "install")
	if text.Code != cli.Attention || !strings.Contains(text.Stderr, "apply --auto </dev/null || :") || !strings.Contains(text.Stderr, "' apply --auto") {
		t.Fatalf("text refusal: %+v", text)
	}
	js := applyJSON(t, testutil.RunCLI(t, f, "hook", "install", "--json"), cli.Attention, "daypack hook install")
	item := js["items"].(map[string]any)
	inv, _ := item["invocation"].(string)
	if item["conflict"] != true || !strings.HasPrefix(inv, "( '") || !strings.HasSuffix(inv, "' apply --auto </dev/null || : )") {
		t.Fatalf("json item %v", item)
	}
	un := testutil.RunCLI(t, f, "hook", "uninstall")
	if un.Code != cli.Attention {
		t.Fatalf("uninstall foreign: %+v", un)
	}
	testutil.AssertTree(t, filepath.Dir(path), before)
	if data := readFile(t, path); string(data) != foreign {
		t.Fatal("foreign hook changed")
	}
}

func TestHookCLIErrors(t *testing.T) {
	f := testutil.NewFixture(t, false)
	for _, args := range [][]string{{"hook", "install", "extra"}, {"hook", "uninstall", "--bogus"}} {
		if r := testutil.RunCLI(t, f, args...); r.Code != cli.Usage {
			t.Fatalf("%v: %+v", args, r)
		}
	}
	f.Project = t.TempDir()
	if r := testutil.RunCLI(t, f, "hook", "install"); r.Code != cli.Error {
		t.Fatalf("non-git: %+v", r)
	}
}

func buildDaypack(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "bin dir", "daypack")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/fluid-movement/daypack/cmd/daypack")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func worktreeFiles(t *testing.T, root string) map[string]testutil.TreeEntry {
	t.Helper()
	tree := testutil.SnapshotTree(t, root)
	for rel := range tree {
		if rel == ".git" || strings.HasPrefix(rel, ".git/") {
			delete(tree, rel)
		}
	}
	return tree
}

func mtimes(t *testing.T, root string) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	for rel := range testutil.SnapshotTree(t, root) {
		if strings.HasPrefix(rel, ".git") && rel != ".git" {
			continue
		}
		info, err := os.Lstat(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		out[rel] = info.ModTime().UnixNano()
	}
	return out
}

func TestHookRunsOnRealWorktreeAndCheckout(t *testing.T) {
	bin := buildDaypack(t) // before the fixture redirects HOME away from the module cache
	f := testutil.NewFixture(t, false)
	sentinel := testutil.PlantSentinel(t, f)
	writeApplyManifest(t, f.Project, supportedApplyManifest)
	hand := filepath.Join(f.Project, ".claude/agents/hand.md")
	if err := os.MkdirAll(filepath.Dir(hand), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hand, []byte("Handwritten.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, f.Project, "-c", "core.hooksPath="+os.DevNull, "add", ".")
	gitOut(t, f.Project, "-c", "core.hooksPath="+os.DevNull, "commit", "-m", "manifest")
	other := filepath.Join(f.Project, ".git/info/exclude")
	if err := os.MkdirAll(filepath.Dir(other), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("# other tool begin\nscratch/\n# other tool end\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	install := exec.Command(bin, "hook", "install", "--project", f.Project)
	if out, err := install.CombinedOutput(); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	script, err := os.ReadFile(hookGitPath(t, f.Project))
	if err != nil || !strings.Contains(string(script), bin) {
		t.Fatalf("hook does not name the built binary: %q %v", script, err)
	}
	mainBefore := worktreeFiles(t, f.Project)

	wtParent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(wtParent, "wt")
	out := gitOut(t, f.Project, "worktree", "add", "-b", "feature", wt)
	link, err := os.Readlink(filepath.Join(wt, ".claude/skills/testing"))
	if err != nil || link != filepath.Join(f.Library, "skills/testing") {
		t.Fatalf("skill not wired by hook (%q, %v)\n%s", link, err, out)
	}
	if _, err := os.Stat(filepath.Join(wt, ".claude/skills/testing/support.txt")); err != nil {
		t.Fatal("skill link misses support file: ", err)
	}
	if agent, err := os.ReadFile(filepath.Join(wt, ".claude/agents/reviewer.md")); err != nil || !strings.Contains(string(agent), "Review carefully.") {
		t.Fatalf("agent not rendered: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(wt, ".claude/agents/hand.md")); err != nil || string(data) != "Handwritten.\n" {
		t.Fatal("handwritten sibling changed")
	}
	if lockData, err := os.ReadFile(filepath.Join(wt, ".daypack/lock.toml")); err != nil || !strings.Contains(string(lockData), "skill:testing") {
		t.Fatalf("worktree lock missing: %v", err)
	}
	exclude := readFile(t, other)
	if !strings.Contains(string(exclude), "# other tool begin\nscratch/\n# other tool end\n") || !strings.Contains(string(exclude), "# daypack begin "+wt) {
		t.Fatalf("exclude blocks wrong:\n%s", exclude)
	}
	for _, rel := range []string{".claude/skills", ".daypack"} {
		if _, err := os.Lstat(filepath.Join(f.Project, rel)); err == nil {
			t.Fatalf("main checkout gained %s", rel)
		}
	}
	if !reflect.DeepEqual(mainBefore, worktreeFiles(t, f.Project)) {
		t.Fatal("main checkout files changed")
	}

	// Repeated checkouts converge without rewriting anything.
	gitOut(t, wt, "branch", "second")
	before, beforeTimes := testutil.SnapshotTree(t, wt), mtimes(t, wt)
	excludeBefore := readFile(t, other)
	time.Sleep(20 * time.Millisecond)
	gitOut(t, wt, "checkout", "second")
	gitOut(t, wt, "checkout", "feature")
	testutil.AssertTree(t, wt, before)
	if !reflect.DeepEqual(beforeTimes, mtimes(t, wt)) {
		t.Fatal("repeated checkout changed mtimes")
	}
	if after := readFile(t, other); string(after) != string(excludeBefore) {
		t.Fatal("repeated checkout changed exclude")
	}
	if link, err := os.Readlink(filepath.Join(wt, ".claude/skills/testing")); err != nil || link == "" {
		t.Fatal("link lost after checkout")
	}
	testutil.AssertNoSecret(t, wt, sentinel, out, string(script))

	// Uninstall through the product binary removes only the hook.
	un := exec.Command(bin, "hook", "uninstall", "--project", f.Project)
	if o, err := un.CombinedOutput(); err != nil {
		t.Fatalf("uninstall: %v\n%s", err, o)
	}
	if _, err := os.Lstat(hookGitPath(t, f.Project)); !os.IsNotExist(err) {
		t.Fatal("hook not removed")
	}
}

func TestHookMissingBinaryDoesNotBlockCheckout(t *testing.T) {
	bin := buildDaypack(t)
	f := testutil.NewFixture(t, false)
	writeApplyManifest(t, f.Project, supportedApplyManifest)
	gitOut(t, f.Project, "-c", "core.hooksPath="+os.DevNull, "add", ".")
	gitOut(t, f.Project, "-c", "core.hooksPath="+os.DevNull, "commit", "-m", "manifest")
	if out, err := exec.Command(bin, "hook", "install", "--project", f.Project).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if err := os.Remove(bin); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(t.TempDir(), "wt")
	gitOut(t, f.Project, "worktree", "add", "-b", "feature", wt)
	if _, err := os.Lstat(filepath.Join(wt, ".claude")); err == nil {
		t.Fatal("missing binary somehow applied")
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func stat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}
