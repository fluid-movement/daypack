package claudecode_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/claudecode"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func read(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	must(t, err)
	return b
}
func write(t *testing.T, p string, b []byte) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(p), 0700))
	must(t, os.WriteFile(p, b, 0600))
}
func registry(t *testing.T) *adapters.Registry {
	t.Helper()
	r, err := adapters.NewRegistry(claudecode.New(), testadapter.New("pi", false))
	must(t, err)
	return r
}
func renderFixture(t *testing.T, f testutil.Fixture, root string, r *adapters.Registry) adapters.Result {
	t.Helper()
	beforeProject, beforeLibrary := testutil.SnapshotTree(t, root), testutil.SnapshotTree(t, f.Library)
	lib, err := library.Load(f.Library)
	must(t, err)
	m, err := manifest.LoadManifest(filepath.Join(root, "daypack.toml"))
	must(t, err)
	if problems := m.Validate(lib); len(problems) != 0 {
		t.Fatal(problems)
	}
	var entries []contract.Entry
	for _, e := range m.Entries() {
		a, err := lib.Resolve(string(e.Type) + ":" + e.Name)
		must(t, err)
		entries = append(entries, contract.FromManifest(e, a, nil))
	}
	result, err := r.Desired(entries, contract.Project{Root: root, Harnesses: m.Harnesses})
	must(t, err)
	testutil.AssertTree(t, root, beforeProject)
	testutil.AssertTree(t, f.Library, beforeLibrary)
	return result
}
func claudeItem(t *testing.T, result adapters.Result) contract.Item {
	t.Helper()
	var found []contract.Item
	for _, i := range result.Items {
		if i.Target == contract.ClaudeSkillsDir {
			found = append(found, i)
		}
	}
	if len(found) != 1 {
		t.Fatalf("Claude items: %#v", found)
	}
	return found[0]
}
func roots(r *adapters.Registry) []string {
	var out []string
	for _, target := range r.Targets() {
		out = append(out, target.Root)
	}
	return out
}
func ownedItem(t *testing.T, root string, i contract.Item) lock.Item {
	t.Helper()
	tracked, err := lock.IsTracked(root, i.Path)
	must(t, err)
	return lock.Item{Target: i.Target, Entry: i.Entry, Kind: i.Kind, Path: i.Path, Link: i.Link, Tracked: tracked}
}
func materialize(t *testing.T, root string, l *lock.Lock, i lock.Item, trusted []string) {
	t.Helper()
	must(t, lock.ValidatePath(root, i.Path, trusted))
	obs, err := l.Status(root, i)
	must(t, err)
	if obs.State != lock.Missing {
		t.Fatalf("expected missing: %#v", obs)
	}
	p := filepath.Join(root, filepath.FromSlash(i.Path))
	must(t, os.MkdirAll(filepath.Dir(p), 0700))
	must(t, lock.ValidatePath(root, i.Path, trusted))
	must(t, os.Symlink(i.Link, p))
}
func persist(t *testing.T, root string, l *lock.Lock, i lock.Item) {
	t.Helper()
	must(t, l.Put(i))
	must(t, l.Save(root))
	must(t, lock.UpdateExclude(root, l.Items()))
}
func excludeFile(t *testing.T, root string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "rev-parse", "--git-path", "info/exclude").Output()
	must(t, err)
	p := strings.TrimSuffix(string(out), "\n")
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	return p
}
func assertStatus(t *testing.T, l *lock.Lock, root string, i lock.Item, state lock.State) {
	t.Helper()
	obs, err := l.Status(root, i)
	must(t, err)
	if obs.State != state || state == lock.OK && !obs.Converged {
		t.Fatalf("observation: %#v", obs)
	}
}

