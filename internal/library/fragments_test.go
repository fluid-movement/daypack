package library_test

import (
	"testing"

	"github.com/fluid-movement/daypack/internal/library"
)

func TestFragmentBody(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"no frontmatter", "Plain text.\n", "Plain text.\n"},
		{"empty", "", ""},
		{"frontmatter", "---\ndescription: Test\n---\nBody.\n", "Body.\n"},
		{"empty body after frontmatter", "---\ndescription: Test\n---\n", ""},
		{"frontmatter without final newline", "---\ndescription: Test\n---", ""},
		{"CRLF body kept", "---\r\ndescription: Test\r\n---\r\nline one\r\nline two\r\n", "line one\r\nline two\r\n"},
		{"CRLF without frontmatter", "a\r\nb\r\n", "a\r\nb\r\n"},
		{"no final newline", "---\ndescription: x\n---\nend", "end"},
		{"later rule is body", "intro\n---\nnot frontmatter\n---\n", "intro\n---\nnot frontmatter\n---\n"},
		{"leading blank body lines survive", "---\ndescription: x\n---\n\n\ntext\n", "\n\ntext\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := []byte(c.in)
			got, err := library.FragmentBody(in)
			if err != nil || string(got) != c.want {
				t.Fatalf("got %q, %v; want %q", got, err, c.want)
			}
			if string(in) != c.in {
				t.Fatal("input mutated")
			}
			if len(got) > 0 {
				got[0] ^= 0xff
				if string(in) != c.in {
					t.Fatal("result aliases input")
				}
			}
		})
	}
}

func TestFragmentBodyRejectsMalformedFrontmatter(t *testing.T) {
	for name, in := range map[string]string{
		"unterminated":       "---\ndescription: x\nbody\n",
		"only opener":        "---",
		"opener and newline": "---\n",
		"invalid yaml":       "---\n: : :\n  - [\n---\nbody\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := library.FragmentBody([]byte(in)); err == nil || got != nil {
				t.Fatalf("accepted %q: %q", in, got)
			}
		})
	}
}
