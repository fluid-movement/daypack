package library

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitTest(t *testing.T, root string, args ...string) {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, output)
	}
}
func isolatedGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_COUNT", "0")
	for _, name := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR"} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}
func doctorHasProblem(problems []Problem, severity Severity, part string) bool {
	for _, p := range problems {
		if p.Severity == severity && strings.Contains(p.Message, part) {
			return true
		}
	}
	return false
}
func TestDoctorOutsideGit(t *testing.T) {
	root := mutationRoot(t)
	isolatedGit(t)
	problems, err := Doctor(root)
	if err != nil || len(problems) != 0 {
		t.Fatalf("%v %v", problems, err)
	}
	writeNative(t, filepath.Join(root, "unexpected"), []byte("ignored"))
	problems, err = Doctor(root)
	if err != nil || !doctorHasProblem(problems, Warning, "unknown") || doctorHasProblem(problems, Error, "") {
		t.Fatalf("%v %v", problems, err)
	}
	if err = os.Remove(filepath.Join(root, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	problems, err = Doctor(root)
	if err != nil || !doctorHasProblem(problems, Error, "ignore rule") {
		t.Fatalf("%v %v", problems, err)
	}
}
func TestDoctorGitHygiene(t *testing.T) {
	for _, mode := range []string{"root", "nested", "git-file", "worktree"} {
		t.Run(mode, func(t *testing.T) {
			root := mutationRoot(t)
			isolatedGit(t)
			repo := root
			if mode == "nested" {
				repo = filepath.Dir(root)
			}
			args := []string{"init", "--quiet"}
			if mode == "git-file" {
				args = append(args, "--separate-git-dir", filepath.Join(filepath.Dir(root), "git-metadata"))
			}
			gitTest(t, repo, args...)
			if mode == "worktree" {
				linked := filepath.Join(filepath.Dir(root), "linked-worktree")
				gitTest(t, repo, "worktree", "add", "--quiet", "--orphan", "-b", "linked", linked)
				root = filepath.Join(linked, "library")
				if err := Init(root); err != nil {
					t.Fatal(err)
				}
			}
			problems, err := Doctor(root)
			if err != nil || len(problems) != 0 {
				t.Fatalf("missing secrets file should be ignored: %v %v", problems, err)
			}
			writeNative(t, filepath.Join(root, "secrets.env"), []byte("TOKEN=DOCTOR_SENTINEL_PRIVATE\n"))
			gitTest(t, root, "add", "--force", "--", "secrets.env")
			problems, err = Doctor(root)
			if err != nil || !doctorHasProblem(problems, Error, "tracked") || doctorHasProblem(problems, Error, "not ignored") {
				t.Fatalf("%v %v", problems, err)
			}
			writeNative(t, filepath.Join(root, ".gitignore"), []byte("/secrets.env\n!/secrets.env\n"))
			problems, err = Doctor(root)
			if err != nil || !doctorHasProblem(problems, Error, "not ignored") {
				t.Fatalf("%v %v", problems, err)
			}
			for _, p := range problems {
				if strings.Contains(p.Message, "DOCTOR_SENTINEL_PRIVATE") || p.File == "" {
					t.Fatal("unsafe or unlocated diagnostic")
				}
			}
		})
	}
}
func TestDoctorInvalidNativeAndGitFailure(t *testing.T) {
	root := mutationRoot(t)
	isolatedGit(t)
	writeNative(t, filepath.Join(root, "agents", "broken.md"), []byte("malformed native file"))
	writeNative(t, filepath.Join(root, "mcp", "broken.toml"), []byte("type = [SECRET_SENTINEL_INVALID"))
	problems, err := Doctor(root)
	if err != nil || !doctorHasProblem(problems, Error, "frontmatter") || !doctorHasProblem(problems, Error, "TOML") {
		t.Fatalf("%v %v", problems, err)
	}
	for i, p := range problems {
		if p.File == "" || strings.Contains(p.Message, "SECRET_SENTINEL_INVALID") {
			t.Fatal("unsafe diagnostic")
		}
		if i > 0 && problems[i-1].File > p.File {
			t.Fatal("unstable ordering")
		}
	}
	writeNative(t, filepath.Join(root, ".git"), []byte("gitdir: missing-directory\n"))
	problems, err = Doctor(root)
	if err != nil || !doctorHasProblem(problems, Error, "repository") {
		t.Fatalf("%v %v", problems, err)
	}
}
