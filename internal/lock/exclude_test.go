package lock

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExcludeBlock(t *testing.T) {
	r := sandbox(t)
	items := []Item{
		{Kind: File, Path: "generated"}, {Kind: File, Path: "a[1] #!"},
		{Kind: JSONEntry, Path: "shared.json", Key: "a"}, {Kind: JSONEntry, Path: "shared.json", Key: "b", Tracked: true},
		{Kind: MDSection, Path: "AGENTS.md", Marker: "instructions", Tracked: true},
		{Kind: JSONEntry, Path: "settings.json", Key: "a"}, {Kind: JSONEntry, Path: "settings.json", Key: "b"},
	}
	b, e := ExcludeBlock(r, items)
	must(t, e)
	want := "# daypack begin " + r + "\n/.daypack/\n/a\\[1\\]\\ \\#\\!\n/generated\n/settings.json\n# daypack end " + r + "\n"
	if b != want {
		t.Fatalf("got %q want %q", b, want)
	}
	if _, e := ExcludeBlock(r+"\n", nil); e == nil {
		t.Fatal("newline root")
	}
	if _, e := ExcludeBlock(r, []Item{{Kind: File, Path: "a\nb"}}); e == nil {
		t.Fatal("newline path")
	}
	must(t, UpdateExclude(r, items))
	if _, e := os.Stat(filepath.Join(r, ".git")); !os.IsNotExist(e) {
		t.Fatal("created git outside repo")
	}
}
func TestWorktreeExclusion(t *testing.T) {
	r := sandbox(t)
	git(t, r, "init", "-q")
	git(t, r, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-qm", "fixture")
	wt := filepath.Join(t.TempDir(), "worktree")
	git(t, r, "-c", "core.hooksPath=/dev/null", "worktree", "add", "-qb", "test-worktree", wt)
	p, e := excludePath(r)
	must(t, e)
	wp, e := excludePath(wt)
	must(t, e)
	resolvedP, e := filepath.EvalSymlinks(p)
	must(t, e)
	resolvedWP, e := filepath.EvalSymlinks(wp)
	must(t, e)
	if resolvedP != resolvedWP {
		t.Fatalf("worktrees do not share exclude: %s %s", p, wp)
	}
	original := []byte("# user rules\nmanual\n")
	must(t, os.WriteFile(p, original, 0600))
	i := Item{Kind: File, Path: "generated"}
	must(t, UpdateExclude(r, []Item{i}))
	rootBlock, e := ExcludeBlock(r, []Item{i})
	must(t, e)
	must(t, UpdateExclude(wt, []Item{{Kind: File, Path: "other"}}))
	before, e := os.ReadFile(p)
	must(t, e)
	if !bytes.Contains(before, []byte(rootBlock)) || !bytes.HasPrefix(before, original) {
		t.Fatal(string(before))
	}
	must(t, UpdateExclude(wt, nil))
	after, e := os.ReadFile(p)
	must(t, e)
	if !bytes.Contains(after, []byte(rootBlock)) || bytes.Contains(after, []byte("/other\n")) {
		t.Fatal(string(after))
	}
	must(t, UpdateExclude(wt, nil))
	same, e := os.ReadFile(p)
	must(t, e)
	if !bytes.Equal(after, same) {
		t.Fatal("not stable")
	}
	// Exact exclusions keep handwritten siblings trackable.
	write(t, wt, "generated", []byte("x"))
	write(t, wt, "sibling", []byte("mine"))
	must(t, UpdateExclude(wt, []Item{i}))
	ignored := git(t, wt, "check-ignore", "generated")
	if ignored != "generated" {
		t.Fatal(ignored)
	}
	if strings.Contains(git(t, wt, "ls-files", "--others", "--exclude-standard"), "generated") {
		t.Fatal("generated visible")
	}
	if !strings.Contains(git(t, wt, "ls-files", "--others", "--exclude-standard"), "sibling") {
		t.Fatal("sibling hidden")
	}
	must(t, RemoveExclude(wt))
	data, e := os.ReadFile(p)
	must(t, e)
	if string(data) != string(original)+rootBlock {
		t.Fatal(string(data))
	}
	must(t, RemoveExclude(r))
	data, e = os.ReadFile(p)
	must(t, e)
	if !bytes.Equal(data, original) {
		t.Fatal("unrelated bytes changed")
	}
	dead := filepath.Join(t.TempDir(), "absent")
	block, e := ExcludeBlock(dead, nil)
	must(t, e)
	inspected, e := InspectExclude([]byte(block))
	must(t, e)
	if len(inspected) != 1 || !inspected[0].Stale {
		t.Fatal(inspected)
	}
}
func TestMalformedExcludeBlocks(t *testing.T) {
	sandbox(t)
	for _, b := range []string{"# daypack begin /a\n", "# daypack end /a\n", "# daypack begin /a\n# daypack begin /b\n# daypack end /b\n# daypack end /a\n", "# daypack begin /a\n# daypack end /b\n"} {
		if _, e := InspectExclude([]byte(b)); e == nil {
			t.Fatal("accepted malformed blocks")
		}
	}
}
