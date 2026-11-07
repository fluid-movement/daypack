package lock

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func sandbox(t *testing.T) string {
	t.Helper()
	r := t.TempDir()
	for _, k := range []string{"HOME", "DAYPACK_HOME", "XDG_CONFIG_HOME"} {
		t.Setenv(k, t.TempDir())
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_COUNT", "0")
	return r
}
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func write(t *testing.T, root, p string, b []byte) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0700))
	must(t, os.WriteFile(filepath.Join(root, p), b, 0600))
}
func read(t *testing.T, root, p string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(root, p))
	must(t, e)
	return b
}
func git(t *testing.T, r string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r}, args...)...)
	b, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("git %v: %s: %v", args, b, e)
	}
	return strings.TrimSpace(string(b))
}
func fixtureItems() []Item {
	return []Item{
		{Target: "skills", Entry: "skill:x", Kind: Symlink, Path: ".claude/skills/x", Link: "/external/skills/x"},
		{Kind: File, Path: "generated", Hash: HashBytes([]byte("content"))},
		{Kind: JSONEntry, Path: ".mcp.json", File: ".mcp.json", Key: "mcpServers.x", Hash: HashBytes([]byte("{}")), Tracked: true},
		{Kind: JSONArrayElement, Path: "settings.json", Key: "enabled", Value: "x", Preserve: true},
		{Kind: MDSection, Path: "AGENTS.md", Marker: "instructions", Hash: HashBytes([]byte("body")), Tracked: true},
	}
}
func TestPersistence(t *testing.T) {
	r := sandbox(t)
	l, e := Load(r)
	must(t, e)
	if l.Version != 1 || len(l.Items()) != 0 {
		t.Fatal(l)
	}
	for _, i := range fixtureItems() {
		must(t, l.Put(i))
	}
	must(t, l.Save(r))
	before := read(t, r, ".daypack/lock.toml")
	l2, e := Load(r)
	must(t, e)
	if !reflect.DeepEqual(l.Items(), l2.Items()) {
		t.Fatal("round trip")
	}
	must(t, l2.Save(r))
	if !bytes.Equal(before, read(t, r, ".daypack/lock.toml")) {
		t.Fatal("save not stable")
	}
	items := l2.Items()
	items[0].Path = "mutated"
	if l2.Items()[0].Path == "mutated" {
		t.Fatal("Items aliases storage")
	}
	s, e := os.Stat(filepath.Join(r, ".daypack/lock.toml"))
	must(t, e)
	if s.Mode().Perm() != 0600 {
		t.Fatal(s.Mode())
	}
	for _, data := range []string{"", "version=2", "invalid toml", "version=1\nunknown=true", "version=1\n[[item]]\nkind='file'\npath='../escape'", "version=1\n[[item]]\nkind='file'\npath='x'\n[[item]]\nkind='file'\npath='x'"} {
		write(t, r, ".daypack/lock.toml", []byte(data))
		if _, e := Load(r); e == nil {
			t.Fatalf("accepted %q", data)
		}
	}
}
func TestOwnershipIdentities(t *testing.T) {
	sandbox(t)
	l := New()
	a := Item{Kind: JSONEntry, Path: "shared", Key: "a"}
	must(t, l.Put(a))
	must(t, l.Put(Item{Kind: JSONEntry, Path: "shared", Key: "b"}))
	for _, i := range []Item{{Kind: File, Path: "shared"}, {Kind: JSONEntry, Path: "shared", Key: "a.child"}, {Kind: MDSection, Path: "shared", Marker: "x"}, {Kind: JSONEntry, Path: "x", File: "different", Key: "a"}} {
		if e := l.Put(i); e == nil {
			t.Fatalf("accepted %#v", i)
		}
	}
	l.Forget(a)
	if _, ok := l.Find(a); ok {
		t.Fatal("not forgotten")
	}
}
func TestStatusKinds(t *testing.T) {
	r := sandbox(t)
	cases := []struct {
		i                Item
		initial, desired []byte
	}{
		{Item{Kind: File, Path: "file"}, []byte("old"), []byte("new")},
		{Item{Kind: JSONEntry, Path: "json", Key: "owned"}, []byte(`{"owned":{"b":2,"a":1}}`), []byte(`{"a":2}`)},
		{Item{Kind: MDSection, Path: "md", Marker: "instructions", Tracked: true}, []byte("<!-- daypack:begin instructions -->old<!-- daypack:end instructions -->"), []byte("new")},
	}
	for _, c := range cases {
		t.Run(string(c.i.Kind), func(t *testing.T) {
			i := c.i
			if i.Kind == JSONEntry {
				var err error
				i.Hash, err = HashJSON(c.desired)
				must(t, err)
			} else {
				i.Hash = HashBytes(c.desired)
			}
			l := New()
			o, e := l.Status(r, i)
			must(t, e)
			if o.State != Missing {
				t.Fatal(o)
			}
			write(t, r, i.Path, c.initial)
			o, e = l.Status(r, i)
			must(t, e)
			if o.State != Collision {
				t.Fatal(o)
			}
			old := i
			switch i.Kind {
			case File:
				old.Hash = HashBytes(c.initial)
			case JSONEntry:
				old.Hash, e = HashJSON([]byte(`{"a":1,"b":2}`))
				must(t, e)
			case MDSection:
				old.Hash = HashBytes([]byte("old"))
			}
			must(t, l.Put(old))
			o, e = l.Status(r, i)
			must(t, e)
			if o.State != OK || o.Converged {
				t.Fatal(o)
			}
			var updated []byte
			switch i.Kind {
			case File:
				updated = c.desired
			case JSONEntry:
				updated, e = SetJSON(c.initial, i, c.desired)
			case MDSection:
				updated, e = SetSection(c.initial, i.Marker, c.desired)
			}
			must(t, e)
			write(t, r, i.Path, updated)
			o, e = l.Status(r, i)
			must(t, e)
			if !o.Converged || o.Adopt {
				t.Fatal(o)
			}
			o, e = New().Status(r, i)
			must(t, e)
			if !o.Converged || !o.Adopt {
				t.Fatal(o)
			}
			write(t, r, i.Path, bytes.ReplaceAll(updated, []byte("new"), []byte("edited")))
			if i.Kind == JSONEntry {
				write(t, r, i.Path, []byte(`{"owned":false}`))
			}
			o, e = l.Status(r, i)
			must(t, e)
			if o.State != Drifted {
				t.Fatal(o)
			}
		})
	}
	i := Item{Kind: Symlink, Path: "link", Link: "/external/x"}
	must(t, os.Symlink(i.Link, filepath.Join(r, i.Path)))
	o, e := New().Status(r, i)
	must(t, e)
	if !o.Adopt || !o.Converged {
		t.Fatal(o)
	}
	l := New()
	must(t, l.Put(i))
	must(t, os.Remove(filepath.Join(r, i.Path)))
	must(t, os.Symlink("/different", filepath.Join(r, i.Path)))
	o, e = l.Status(r, i)
	must(t, e)
	if o.State != Drifted {
		t.Fatal(o)
	}
}
func TestSymlinkStatusStates(t *testing.T) {
	for _, owned := range []bool{false, true} {
		for _, current := range []string{"missing", "desired", "different", "regular-file"} {
			t.Run(fmt.Sprintf("owned=%t/%s", owned, current), func(t *testing.T) {
				r := sandbox(t)
				i := Item{Kind: Symlink, Path: "link", Link: "/external/desired"}
				l := New()
				if owned {
					must(t, l.Put(i))
				}
				want := Observation{State: Missing}
				switch current {
				case "desired":
					must(t, os.Symlink(i.Link, filepath.Join(r, i.Path)))
					want = Observation{State: OK, Converged: true, Adopt: !owned}
				case "different", "regular-file":
					if current == "different" {
						must(t, os.Symlink("/external/different", filepath.Join(r, i.Path)))
					} else {
						write(t, r, i.Path, []byte("handwritten"))
					}
					want.State = Collision
					if owned {
						want.State = Drifted
					}
				}
				got, err := l.Status(r, i)
				must(t, err)
				if got != want {
					t.Fatalf("got %+v, want %+v", got, want)
				}
			})
		}
	}
}

