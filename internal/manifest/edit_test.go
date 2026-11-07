package manifest

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/library"
)

func TestEditGolden(t *testing.T) {
	isolated(t)
	before, err := os.ReadFile("testdata/edit-before.toml")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/edit-after.toml")
	if err != nil {
		t.Fatal(err)
	}
	original := bytes.Clone(before)
	got, err := AddEntry(before, library.Fragment, Entry{Name: "last", Harnesses: []string{"claude-code"}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got:\n%s", got)
	}
	restored, err := RemoveEntry(got, library.Fragment, "last")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, before) || !bytes.Equal(before, original) {
		t.Fatal("round trip changed bytes")
	}
}
func TestEditArrays(t *testing.T) {
	isolated(t)
	cases := []struct{ array, name, want string }{
		{`["one", "two", "three"]`, "one", `["two", "three"]`},
		{`["one", "two", "three"]`, "two", `["one", "three"]`},
		{`["one", "two", "three"]`, "three", `["one", "two"]`},
		{`[ 'one', ]`, "one", `[  ]`},
		{`["one", {name='two', harnesses=['pi','claude-code']},]`, "two", `["one",]`},
	}
	for _, tc := range cases {
		got, err := RemoveEntry([]byte(header+"skills = "+tc.array+" # tail\n"), library.Skill, tc.name)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != header+"skills = "+tc.want+" # tail\n" {
			t.Fatalf("got %s", got)
		}
	}
	for _, array := range []string{`[]`, `[ ]`, `["one"]`, `[ 'one', ]`, `[{name='one', harnesses=['pi','claude-code']}]`} {
		input := []byte(header + "skills = " + array + " # tail\n")
		added, err := AddEntry(input, library.Skill, Entry{Name: "new"})
		if err != nil {
			t.Fatal(err)
		}
		removed, err := RemoveEntry(added, library.Skill, "new")
		if err != nil || !bytes.Equal(input, removed) {
			t.Fatalf("round trip %s -> %s: %v", input, removed, err)
		}
	}
	for _, input := range []string{header, header + "[inline.mcp.local]\ntype='stdio'\ncommand='echo'\n", strings.ReplaceAll(header, "\n", "\r\n")} {
		got, err := AddEntry([]byte(input), library.Agent, Entry{Name: "reviewer"})
		if err != nil {
			t.Fatal(err)
		}
		m, err := parse(got, "test")
		if err != nil || len(m.Agents) != 1 {
			t.Fatalf("%s: %v", got, err)
		}
	}
}
func TestEditRefusal(t *testing.T) {
	isolated(t)
	for _, array := range []string{"[\n 'one', # comment\n]", "[\n'one'\n]"} {
		input := []byte(header + "skills = " + array + "\n")
		original := bytes.Clone(input)
		for _, add := range []bool{true, false} {
			var got []byte
			var err error
			if add {
				got, err = AddEntry(input, library.Skill, Entry{Name: "new"})
			} else {
				got, err = RemoveEntry(input, library.Skill, "one")
			}
			if err == nil || !strings.Contains(err.Error(), "edit daypack.toml by hand") || !strings.Contains(err.Error(), "skills =") {
				t.Fatalf("got %s %v", got, err)
			}
			if !bytes.Equal(input, original) {
				t.Fatal("mutated input")
			}
		}
	}
	for _, input := range []string{header + "'skills' = []", header + "'skills' = ['one']"} {
		if _, err := AddEntry([]byte(input), library.Skill, Entry{Name: "new"}); err == nil || !strings.Contains(err.Error(), "by hand") {
			t.Fatal(err)
		}
	}
	data := []byte(header + "skills=['one']")
	for _, entry := range []Entry{{Name: "one"}, {Name: "Bad"}, {Name: "new", Harnesses: []string{}}, {Name: "new", Harnesses: []string{"unknown"}}, {Name: "new", Inline: &library.MCPDefinition{}}, {Name: "new", Type: library.Agent}} {
		if _, err := AddEntry(data, library.Skill, entry); err == nil {
			t.Fatalf("accepted %+v", entry)
		}
	}
	if _, err := AddEntry(data, library.ArtifactType("unknown"), Entry{Name: "new"}); err == nil {
		t.Fatal("accepted unknown type")
	}
	if got, err := RemoveEntry(data, library.Skill, "absent"); err != nil || !bytes.Equal(got, data) {
		t.Fatal(err)
	}
	if _, err := AddEntry([]byte("bad TOML"), library.Skill, Entry{Name: "new"}); err == nil {
		t.Fatal("accepted invalid input")
	}
}
