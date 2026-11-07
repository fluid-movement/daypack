package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotAndAssertTree(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	writeFixture(t, filepath.Join(external, "outside"), "must not traverse")
	writeFixture(t, filepath.Join(root, ".hidden", "config.json"), `{"handwritten":true}`)
	if err := os.Symlink(external, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	want := map[string]TreeEntry{".hidden": {Kind: "directory"}, ".hidden/config.json": {Kind: "file", Bytes: []byte(`{"handwritten":true}`)}, "link": {Kind: "symlink", LinkTarget: external}}
	AssertTree(t, root, want)
	snapshot := SnapshotTree(t, root)
	if len(snapshot) != 3 {
		t.Fatal("symlink followed or hidden files skipped")
	}
	if diff := treeDiff(snapshot, map[string]TreeEntry{"absent": {Kind: "file"}}); !strings.Contains(diff, "missing: absent") || !strings.Contains(diff, "unexpected: .hidden") {
		t.Fatal(diff)
	}
	want[".hidden/config.json"] = TreeEntry{Kind: "file", Bytes: []byte("different")}
	if !strings.Contains(treeDiff(snapshot, want), "different content/kind: .hidden/config.json") {
		t.Fatal("content difference lost")
	}
}
