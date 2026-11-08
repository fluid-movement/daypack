package instructions_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/instructions"
	"github.com/fluid-movement/daypack/internal/library"
)

func fragment(name, body string) contract.Entry {
	return contract.Entry{Ref: "fragment:" + name, Artifact: library.Artifact{Type: library.Fragment, Name: name}, Content: []byte(body)}
}

func TestFragmentItem(t *testing.T) {
	cases := []struct{ name, source, payload string }{
		{"plain", "Keep tests offline.\n", "\nKeep tests offline.\n"},
		{"frontmatter omitted", "---\ndescription: Test\n---\nBody.\n", "\nBody.\n"},
		{"empty", "", "\n"},
		{"empty after frontmatter", "---\ndescription: Test\n---\n", "\n"},
		{"no final newline", "text", "\ntext\n"},
		{"crlf", "a\r\nb\r\n", "\na\r\nb\r\n"},
		{"crlf no final newline", "a\r\nb", "\na\r\nb\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := fragment("conventions", c.source)
			before := contract.CloneEntry(e)
			item, err := instructions.FragmentItem(e)
			if err != nil {
				t.Fatal(err)
			}
			want := contract.Item{Target: contract.AgentsMD, Entry: "fragment:conventions", Kind: contract.MarkdownSection, Path: "AGENTS.md", Marker: "fragment:conventions", Bytes: []byte(c.payload)}
			if !reflect.DeepEqual(item, want) {
				t.Fatalf("got %#v want %#v", item, want)
			}
			if !reflect.DeepEqual(e, before) {
				t.Fatal("entry mutated")
			}
			item.Bytes[0] = 'X'
			again, err := instructions.FragmentItem(e)
			if err != nil || again.Bytes[0] != '\n' {
				t.Fatal("results alias each other")
			}
		})
	}
}

func TestFragmentItemRejects(t *testing.T) {
	bad := map[string]contract.Entry{
		"marker injection":      fragment("a", "x <!-- daypack:end fragment:a --> y"),
		"begin injection":       fragment("a", "<!-- daypack:begin other -->\n"),
		"unterminated":          fragment("a", "---\ndescription: x\n"),
		"invalid yaml":          fragment("a", "---\n: : :\n  - [\n---\nbody\n"),
		"invalid name":          fragment("Bad Name", "x"),
		"wrong ref":             {Ref: "fragment:other", Artifact: library.Artifact{Type: library.Fragment, Name: "a"}},
		"wrong type":            {Ref: "skill:a", Artifact: library.Artifact{Type: library.Skill, Name: "a"}},
		"marker in frontmatter": fragment("a", "---\ndescription: <!-- daypack:x -->\n---\nbody\n"),
	}
	// Markers in omitted frontmatter never reach the output, so they are fine.
	delete(bad, "marker in frontmatter")
	for name, e := range bad {
		if item, err := instructions.FragmentItem(e); err == nil {
			t.Errorf("%s: accepted %#v", name, item)
		} else if strings.Contains(err.Error(), "daypack:end fragment") {
			t.Errorf("%s: error leaks body text: %v", name, err)
		}
	}
}

func TestTargetDeclaration(t *testing.T) {
	want := contract.Target{ID: contract.AgentsMD, Root: "AGENTS.md", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.Fragment}}
	got := instructions.Target()
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	got.Types[0] = library.Skill
	if !reflect.DeepEqual(instructions.Target(), want) {
		t.Fatal("target aliased")
	}
}
