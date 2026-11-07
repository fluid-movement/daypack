// Package testutil supplies offline fixtures and assertions for daypack tests.
// Only test files should import this package. Helpers that set environment
// variables must not be used from parallel tests.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type Fixture struct{ Home, Library, Project, Worktree string }

func NewFixture(t *testing.T, withWorktree bool) Fixture {
	t.Helper()
	f := Fixture{Home: t.TempDir(), Library: t.TempDir(), Project: t.TempDir()}
	t.Setenv("HOME", f.Home)
	t.Setenv("DAYPACK_HOME", f.Library)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(f.Home, ".config"))
	files := map[string]string{
		"skills/testing/SKILL.md":    "---\nname: testing\ndescription: Offline test skill\n---\nUse support.txt.\n",
		"skills/testing/support.txt": "Test support.\n",
		"mcp/docs.toml":              "type = \"stdio\"\ncommand = \"echo\"\n[env]\nTOKEN = \"${DAYPACK_TEST_SECRET}\"\n",
		"fragments/conventions.md":   "---\ndescription: Test conventions\n---\nKeep tests offline.\n",
		"agents/reviewer.md":         "---\nname: reviewer\ndescription: Review test changes\n---\nReview carefully.\n",
	}
	for path, body := range files {
		writeFixture(t, filepath.Join(f.Library, filepath.FromSlash(path)), body)
	}
	writeFixture(t, filepath.Join(f.Project, "daypack.toml"), "version = 1\nharnesses = [\"claude-code\", \"pi\"]\nskills = [\"testing\"]\nmcp = [\"docs\"]\nfragments = [\"conventions\"]\nagents = [\"reviewer\"]\n")
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal("test fixtures require local git: ", err)
	}
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command(git, append([]string{"-C", f.Project, "-c", "core.hooksPath=" + os.DevNull, "-c", "core.excludesFile=" + os.DevNull, "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
		for _, variable := range os.Environ() {
			if !strings.HasPrefix(variable, "GIT_") {
				cmd.Env = append(cmd.Env, variable)
			}
		}
		cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_COUNT=0", "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	runGit("init", "--initial-branch=main", "--template=")
	runGit("config", "user.name", "Daypack Test")
	runGit("config", "user.email", "test@example.invalid")
	if withWorktree {
		runGit("add", "daypack.toml")
		runGit("commit", "-m", "fixture")
		f.Worktree = filepath.Join(t.TempDir(), "worktree")
		runGit("worktree", "add", "-b", "test-worktree", f.Worktree)
		t.Cleanup(func() { runGit("worktree", "remove", "--force", f.Worktree) })
	}
	return f
}

func writeFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