func TestArrayElementStatusStates(t *testing.T) {
	for _, owned := range []bool{false, true} {
		for _, current := range []string{"missing-file", "missing-key", "missing-member", "present", "symlink", "directory"} {
			t.Run(fmt.Sprintf("owned=%t/%s", owned, current), func(t *testing.T) {
				r := sandbox(t)
				i := Item{Kind: JSONArrayElement, Path: "settings.json", Key: "enabled", Value: "mine"}
				l := New()
				if owned {
					must(t, l.Put(i))
				}
				want := Observation{State: Missing}
				switch current {
				case "missing-key":
					write(t, r, i.Path, []byte(`{"other":true}`))
				case "missing-member":
					write(t, r, i.Path, []byte(`{"enabled":["other"]}`))
				case "present":
					write(t, r, i.Path, []byte(`{"enabled":["mine","other"]}`))
					want = Observation{State: OK, Converged: true, Adopt: !owned, Preserve: !owned}
				case "symlink", "directory":
					if current == "symlink" {
						must(t, os.Symlink("/external/settings", filepath.Join(r, i.Path)))
					} else {
						must(t, os.Mkdir(filepath.Join(r, i.Path), 0700))
					}
					want.State = Collision
					if owned {
						want.State = Drifted
					}
				}
				got, err := l.Status(r, i)
				must(t, err)
				if got != want {
					t.Fatalf("got %+v, want %+v", got, want)
				}
			})
		}
	}
}

