package manifest

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindProject(t *testing.T) {
	for _, worktree := range []bool{false, true} {
		t.Run(map[bool]string{false: "git directory", true: "worktree file"}[worktree], func(t *testing.T) {
			root := isolated(t)
			write(t, filepath.Join(root, "daypack.toml"), header)
			repo := filepath.Join(root, "repo")
			child := filepath.Join(repo, "nested", "deep")
			if err := os.MkdirAll(child, 0700); err != nil {
				t.Fatal(err)
			}
			if worktree {
				write(t, filepath.Join(repo, ".git"), "gitdir: elsewhere")
			} else if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
				t.Fatal(err)
			}
			if _, err := FindProject(child); err == nil || !strings.Contains(err.Error(), "daypack init") {
				t.Fatalf("crossed git boundary: %v", err)
			}
			write(t, filepath.Join(repo, "daypack.toml"), header)
			if got, err := FindProject(child); err != nil || got != repo {
				t.Fatalf("got %q, %v", got, err)
			}
			nearer := filepath.Join(repo, "nested")
			write(t, filepath.Join(nearer, "daypack.toml"), header)
			if got, err := FindProject(child); err != nil || got != nearer {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}
func TestFindProjectErrors(t *testing.T) {
	dir := isolated(t)
	if _, err := FindProject(filepath.Join(dir, "absent")); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "file"), "")
	if _, err := FindProject(filepath.Join(dir, "file")); err == nil || strings.Contains(err.Error(), "daypack init") {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "daypack.toml"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := FindProject(dir); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatal(err)
	}
	if _, err := FindProject(string(filepath.Separator)); err == nil || !strings.Contains(err.Error(), "daypack init") {
		t.Fatal(err)
	}
	// ELOOP is an inspection error, not a missing-manifest hint.
	loop := filepath.Join(dir, "loop")
	if err := os.Mkdir(loop, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("daypack.toml", filepath.Join(loop, "daypack.toml")); err != nil {
		t.Fatal(err)
	}
	if _, err := FindProject(loop); err == nil || strings.Contains(err.Error(), "daypack init") {
		t.Fatal(err)
	}
}
