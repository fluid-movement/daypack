package claudecode_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters/claudecode"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/fluid-movement/daypack/internal/testutil"
)

// This composes the renderer with existing ownership APIs until production
// plan/apply exist; it does not exercise an apply command or engine.
func TestCopiedAgentOwnershipLifecycle(t *testing.T) {
	f := testutil.NewFixture(t, false)
	sentinel := testutil.PlantSentinel(t, f)
	defer testutil.AssertNoSecret(t, f.Project, sentinel)
	a := claudecode.New()
	roots := []string{a.Targets()[0].Root}
	render := func() contract.Item {
		t.Helper()
		lib, err := library.Load(f.Library)
		if err != nil {
			t.Fatal(err)
		}
		m, err := manifest.LoadManifest(filepath.Join(f.Project, "daypack.toml"))
		if err != nil {
			t.Fatal(err)
		}
		if problems := m.Validate(lib); len(problems) != 0 {
			t.Fatal(problems)
		}
		for _, e := range m.Entries() {
			if e.Type != library.Agent {
				continue
			}
			artifact, err := lib.Resolve("agent:" + e.Name)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(artifact.Path)
			if err != nil {
				t.Fatal(err)
			}
			items, err := a.Desired(contract.FromManifest(e, artifact, data), contract.Project{Root: f.Project, Harnesses: m.Harnesses})
			if err != nil || len(items) != 1 {
				t.Fatalf("render: %v %v", items, err)
			}
			return items[0]
		}
		t.Fatal("missing fixture agent")
		return contract.Item{}
	}
	ownership := func(i contract.Item) lock.Item {
		return lock.Item{Target: i.Target, Entry: i.Entry, Kind: lock.File, Path: i.Path, Hash: lock.HashBytes(i.Bytes)}
	}
	l := lock.New()
	status := func(i lock.Item, state lock.State, converged bool) {
		t.Helper()
		before := testutil.SnapshotTree(t, f.Project)
		obs, err := l.Status(f.Project, i)
		if err != nil || obs.State != state || obs.Converged != converged {
			t.Fatalf("status: %+v %v; want %s converged=%v", obs, err, state, converged)
		}
		testutil.AssertTree(t, f.Project, before)
	}
	persist := func() {
		t.Helper()
		if err := l.Save(f.Project); err != nil {
			t.Fatal(err)
		}
		loaded, err := lock.Load(f.Project)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(l.Items(), loaded.Items()) {
			t.Fatal("ownership persistence mismatch")
		}
		l = loaded
	}
	write := func(i contract.Item) {
		t.Helper()
		if err := lock.WriteFile(f.Project, i.Path, roots, i.Bytes, 0600); err != nil {
			t.Fatal(err)
		}
		if err := l.Put(ownership(i)); err != nil {
			t.Fatal(err)
		}
		persist()
	}
	before := testutil.SnapshotTree(t, f.Project)
	item := render()
	testutil.AssertTree(t, f.Project, before)
	row := ownership(item)
	status(row, lock.Missing, false)
	if err := os.MkdirAll(filepath.Join(f.Project, roots[0]), 0700); err != nil {
		t.Fatal(err)
	}
	write(item)
	tree := testutil.SnapshotTree(t, f.Project)
	entry := tree[item.Path]
	if entry.Kind != "file" || entry.LinkTarget != "" || !bytes.Equal(entry.Bytes, item.Bytes) {
		t.Fatal("not an exact regular copy")
	}
	status(ownership(render()), lock.OK, true)
	path := filepath.Join(f.Project, item.Path)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.WriteFile(f.Project, item.Path, roots, item.Bytes, 0600); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(info, after) || !info.ModTime().Equal(after.ModTime()) {
		t.Fatal("identical write replaced file")
	}
	testutil.AssertTree(t, f.Project, tree)

	source := filepath.Join(f.Library, "agents/reviewer.md")
	updated := append(bytes.Clone(item.Bytes), []byte("Updated library body.\n")...)
	if err := os.WriteFile(source, updated, 0600); err != nil {
		t.Fatal(err)
	}
	item = render()
	if !bytes.Equal(item.Bytes, updated) {
		t.Fatal("source update not rendered")
	}
	testutil.AssertTree(t, f.Project, tree)
	status(ownership(item), lock.OK, false)
	old, ok := l.Find(row)
	if !ok || old.Hash != row.Hash {
		t.Fatal("pending update changed previous ownership")
	}
	write(item)
	row = ownership(item)
	status(row, lock.OK, true)
	saved, ok := l.Find(row)
	if !ok || saved.Hash != lock.HashBytes(updated) {
		t.Fatal("written hash not saved")
	}

	if err := os.WriteFile(path, []byte("Hand edit.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	status(row, lock.Drifted, false)
	tree = testutil.SnapshotTree(t, f.Project)
	if err := l.RemoveOwned(f.Project, row, roots); err == nil {
		t.Fatal("removed drift")
	}
	testutil.AssertTree(t, f.Project, tree)
	testutil.AssertNoSecret(t, f.Project, sentinel)
	// Restore the managed bytes explicitly before testing clean removal.
	if err := os.WriteFile(path, item.Bytes, 0600); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(f.Project, roots[0], "handwritten.md")
	if err := os.WriteFile(sibling, []byte("Handwritten sibling.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := l.RemoveOwned(f.Project, row, roots); err != nil {
		t.Fatal(err)
	}
	persist()
	if len(l.Items()) != 0 {
		t.Fatal("removed ownership persisted")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("managed copy still exists: %v", err)
	}
	if data, err := os.ReadFile(sibling); err != nil || string(data) != "Handwritten sibling.\n" {
		t.Fatal("sibling changed")
	}

	if err := os.WriteFile(path, []byte("Unowned collision.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	status(row, lock.Collision, false)
	tree = testutil.SnapshotTree(t, f.Project)
	if err := l.RemoveOwned(f.Project, row, roots); err == nil {
		t.Fatal("removed unowned collision")
	}
	testutil.AssertTree(t, f.Project, tree)
}