func TestCollisionRefused(t *testing.T) {
	r := sandbox(t)
	i := Item{Kind: File, Path: "manual", Hash: HashBytes([]byte("desired"))}
	write(t, r, i.Path, []byte("mine"))
	o, e := New().Status(r, i)
	must(t, e)
	if o.State != Collision {
		t.Fatal(o)
	}
	if e := New().RemoveOwned(r, i, []string{"manual"}); e == nil {
		t.Fatal("removed unowned")
	}
	if string(read(t, r, "manual")) != "mine" {
		t.Fatal("changed handwritten")
	}
}
func TestManagedDriftReported(t *testing.T) {
	r := sandbox(t)
	i := Item{Kind: File, Path: "owned", Hash: HashBytes([]byte("old"))}
	l := New()
	must(t, l.Put(i))
	write(t, r, i.Path, []byte("edited"))
	o, e := l.Status(r, i)
	must(t, e)
	if o.State != Drifted {
		t.Fatal(o)
	}
	if e := l.RemoveOwned(r, i, []string{"owned"}); e == nil {
		t.Fatal("removed drift")
	}
}
func TestHandwrittenFilesAndJSONKeysSurvive(t *testing.T) {
	r := sandbox(t)
	data, e := os.ReadFile("testdata/shared.json")
	must(t, e)
	i := Item{Kind: JSONEntry, Path: "shared.json", Key: `mcpServers.managed\.name`}
	value := []byte(`{"command":"new"}`)
	i.Hash, e = HashJSON(value)
	must(t, e)
	out, e := SetJSON(data, i, value)
	must(t, e)
	want := bytes.Replace(data, []byte(`{ "command": "old" }`), value, 1)
	if !bytes.Equal(out, want) {
		t.Fatalf("unexpected diff\n%s", out)
	}
	write(t, r, i.Path, out)
	write(t, r, "sibling", []byte("mine"))
	l := New()
	must(t, l.Put(i))
	must(t, l.RemoveOwned(r, i, []string{"shared.json"}))
	remaining := read(t, r, i.Path)
	if !bytes.Contains(remaining, []byte(`"handwritten" : { "keep": true }`)) || !bytes.Contains(remaining, []byte(`"manual": {"command":"mine"}`)) {
		t.Fatal(string(remaining))
	}
	if string(read(t, r, "sibling")) != "mine" {
		t.Fatal("sibling changed")
	}
}
func TestArrayPreservation(t *testing.T) {
	r := sandbox(t)
	i := Item{Kind: JSONArrayElement, Path: "settings", Key: "enabled", Value: "mine"}
	data := []byte(`{"enabled":["mine","other"]}`)
	write(t, r, i.Path, data)
	l := New()
	o, e := l.Status(r, i)
	must(t, e)
	if !o.Adopt || !o.Preserve {
		t.Fatal(o)
	}
	i.Preserve = o.Preserve
	must(t, l.Put(i))
	must(t, l.Save(r))
	l, e = Load(r)
	must(t, e)
	must(t, l.RemoveOwned(r, i, []string{i.Path}))
	if !bytes.Equal(data, read(t, r, i.Path)) {
		t.Fatal("user membership removed")
	}
	i.Value = "new"
	i.Preserve = false
	out, e := SetJSON(data, i, nil)
	must(t, e)
	write(t, r, i.Path, out)
	must(t, l.Put(i))
	must(t, l.RemoveOwned(r, i, []string{i.Path}))
	if !bytes.Equal(data, read(t, r, i.Path)) {
		t.Fatal(string(read(t, r, i.Path)))
	}
	for _, b := range []string{`{"enabled":true}`, `{"enabled":["mine","mine"]}`, `{"enabled":[1]}`} {
		if _, e := SetJSON([]byte(b), Item{Kind: JSONArrayElement, Key: "enabled", Value: "mine"}, nil); e == nil {
			t.Fatal("accepted invalid array")
		}
	}
}
func TestJSONFormattingAndValidation(t *testing.T) {
	sandbox(t)
	for _, c := range []struct{ data, key, value, want string }{
		{"{\n\t\"a\": 1\n}\n", "b", "2", "{\n\t\"a\": 1,\n\t\"b\": 2\n}\n"},
		{`{"a":{"b":1},"unknown":true}`, "a.c.d", "2", `{"a":{"b":1, "c":{"d":2}},"unknown":true}`},
		{`{"x.y":{"*":1}}`, `x\.y.*`, "2", `{"x.y":{"*":2}}`},
		{`{"a":1}`, "b", "2", `{"a":1, "b":2}`},
	} {
		out, e := SetJSON([]byte(c.data), Item{Kind: JSONEntry, Key: c.key}, []byte(c.value))
		must(t, e)
		if string(out) != c.want {
			t.Fatalf("got %s want %s", out, c.want)
		}
	}
	for _, b := range []string{`{"a":1,"a":2}`, `{"a":true} junk`, `{"a":[1]}`} {
		if _, e := SetJSON([]byte(b), Item{Kind: JSONEntry, Key: "a.child"}, []byte("1")); e == nil {
			t.Fatalf("accepted %s", b)
		}
	}
	h, e := HashJSON([]byte(`{"z":9007199254740993,"a":[1,2]}`))
	must(t, e)
	h2, e := HashJSON([]byte(`{ "a": [1,2], "z": 9007199254740993 }`))
	must(t, e)
	if h != h2 {
		t.Fatal("not canonical")
	}
	h3, e := HashJSON([]byte(`{"z":9007199254740992,"a":[1,2]}`))
	must(t, e)
	if h3 == h {
		t.Fatal("lost precision")
	}
	for _, b := range []string{"", "{}{}", "{} x"} {
		if _, e := HashJSON([]byte(b)); e == nil {
			t.Fatal("invalid hash input")
		}
	}
}
func TestMarkdown(t *testing.T) {
	sandbox(t)
	original := []byte("before\n<!-- daypack:begin instructions -->old<!-- daypack:end instructions -->\nafter")
	out, e := SetSection(original, "instructions", []byte("new"))
	must(t, e)
	if !bytes.Equal(out, bytes.Replace(original, []byte("old"), []byte("new"), 1)) {
		t.Fatal("external text changed")
	}
	out, e = RemoveSection(out, "instructions")
	must(t, e)
	if string(out) != "before\n\nafter" {
		t.Fatal(string(out))
	}
	for _, b := range []string{"<!-- daypack:end x -->", "<!-- daypack:begin x -->", "<!-- daypack:begin x --><!-- daypack:begin y --><!-- daypack:end y --><!-- daypack:end x -->", "<!-- daypack:begin x --><!-- daypack:end x --><!-- daypack:begin x --><!-- daypack:end x -->"} {
		if _, e := SetSection([]byte(b), "x", nil); e == nil {
			t.Fatal("accepted malformed markers")
		}
	}
	for _, name := range []string{"", "x\ny", "x -->"} {
		if _, e := SetSection(nil, name, nil); e == nil {
			t.Fatal("accepted marker")
		}
	}
}
func TestPathSafety(t *testing.T) {
	r := sandbox(t)
	for _, p := range []string{"../outside", "a/../outside", "/absolute", "a/../../outside", "target-evil/x", ""} {
		if e := ValidatePath(r, p, []string{"target"}); e == nil {
			t.Fatalf("accepted %q", p)
		}
	}
	must(t, ValidatePath(r, "target/x", []string{"target"}))
	must(t, ValidatePath(r, ".daypack/lock.toml", nil))
	must(t, os.Symlink(t.TempDir(), filepath.Join(r, "target")))
	if e := ValidatePath(r, "target/x", []string{"target"}); e == nil {
		t.Fatal("symlink parent allowed")
	}
	must(t, os.Symlink(t.TempDir(), filepath.Join(r, ".daypack")))
	if _, e := Load(r); e == nil {
		t.Fatal("symlink .daypack allowed")
	}
	if e := New().Save(r); e == nil {
		t.Fatal("saved through symlink")
	}
}
func TestTrackedLockRefused(t *testing.T) {
	r := sandbox(t)
	git(t, r, "init", "-q")
	write(t, r, ".daypack/lock.toml", []byte("version=1\n"))
	git(t, r, "add", ".daypack/lock.toml")
	if e := New().Save(r); e == nil {
		t.Fatal("tracked lock allowed")
	}
	if _, e := Load(r); e == nil {
		t.Fatal("tracked lock loaded")
	}
}
func TestAtomicFailure(t *testing.T) {
	r := sandbox(t)
	p := filepath.Join(r, "file")
	must(t, os.WriteFile(p, []byte("old"), 0600))
	failure := errors.New("injected")
	for _, phase := range []string{"write", "rename"} {
		e := atomicWrite(p, []byte("new"), 0600, func(f *os.File, b []byte) error {
			if phase == "write" {
				return failure
			}
			_, e := f.Write(b)
			return e
		}, func(a, b string) error {
			if filepath.Dir(a) != filepath.Dir(b) {
				t.Fatal("different temp dir")
			}
			return failure
		})
		if !errors.Is(e, failure) {
			t.Fatal(e)
		}
		if string(read(t, r, "file")) != "old" {
			t.Fatal("partial replacement")
		}
		files, e := os.ReadDir(r)
		must(t, e)
		if len(files) != 1 {
			t.Fatal("temp leaked")
		}
	}
	must(t, writeAtomic(p, []byte("new"), 0600))
	before, e := os.Stat(p)
	must(t, e)
	must(t, writeAtomic(p, []byte("new"), 0600))
	after, e := os.Stat(p)
	must(t, e)
	if !os.SameFile(before, after) {
		t.Fatal("identical bytes rewritten")
	}
}

