package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFixture(t *testing.T) {
	for _, worktree := range []bool{false, true} {
		t.Run(map[bool]string{false: "project", true: "worktree"}[worktree], func(t *testing.T) {
			// Deliberately poison inherited Git settings; fixtures must ignore them.
			t.Setenv("GIT_CONFIG_COUNT", "1")
			t.Setenv("GIT_CONFIG_KEY_0", "core.bare")
			t.Setenv("GIT_CONFIG_VALUE_0", "true")
			t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "missing"))
			f := NewFixture(t, worktree)
			for _, key := range []string{"HOME", "DAYPACK_HOME", "XDG_CONFIG_HOME"} {
				if os.Getenv(key) == "" {
					t.Fatalf("missing %s", key)
				}
			}
			if f.Home == f.Library || f.Home == f.Project || f.Library == f.Project {
				t.Fatal("fixture roots overlap")
			}
			for _, path := range []string{"skills/testing/SKILL.md", "skills/testing/support.txt", "mcp/docs.toml", "fragments/conventions.md", "agents/reviewer.md"} {
				if _, err := os.Stat(filepath.Join(f.Library, filepath.FromSlash(path))); err != nil {
					t.Fatal(err)
				}
			}
			info, err := os.Stat(filepath.Join(f.Project, ".git"))
			if err != nil || !info.IsDir() {
				t.Fatalf("project git directory: %v", err)
			}
			if worktree {
				info, err = os.Stat(filepath.Join(f.Worktree, ".git"))
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("worktree git file: %v", err)
				}
			}
			// Tests may replace fixture definitions with malformed data directly.
			writeFixture(t, filepath.Join(f.Library, "mcp/docs.toml"), "invalid [")
		})
	}
}
