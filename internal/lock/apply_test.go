package lock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMCPIdentityBackwardCompatible(t *testing.T) {
	root := t.TempDir()
	l := New()
	item := Item{Target: "mcp", Entry: "mcp:docs", Kind: JSONEntry, Path: ".mcp.json", Key: "mcpServers.docs", Hash: HashBytes([]byte("{}"))}
	if err := l.Put(item); err != nil {
		t.Fatal(err)
	}
	if err := l.Save(root); err != nil {
		t.Fatal(err)
	}
	old, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if old.Items()[0].MCPIdentity != "" {
		t.Fatal("legacy consent must remain unverified")
	}
	item.MCPIdentity = HashBytes([]byte("executable identity"))
	if err = l.Put(item); err != nil {
		t.Fatal(err)
	}
	if err = l.Save(root); err != nil {
		t.Fatal(err)
	}
	current, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if current.Items()[0] != item || current.Version != 1 {
		t.Fatal("optional metadata did not round trip")
	}
}
func TestForceRemovalKeepsGuards(t *testing.T) {
	for _, kind := range []Kind{File, Symlink, JSONEntry, MDSection} {
		t.Run(string(kind), func(t *testing.T) {
			root := t.TempDir()
			l := New()
			item := Item{Target: "target", Entry: "agent:reviewer", Kind: kind, Path: "managed", Hash: HashBytes([]byte("original")), Link: "original", Key: "server", Marker: "test"}
			var err error
			switch kind {
			case Symlink:
				err = os.Symlink("changed", filepath.Join(root, item.Path))
			case JSONEntry:
				err = os.WriteFile(filepath.Join(root, item.Path), []byte(`{"server":"changed","handwritten":true}`), 0644)
			case MDSection:
				err = os.WriteFile(filepath.Join(root, item.Path), []byte("handwritten\n<!-- daypack:begin test -->changed<!-- daypack:end test -->\n"), 0644)
			default:
				err = os.WriteFile(filepath.Join(root, item.Path), []byte("changed"), 0644)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = l.Put(item); err != nil {
				t.Fatal(err)
			}
			if err = l.RemoveOwned(root, item, []string{"managed"}); err == nil {
				t.Fatal("ordinary removal allowed drift")
			}
			if err = l.RemoveOwnedWithForce(root, item, []string{"managed"}, true); err != nil {
				t.Fatal(err)
			}
			if len(l.Items()) != 0 {
				t.Fatal("successful removal still owned")
			}
			if kind == JSONEntry || kind == MDSection {
				data, err := os.ReadFile(filepath.Join(root, item.Path))
				if err != nil || len(data) == 0 {
					t.Fatal("shared handwritten content lost")
				}
			}
		})
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "sentinel"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "managed")); err != nil {
		t.Fatal(err)
	}
	l := New()
	item := Item{Target: "mcp", Entry: "mcp:docs", Kind: JSONEntry, Path: "managed", Key: "server"}
	if err := l.Put(item); err != nil {
		t.Fatal(err)
	}
	if err := l.RemoveOwnedWithForce(root, item, []string{"managed"}, true); err == nil {
		t.Fatal("force followed shared container symlink")
	}
}
func TestWriteSymlinkIdempotentAndSafe(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteSymlink(root, "links/skill", []string{"links"}, target); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(filepath.Join(root, "links/skill"))
	if err != nil {
		t.Fatal(err)
	}
	if err = WriteSymlink(root, "links/skill", []string{"links"}, target); err != nil {
		t.Fatal(err)
	}
	after, err := os.Lstat(filepath.Join(root, "links/skill"))
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("identical link rewritten")
	}
	if err = os.Mkdir(filepath.Join(root, "links/directory"), 0755); err != nil {
		t.Fatal(err)
	}
	if err = WriteSymlink(root, "links/directory", []string{"links"}, target); err == nil {
		t.Fatal("directory replaced")
	}
	if err = WriteSymlink(root, "outside/skill", []string{"links"}, target); err == nil {
		t.Fatal("untrusted root accepted")
	}
}