func TestInvalidContainersAreErrors(t *testing.T) {
	r := sandbox(t)
	for _, i := range []Item{{Kind: JSONEntry, Path: "json", Key: "a"}, {Kind: JSONArrayElement, Path: "array", Key: "a", Value: "x"}, {Kind: MDSection, Path: "md", Marker: "instructions"}} {
		write(t, r, i.Path, []byte("<!-- daypack:begin instructions -->"))
		if _, e := New().Status(r, i); e == nil {
			t.Fatal("invalid container treated as missing")
		}
	}
	i := Item{Kind: File, Path: "link", Hash: HashBytes([]byte("secret"))}
	must(t, os.Symlink("/outside/project", filepath.Join(r, i.Path)))
	o, e := New().Status(r, i)
	must(t, e)
	if o.State != Collision {
		t.Fatal(o)
	}
}
func TestIndependentSectionsAndArrayMembers(t *testing.T) {
	r := sandbox(t)
	l := New()
	for _, i := range []Item{{Kind: MDSection, Path: "AGENTS.md", Marker: "a"}, {Kind: MDSection, Path: "AGENTS.md", Marker: "b"}, {Kind: JSONArrayElement, Path: "settings.json", Key: "enabled", Value: "a"}, {Kind: JSONArrayElement, Path: "settings.json", Key: "enabled", Value: "b"}} {
		must(t, l.Put(i))
	}
	must(t, l.Save(r))
	loaded, e := Load(r)
	must(t, e)
	if len(loaded.Items()) != 4 {
		t.Fatal(loaded.Items())
	}
}
func TestRemovalKindsAndFailedSave(t *testing.T) {
	r := sandbox(t)
	l := New()
	data, e := SetSection([]byte("handwritten\n"), "instructions", []byte("body"))
	must(t, e)
	i := Item{Kind: MDSection, Path: "AGENTS.md", Marker: "instructions", Hash: HashBytes([]byte("body"))}
	write(t, r, i.Path, data)
	must(t, l.Put(i))
	must(t, l.RemoveOwned(r, i, []string{i.Path}))
	if string(read(t, r, i.Path)) != "handwritten\n" {
		t.Fatal("handwritten markdown changed")
	}
	external := filepath.Join(t.TempDir(), "target")
	must(t, os.WriteFile(external, []byte("keep"), 0600))
	i = Item{Kind: Symlink, Path: "link", Link: external}
	must(t, os.Symlink(external, filepath.Join(r, i.Path)))
	must(t, l.Put(i))
	must(t, l.RemoveOwned(r, i, []string{i.Path}))
	b, e := os.ReadFile(external)
	must(t, e)
	if string(b) != "keep" {
		t.Fatal("followed final link")
	}
	i = Item{Kind: File, Path: "generated", Hash: HashBytes([]byte("generated"))}
	write(t, r, i.Path, []byte("generated"))
	must(t, l.Put(i))
	must(t, l.Save(r))
	before := read(t, r, ".daypack/lock.toml")
	l.Version = 2
	if e := l.Save(r); e == nil {
		t.Fatal("saved invalid lock")
	}
	if !bytes.Equal(before, read(t, r, ".daypack/lock.toml")) {
		t.Fatal("failed save modified lock")
	}
	l.Version = 1
	must(t, l.RemoveOwned(r, i, []string{i.Path}))
	must(t, l.Save(r))
	loaded, e := Load(r)
	must(t, e)
	if len(loaded.Items()) != 0 {
		t.Fatal("removal not persisted")
	}
}

func TestLiteralJSONKeys(t *testing.T) {
	sandbox(t)
	for _, key := range []string{"server.name", "*", "?", "#", "@this", "!x", "x|y", "x:y", "x\\y", "0", "[x]", "a b", "a\"b"} {
		t.Run(key, func(t *testing.T) {
			literal := strings.ReplaceAll(strings.ReplaceAll(key, `\`, `\\`), ".", `\.`)
			i := Item{Kind: JSONEntry, Key: literal}
			data, e := SetJSON([]byte("{}\n"), i, []byte("1"))
			must(t, e)
			out, e := SetJSON(data, i, []byte("2"))
			must(t, e)
			v, _, e := jsonValue(out, i.Key)
			must(t, e)
			if v.Int() != 2 {
				t.Fatalf("key %q not replaced: %s", key, out)
			}
			removed, e := RemoveJSON(out, i)
			must(t, e)
			if !bytes.Equal(removed, []byte("{}\n")) {
				t.Fatalf("key %q not removed: %s", key, removed)
			}
		})
	}
}
