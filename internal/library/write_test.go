package library

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

func mutationRoot(t *testing.T) string {
	t.Helper()
	// Resolve the operating system's temp alias before exercising confinement.
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", base)
	t.Setenv("DAYPACK_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", base)
	root := filepath.Join(base, "library")
	if err := Init(root); err != nil {
		t.Fatal(err)
	}
	return root
}
func writeNative(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func resolves(t *testing.T, root string, kind ArtifactType, name string) {
	t.Helper()
	l, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.Resolve(string(kind) + ":" + name); err != nil {
		t.Fatal(err)
	}
}
func TestMutationStubsAndImports(t *testing.T) {
	root := mutationRoot(t)
	for _, kind := range []ArtifactType{Skill, MCP, Fragment, Agent} {
		t.Run(string(kind), func(t *testing.T) {
			a, err := Add(root, AddOptions{Type: kind, Name: "shared"})
			if err != nil {
				t.Fatal(err)
			}
			resolves(t, root, kind, "shared")
			before := mustRead(t, contentPath(a.Path, kind))
			if _, err = Add(root, AddOptions{Type: kind, Name: "shared"}); err == nil {
				t.Fatal("duplicate accepted")
			}
			if !bytes.Equal(before, mustRead(t, contentPath(a.Path, kind))) {
				t.Fatal("duplicate changed bytes")
			}
			importedRoot := mutationRoot(t)
			if kind == Skill {
				if err = os.Mkdir(filepath.Join(a.Path, "support"), 0755); err != nil {
					t.Fatal(err)
				}
				writeNative(t, filepath.Join(a.Path, "support", "helper.txt"), []byte("support bytes"))
			}
			imported, err := Add(importedRoot, AddOptions{Type: kind, Name: "shared", Source: a.Path})
			if err != nil {
				t.Fatal(err)
			}
			resolves(t, importedRoot, kind, "shared")
			if kind == Skill && string(mustRead(t, filepath.Join(imported.Path, "support", "helper.txt"))) != "support bytes" {
				t.Fatal("missing support")
			}
			if _, err = Add(importedRoot, AddOptions{Type: kind, Name: "another", Source: a.Path}); (kind == Skill || kind == Agent) && err == nil {
				t.Fatal("mismatched frontmatter accepted")
			}
		})
	}
	// An unrelated invalid artifact does not prevent adding a valid one.
	writeNative(t, filepath.Join(root, "mcp", "bad.toml"), []byte("invalid ["))
	if _, err := Add(root, AddOptions{Type: Fragment, Name: "unrelated"}); err != nil {
		t.Fatal(err)
	}
}

func TestStubFrontmatterStrings(t *testing.T) {
	for _, kind := range []ArtifactType{Skill, Agent} {
		for _, name := range []string{"null", "-", "true", "123", "with:colon", "with: colon", `with"quote\slash`} {
			t.Run(string(kind)+"/"+name, func(t *testing.T) {
				data := stub(kind, name)
				front := bytes.Split(data, []byte("---\n"))[1]
				var fields map[string]any
				if err := yaml.Unmarshal(front, &fields); err != nil {
					t.Fatal(err)
				}
				if fields["name"] != name || fields["description"] != "New "+string(kind) {
					t.Fatalf("frontmatter did not preserve strings: %#v", fields)
				}
				if description, problems := metadata(data, "stub.md", kind, name); len(problems) != 0 || description != "New "+string(kind) {
					t.Fatalf("metadata: %q, %v", description, problems)
				}
			})
		}
	}
}

func TestMutationStubYAMLNames(t *testing.T) {
	for _, kind := range []ArtifactType{Skill, Agent} {
		for _, name := range []string{"null", "-", "true", "123"} {
			t.Run(string(kind)+"/"+name, func(t *testing.T) {
				root := mutationRoot(t)
				if _, err := Add(root, AddOptions{Type: kind, Name: name}); err != nil {
					t.Fatal(err)
				}
				resolves(t, root, kind, name)
				if problems, err := Doctor(root); err != nil || len(problems) != 0 {
					t.Fatalf("doctor: %v, %v", problems, err)
				}
			})
		}
	}
}
func TestInitRefusesExisting(t *testing.T) {
	root := mutationRoot(t)
	for _, path := range []string{root, filepath.Join(root, "empty"), filepath.Join(root, "file"), filepath.Join(root, "dangling")} {
		switch filepath.Base(path) {
		case "empty":
			if err := os.Mkdir(path, 0755); err != nil {
				t.Fatal(err)
			}
		case "file":
			writeNative(t, path, []byte("sentinel"))
		case "dangling":
			if err := os.Symlink(filepath.Join(root, "missing"), path); err != nil {
				t.Fatal(err)
			}
		}
		if err := Init(path); err == nil {
			t.Fatalf("overwrote %s", path)
		}
	}
	if string(mustRead(t, filepath.Join(root, ".gitignore"))) != "/secrets.env\n" {
		t.Fatal("ignore changed")
	}
	if _, err := os.Stat(filepath.Join(root, "secrets.env")); !os.IsNotExist(err) {
		t.Fatal("created secrets file")
	}
}
func TestMutationRejectedInputs(t *testing.T) {
	root := mutationRoot(t)
	for _, name := range []string{"../escape", "", "Upper", strings.Repeat("a", 65), "a/b", "with:colon", "with: colon"} {
		if _, err := Add(root, AddOptions{Type: Skill, Name: name}); err == nil {
			t.Fatalf("accepted %q", name)
		}
		if err := Remove(root, Skill, name); err == nil {
			t.Fatalf("removed %q", name)
		}
	}
	for _, opts := range []AddOptions{{Type: "unknown", Name: "a"}, {Type: Skill, Name: "missing", Source: filepath.Join(root, "missing")}, {Type: Skill, Name: "wrong", Source: filepath.Join(root, ".gitignore")}} {
		if _, err := Add(root, opts); err == nil {
			t.Fatal("accepted bad input")
		}
	}
	if _, err := Add(filepath.Join(root, "missing"), AddOptions{Type: Agent, Name: "a"}); err == nil {
		t.Fatal("missing library accepted")
	}
	// An invalid import must leave neither a destination nor staging debris.
	source := filepath.Join(root, "bad.md")
	writeNative(t, source, []byte("---\nname: wrong\n---\n"))
	if _, err := Add(root, AddOptions{Type: Agent, Name: "bad", Source: source}); err == nil {
		t.Fatal("bad import accepted")
	}
	entries, err := os.ReadDir(filepath.Join(root, "agents"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("failed import left files")
	}
}
func TestMutationSymlinks(t *testing.T) {
	root := mutationRoot(t)
	outside := filepath.Join(filepath.Dir(root), "outside")
	if err := os.Mkdir(outside, 0755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outside, "SKILL.md")
	writeNative(t, sentinel, stub(Skill, "linked"))
	dest := filepath.Join(root, "skills", "linked")
	if err := os.Symlink(outside, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(root, AddOptions{Type: Skill, Name: "linked"}); err == nil {
		t.Fatal("destination symlink accepted")
	}
	if err := Remove(root, Skill, "linked"); err == nil {
		t.Fatal("removed symlink target")
	}
	if _, err := EditPath(root, Skill, "linked"); err == nil {
		t.Fatal("edited symlink")
	}
	if _, err := Add(root, AddOptions{Type: Skill, Name: "imported", Source: dest}); err == nil {
		t.Fatal("source symlink accepted")
	}
	if err := os.Symlink(sentinel, filepath.Join(outside, "support")); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(root, AddOptions{Type: Skill, Name: "linked", Source: outside}); err == nil {
		t.Fatal("duplicate accepted")
	}
	if _, err := Add(root, AddOptions{Type: Skill, Name: "nested", Source: outside}); err == nil {
		t.Fatal("nested source symlink accepted")
	}
	if err := os.Remove(filepath.Join(root, "agents")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "agents")); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(root, AddOptions{Type: Agent, Name: "a"}); err == nil {
		t.Fatal("parent symlink accepted")
	}
	if !bytes.Equal(mustRead(t, sentinel), stub(Skill, "linked")) {
		t.Fatal("outside changed")
	}
}
func TestRenamePreservesNativeBytes(t *testing.T) {
	for _, kind := range []ArtifactType{Skill, Agent, MCP, Fragment} {
		t.Run(string(kind), func(t *testing.T) {
			root := mutationRoot(t)
			a, err := Add(root, AddOptions{Type: kind, Name: "old"})
			if err != nil {
				t.Fatal(err)
			}
			data := []byte("---\r\n# Keep this\r\nname: 'old' # identity\r\ndescription: Useful description\r\ncustom: [one, two]\r\n---\r\n\r\nBody stays.\r\n")
			var want []byte
			if kind == Skill || kind == Agent {
				writeNative(t, contentPath(a.Path, kind), data)
				want = bytes.Replace(data, []byte("'old'"), []byte("'new'"), 1)
			} else {
				want = mustRead(t, a.Path)
			}
			if kind == Skill {
				writeNative(t, filepath.Join(a.Path, "support.txt"), []byte("unchanged"))
			}
			renamed, err := Rename(root, kind, "old", "new")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(want, mustRead(t, contentPath(renamed.Path, kind))) {
				t.Fatal("native bytes changed unexpectedly")
			}
			if _, err = os.Lstat(a.Path); !os.IsNotExist(err) {
				t.Fatal("old artifact remains")
			}
			if kind == Skill && string(mustRead(t, filepath.Join(renamed.Path, "support.txt"))) != "unchanged" {
				t.Fatal("support changed")
			}
			resolves(t, root, kind, "new")
			sibling := filepath.Join(root, "handwritten.txt")
			writeNative(t, sibling, []byte("keep"))
			if err = Remove(root, kind, "new"); err != nil {
				t.Fatal(err)
			}
			if string(mustRead(t, sibling)) != "keep" {
				t.Fatal("sibling removed")
			}
		})
	}
}
func TestRenameYAMLNameShapes(t *testing.T) {
	cases := []struct {
		name, front, want string
	}{
		{"tag", "name: !!str old # keep\ndescription: Useful\n", "name: !!str new # keep\ndescription: Useful\n"},
		{"verbatim tag", "name: !<tag:yaml.org,2002:str> old\ndescription: Useful\n", "name: !<tag:yaml.org,2002:str> new\ndescription: Useful\n"},
		{"custom tag", "name: !identity old\ndescription: Useful\n", "name: !identity new\ndescription: Useful\n"},
		{"anchor", "name: &identity old\ndescription: Useful\n", "name: &identity new\ndescription: Useful\n"},
		{"tag and anchor", "name: !!str &identity 'old'\ndescription: Useful\n", "name: !!str &identity 'new'\ndescription: Useful\n"},
		{"anchor and tag", "name: &identity !!str old\ndescription: Useful\n", "name: &identity !!str new\ndescription: Useful\n"},
		{"multiline properties", "name: !!str\n  &identity old\ndescription: Useful\n", "name: !!str\n  &identity new\ndescription: Useful\n"},
		{"property comment", "name: &identity # keep\n  old\ndescription: Useful\n", "name: &identity # keep\n  new\ndescription: Useful\n"},
		{"literal block", "name: |- # keep\n  old\ndescription: Useful\n", "name: new # keep\ndescription: Useful\n"},
		{"folded block", "name: >-\n  old\ndescription: Useful\n", "name: new\ndescription: Useful\n"},
		{"explicit block indent", "name: &identity !!str |2-\n  old\ndescription: Useful\n", "name: &identity !!str new\ndescription: Useful\n"},
		{"multiline quoted", "name: \"o\\\n  ld\"\ndescription: Useful\n", "name: \"new\"\ndescription: Useful\n"},
		{"alias", "custom: &identity old\nname: *identity\ndescription: Useful\n", "custom: &identity old\nname: new\ndescription: Useful\n"},
		{"merge", "custom: &defaults {name: old, description: Useful}\n<<: *defaults\n", "name: new\ncustom: &defaults {name: old, description: Useful}\n<<: *defaults\n"},
		{"flow merge", "{custom: &defaults {name: old, description: Useful}, <<: *defaults}\n", "{name: new, custom: &defaults {name: old, description: Useful}, <<: *defaults}\n"},
		{"flow", "{name: !!str &identity old, description: Useful}\n", "{name: !!str &identity new, description: Useful}\n"},
		{"unicode before value", "{custom: café, name: !!str old, description: Useful}\n", "{custom: café, name: !!str new, description: Useful}\n"},
	}
	for _, tc := range cases {
		for _, kind := range []ArtifactType{Skill, Agent} {
			for _, newline := range []string{"\n", "\r\n"} {
				t.Run(tc.name+"/"+string(kind)+"/"+fmt.Sprintf("%q", newline), func(t *testing.T) {
					root := mutationRoot(t)
					a, err := Add(root, AddOptions{Type: kind, Name: "old"})
					if err != nil {
						t.Fatal(err)
					}
					document := func(front string) []byte {
						return []byte(strings.ReplaceAll("---\n"+front+"---\n\nBody stays.\n", "\n", newline))
					}
					writeNative(t, contentPath(a.Path, kind), document(tc.front))
					assertDoctorClean := func() {
						t.Helper()
						problems, err := Doctor(root)
						if err != nil || len(problems) != 0 {
							t.Fatalf("doctor: %v, %v", problems, err)
						}
					}
					assertDoctorClean()
					renamed, err := Rename(root, kind, "old", "new")
					if err != nil {
						t.Fatal(err)
					}
					if got, want := mustRead(t, contentPath(renamed.Path, kind)), document(tc.want); !bytes.Equal(got, want) {
						t.Fatalf("got %q, want %q", got, want)
					}
					if _, err := os.Lstat(a.Path); !os.IsNotExist(err) {
						t.Fatalf("old path remains: %v", err)
					}
					resolves(t, root, kind, "new")
					assertDoctorClean()
				})
			}
		}
	}
}

func TestRenameTypedNamesAndNullDestination(t *testing.T) {
	for _, tc := range []struct{ tag, oldName string }{
		{"!!int", "123"}, {"!!bool", "true"}, {"!!float", "123"},
		{"!<tag:yaml.org,2002:int>", "123"}, {"!!str", "old"}, {"", "old"},
	} {
		for _, newName := range []string{"new", "null", "-"} {
			t.Run(tc.tag+"/"+newName, func(t *testing.T) {
				root := mutationRoot(t)
				a, err := Add(root, AddOptions{Type: Agent, Name: tc.oldName})
				if err != nil {
					t.Fatal(err)
				}
				front := "name: " + tc.tag + " &identity " + tc.oldName + " # keep\ndescription: Useful\n"
				writeNative(t, a.Path, []byte("---\n"+front+"---\nBody stays.\n"))
				if problems, err := Doctor(root); err != nil || len(problems) != 0 {
					t.Fatalf("doctor before rename: %v, %v", problems, err)
				}
				if _, err := Rename(root, Agent, tc.oldName, newName); err != nil {
					t.Fatal(err)
				}
				resolves(t, root, Agent, newName)
				if problems, err := Doctor(root); err != nil || len(problems) != 0 {
					t.Fatalf("doctor after rename: %v, %v", problems, err)
				}
			})
		}
	}
}

func TestMalformedRepairAndRenameFailure(t *testing.T) {
	root := mutationRoot(t)
	a, err := Add(root, AddOptions{Type: Agent, Name: "broken"})
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("---\nname: [bad yaml\n---\n")
	writeNative(t, a.Path, data)
	if _, err = Rename(root, Agent, "broken", "new"); err == nil {
		t.Fatal("renamed malformed metadata")
	}
	if !bytes.Equal(data, mustRead(t, a.Path)) {
		t.Fatal("failed rename changed original")
	}
	if _, err = EditPath(root, Agent, "broken"); err != nil {
		t.Fatal(err)
	}
	if err = Remove(root, Agent, "broken"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if _, err = Add(root, AddOptions{Type: Skill, Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = Rename(root, Skill, "one", "two"); err == nil {
		t.Fatal("rename collision accepted")
	}
	resolves(t, root, Skill, "one")
	resolves(t, root, Skill, "two")
	if _, err = Rename(root, Skill, "one", "one"); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentAddDoesNotOverwrite(t *testing.T) {
	root := mutationRoot(t)
	for _, kind := range []ArtifactType{Skill, Agent} {
		var wg sync.WaitGroup
		results := make(chan error, 12)
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := Add(root, AddOptions{Type: kind, Name: "raced"}); results <- err }()
		}
		wg.Wait()
		close(results)
		successes := 0
		for err := range results {
			if err == nil {
				successes++
			}
		}
		if successes != 1 {
			t.Fatalf("%s: got %d successful additions", kind, successes)
		}
		resolves(t, root, kind, "raced")
	}
}
func TestFailedImportPreservesDestinationAndSource(t *testing.T) {
	root := mutationRoot(t)
	destination := filepath.Join(root, "agents", "broken.md")
	data := []byte("malformed existing artifact")
	writeNative(t, destination, data)
	if _, err := Add(root, AddOptions{Type: Agent, Name: "broken"}); err == nil {
		t.Fatal("overwrote invalid duplicate")
	}
	if !bytes.Equal(mustRead(t, destination), data) {
		t.Fatal("changed invalid duplicate")
	}
	source := filepath.Join(root, "source.md")
	data = stub(Agent, "identity")
	writeNative(t, source, data)
	if _, err := Add(root, AddOptions{Type: Agent, Name: "mismatch", Source: source}); err == nil {
		t.Fatal("accepted mismatch")
	}
	if !bytes.Equal(mustRead(t, source), data) {
		t.Fatal("changed source")
	}
	if _, err := os.Stat(filepath.Join(root, "agents", "mismatch.md")); !os.IsNotExist(err) {
		t.Fatal("published failed import")
	}
}
