package testutil

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type TreeEntry struct {
	Kind       string
	Bytes      []byte
	LinkTarget string
}

// SnapshotTree records all descendants, including dotfiles, without following links.
func SnapshotTree(t *testing.T, root string) map[string]TreeEntry {
	t.Helper()
	tree := map[string]TreeEntry{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry := TreeEntry{}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			entry.Kind = "symlink"
			entry.LinkTarget, err = os.Readlink(path)
		case info.IsDir():
			entry.Kind = "directory"
		case info.Mode().IsRegular():
			entry.Kind = "file"
			entry.Bytes, err = os.ReadFile(path)
		default:
			return fmt.Errorf("unsupported file kind at %s", rel)
		}
		if err != nil {
			return err
		}
		tree[filepath.ToSlash(rel)] = entry
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func AssertTree(t *testing.T, root string, want map[string]TreeEntry) {
	t.Helper()
	if diff := treeDiff(SnapshotTree(t, root), want); diff != "" {
		t.Error(diff)
	}
}

func treeDiff(got, want map[string]TreeEntry) string {
	keys := map[string]bool{}
	for k := range got {
		keys[k] = true
	}
	for k := range want {
		keys[k] = true
	}
	paths := make([]string, 0, len(keys))
	for k := range keys {
		paths = append(paths, k)
	}
	sort.Strings(paths)
	var diff []string
	for _, p := range paths {
		g, gok := got[p]
		w, wok := want[p]
		switch {
		case !gok:
			diff = append(diff, "missing: "+p)
		case !wok:
			diff = append(diff, "unexpected: "+p)
		case g.Kind != w.Kind || g.LinkTarget != w.LinkTarget || !bytes.Equal(g.Bytes, w.Bytes):
			diff = append(diff, "different content/kind: "+p)
		}
	}
	return strings.Join(diff, "\n")
}