func TestFixtureOwnershipLifecycle(t *testing.T) {
	for _, withWorktree := range []bool{false, true} {
		name := "project"
		if withWorktree {
			name = "linked-worktree"
		}
		t.Run(name, func(t *testing.T) {
			f := testutil.NewFixture(t, withWorktree)
			r := registry(t)
			root := f.Project
			var mainLockBytes, mainSibling []byte
			if withWorktree {
				info, err := os.Lstat(filepath.Join(f.Worktree, ".git"))
				must(t, err)
				if !info.Mode().IsRegular() {
					t.Fatal("worktree .git is not a file")
				}
				mainItem := ownedItem(t, f.Project, claudeItem(t, renderFixture(t, f, f.Project, r)))
				mainLock, err := lock.Load(f.Project)
				must(t, err)
				materialize(t, f.Project, mainLock, mainItem, roots(r))
				persist(t, f.Project, mainLock, mainItem)
				write(t, filepath.Join(f.Project, ".claude/skills/handwritten/SKILL.md"), []byte("main handwritten\n"))
				mainLockBytes = read(t, filepath.Join(f.Project, ".daypack/lock.toml"))
				mainSibling = read(t, filepath.Join(f.Project, ".claude/skills/handwritten/SKILL.md"))
				root = f.Worktree
			}
			libraryBefore := testutil.SnapshotTree(t, f.Library)
			item := claudeItem(t, renderFixture(t, f, root, r))
			if item.Link != filepath.Join(f.Library, "skills/testing") || !filepath.IsAbs(item.Link) || item.Path != ".claude/skills/testing" {
				t.Fatal(item)
			}
			if _, err := os.Lstat(filepath.Join(root, ".agents")); !os.IsNotExist(err) {
				t.Fatalf("unexpected .agents: %v", err)
			}
			handwritten := filepath.Join(root, ".claude/skills/handwritten/SKILL.md")
			write(t, handwritten, []byte("handwritten\n"))
			l, err := lock.Load(root)
			must(t, err)
			i := ownedItem(t, root, item)
			if i.Tracked {
				t.Fatal("unexpected tracked skill")
			}
			materialize(t, root, l, i, roots(r))
			testutil.AssertTree(t, filepath.Join(root, ".claude"), map[string]testutil.TreeEntry{
				"skills": {Kind: "directory"}, "skills/testing": {Kind: "symlink", LinkTarget: item.Link}, "skills/handwritten": {Kind: "directory"}, "skills/handwritten/SKILL.md": {Kind: "file", Bytes: []byte("handwritten\n")},
			})
			link, err := os.Readlink(filepath.Join(root, item.Path))
			must(t, err)
			if link != item.Link {
				t.Fatal(link)
			}
			for file, want := range map[string]string{"SKILL.md": "---\nname: testing\ndescription: Offline test skill\n---\nUse support.txt.\n", "support.txt": "Test support.\n"} {
				if string(read(t, filepath.Join(root, item.Path, file))) != want {
					t.Fatalf("wrong %s bytes", file)
				}
			}
			persist(t, root, l, i)
			l, err = lock.Load(root)
			must(t, err)
			if !reflect.DeepEqual(l.Items(), []lock.Item{i}) {
				t.Fatal(l.Items())
			}
			exclude := excludeFile(t, root)
			excludeBefore := read(t, exclude)
			block, err := lock.ExcludeBlock(root, l.Items())
			must(t, err)
			expectedBlock := "# daypack begin " + root + "\n/.claude/skills/testing\n/.daypack/\n# daypack end " + root + "\n"
			if block != expectedBlock || !bytes.Contains(excludeBefore, []byte(block)) {
				t.Fatalf("unexpected excludes: %s", excludeBefore)
			}
			lockPath := filepath.Join(root, ".daypack/lock.toml")
			lockBefore := read(t, lockPath)
			for range 2 {
				assertStatus(t, l, root, i, lock.OK)
				must(t, l.Save(root))
				must(t, lock.UpdateExclude(root, l.Items()))
				if !bytes.Equal(lockBefore, read(t, lockPath)) || !bytes.Equal(excludeBefore, read(t, exclude)) {
					t.Fatal("non-idempotent persistence")
				}
			}
			var mainBlock string
			if withWorktree {
				mainBlock, err = lock.ExcludeBlock(f.Project, []lock.Item{i})
				must(t, err)
			}
			manifestPath := filepath.Join(root, "daypack.toml")
			write(t, manifestPath, bytes.Replace(read(t, manifestPath), []byte("skills = [\"testing\"]"), []byte("skills = []"), 1))
			result := renderFixture(t, f, root, r)
			if len(result.Items) != 0 {
				t.Fatal("removed skill still desired", result.Items)
			}
			must(t, l.RemoveOwned(root, i, roots(r)))
			must(t, l.Save(root))
			must(t, lock.UpdateExclude(root, l.Items()))
			if _, err := os.Lstat(filepath.Join(root, i.Path)); !os.IsNotExist(err) {
				t.Fatalf("link survives: %v", err)
			}
			l, err = lock.Load(root)
			must(t, err)
			if len(l.Items()) != 0 {
				t.Fatal("ownership survives")
			}
			emptyBlock, err := lock.ExcludeBlock(root, nil)
			must(t, err)
			afterExclude := read(t, exclude)
			if !bytes.Contains(afterExclude, []byte(emptyBlock)) || strings.Contains(emptyBlock, "/.claude/skills/testing") {
				t.Fatal("cleanup excludes")
			}
			if bytes.Contains(afterExclude, []byte(expectedBlock)) {
				t.Fatal("stale exclude block")
			}
			if string(read(t, handwritten)) != "handwritten\n" {
				t.Fatal("handwritten changed")
			}
			testutil.AssertTree(t, f.Library, libraryBefore)
			if withWorktree {
				if !bytes.Contains(afterExclude, []byte(mainBlock)) || !bytes.Equal(mainLockBytes, read(t, filepath.Join(f.Project, ".daypack/lock.toml"))) || !bytes.Equal(mainSibling, read(t, filepath.Join(f.Project, ".claude/skills/handwritten/SKILL.md"))) {
					t.Fatal("main ownership changed")
				}
				mainLock, err := lock.Load(f.Project)
				must(t, err)
				assertStatus(t, mainLock, f.Project, i, lock.OK)
			}
		})
	}
}

func TestFixtureRefusesCollisionAndDrift(t *testing.T) {
	for _, kind := range []string{"file", "directory", "different-link", "drifted-link"} {
		t.Run(kind, func(t *testing.T) {
			f := testutil.NewFixture(t, false)
			r := registry(t)
			item := claudeItem(t, renderFixture(t, f, f.Project, r))
			i := ownedItem(t, f.Project, item)
			l, err := lock.Load(f.Project)
			must(t, err)
			p := filepath.Join(f.Project, i.Path)
			must(t, os.MkdirAll(filepath.Dir(p), 0700))
			switch kind {
			case "file":
				write(t, p, []byte("handwritten"))
			case "directory":
				must(t, os.Mkdir(p, 0700))
			default:
				must(t, os.Symlink(filepath.Join(f.Library, "other"), p))
			}
			state := lock.Collision
			if kind == "drifted-link" {
				must(t, l.Put(i))
				state = lock.Drifted
			}
			projectBefore, libraryBefore := testutil.SnapshotTree(t, f.Project), testutil.SnapshotTree(t, f.Library)
			assertStatus(t, l, f.Project, i, state)
			if err := l.RemoveOwned(f.Project, i, roots(r)); err == nil {
				t.Fatal("unsafe removal allowed")
			}
			testutil.AssertTree(t, f.Project, projectBefore)
			testutil.AssertTree(t, f.Library, libraryBefore)
		})
	}
}
