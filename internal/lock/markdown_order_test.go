package lock_test

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func sec(name, body string) lock.Section {
	return lock.Section{Marker: name, Body: []byte("\n" + body + "\n")}
}

func block(name, body string) string {
	return "<!-- daypack:begin " + name + " -->\n" + body + "\n<!-- daypack:end " + name + " -->"
}

// outsideLines returns the lines of a document that are not inside any daypack
// section (markers included), so handwritten text can be compared.
func outsideLines(data string) []string {
	var out []string
	inside := false
	for _, line := range strings.Split(data, "\n") {
		begin := strings.Contains(line, "<!-- daypack:begin ")
		end := strings.Contains(line, "<!-- daypack:end ")
		switch {
		case begin && end:
		case begin:
			inside = true
		case end:
			inside = false
		case !inside && line != "":
			out = append(out, line)
		}
	}
	return out
}

func TestReconcileSectionsGoldens(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		desired []lock.Section
		managed []string
	}{
		{"create-empty", "", []lock.Section{sec("a", "A"), sec("b", "B")}, []string{"a", "b"}},
		{"create-after-handwritten-no-final-newline", "# Title\nprose", []lock.Section{sec("a", "A")}, []string{"a"}},
		{"append-after-last", "pre\n" + block("a", "A") + "\nmid\n" + block("b", "B") + "\npost\n", []lock.Section{sec("a", "A"), sec("b", "B"), sec("c", "C")}, []string{"a", "b", "c"}},
		{"insert-front", "pre\n" + block("a", "A") + "\nmid\n" + block("b", "B") + "\npost\n", []lock.Section{sec("x", "X"), sec("a", "A"), sec("b", "B")}, []string{"a", "b", "x"}},
		{"insert-middle", "pre\n" + block("a", "A") + "\nmid\n" + block("b", "B") + "\npost\n", []lock.Section{sec("a", "A"), sec("x", "X"), sec("b", "B")}, []string{"a", "b", "x"}},
		{"update-body", "pre\n" + block("a", "A") + "\npost\n", []lock.Section{sec("a", "changed")}, []string{"a"}},
		{"reorder", "pre\n" + block("a", "A") + "\nmid prose\n" + block("b", "B") + "\npost\n", []lock.Section{sec("b", "B"), sec("a", "A")}, []string{"a", "b"}},
		{"remove-middle", "pre\n" + block("a", "A") + "\nprose-1\n" + block("b", "B") + "\nprose-2\n" + block("c", "C") + "\npost\n", []lock.Section{sec("a", "A"), sec("c", "C")}, []string{"a", "b", "c"}},
		{"remove-all", "pre\n" + block("a", "A") + "\nmid\n" + block("b", "B") + "\npost\n", nil, []string{"a", "b"}},
		{"unrelated-marker-untouched", block("a", "A") + "\n" + block("other", "O") + "\n" + block("b", "B") + "\n", []lock.Section{sec("b", "B"), sec("a", "A")}, []string{"a", "b"}},
		{"crlf-handwritten", "line one\r\nline two\r\n" + block("a", "A") + "\r\ntail\r\n", []lock.Section{sec("a", "A2"), sec("b", "B")}, []string{"a", "b"}},
		{"crlf-remove", "line one\r\n" + block("a", "A") + "\r\ntail\r\n", nil, []string{"a"}},
		{"no-final-newline-add", block("a", "A"), []lock.Section{sec("a", "A"), sec("b", "B")}, []string{"a", "b"}},
		{"prose-on-marker-line", "x " + block("a", "A") + " y\n", nil, []string{"a"}},
		{"unmanaged-only", "keep\n" + block("other", "O") + "\n", []lock.Section{sec("a", "A")}, []string{"a"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := c.in
			got, err := lock.ReconcileSections([]byte(c.in), c.desired, c.managed)
			if err != nil {
				t.Fatal(err)
			}
			if c.in != before {
				t.Fatal("input mutated")
			}
			testutil.AssertGolden(t, filepath.Join("testdata", "order", c.name+".golden"), got)
			if !reflect.DeepEqual(outsideLines(c.in), outsideLines(string(got))) && !strings.HasPrefix(c.name, "prose-on") {
				t.Fatalf("handwritten lines changed: %q -> %q", outsideLines(c.in), outsideLines(string(got)))
			}
			again, err := lock.ReconcileSections(got, c.desired, c.managed)
			if err != nil || !bytes.Equal(again, got) {
				t.Fatalf("not idempotent: %v\n%q\n%q", err, got, again)
			}
		})
	}
}

func TestReconcileSectionsRejects(t *testing.T) {
	a := block("a", "A")
	cases := []struct {
		name    string
		in      string
		desired []lock.Section
		managed []string
	}{
		{"duplicate", a + "\n" + a + "\n", nil, []string{"a"}},
		{"nested", "<!-- daypack:begin a -->\n<!-- daypack:begin b -->\n<!-- daypack:end b -->\n<!-- daypack:end a -->\n", nil, []string{"a"}},
		{"unclosed", "<!-- daypack:begin a -->\nA\n", nil, []string{"a"}},
		{"unmatched-end", "<!-- daypack:end a -->\n", nil, []string{"a"}},
		{"mismatched", "<!-- daypack:begin a -->\n<!-- daypack:end b -->\n", nil, []string{"a"}},
		{"injected-body", a + "\n", []lock.Section{{Marker: "a", Body: []byte("\n<!-- daypack:end a -->\n")}}, []string{"a"}},
		{"unmanaged-desired", a + "\n", []lock.Section{sec("b", "B")}, []string{"a"}},
		{"duplicate-desired", a + "\n", []lock.Section{sec("a", "1"), sec("a", "2")}, []string{"a"}},
		{"duplicate-managed", a + "\n", nil, []string{"a", "a"}},
		{"bad-name", "", []lock.Section{sec("a b", "1")}, []string{"a b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := c.in
			got, err := lock.ReconcileSections([]byte(c.in), c.desired, c.managed)
			if err == nil || got != nil || c.in != before {
				t.Fatalf("accepted: %q %v", got, err)
			}
		})
	}
}

func TestMovedSections(t *testing.T) {
	in := []byte(block("a", "A") + "\n" + block("b", "B") + "\n" + block("c", "C") + "\n")
	moved, err := lock.MovedSections(in, []string{"a", "b", "c"}, []string{"a", "b", "c"})
	if err != nil || len(moved) != 0 {
		t.Fatalf("%v %v", moved, err)
	}
	// Creation and removal alone are not a move.
	moved, err = lock.MovedSections(in, []string{"a", "c", "x"}, []string{"a", "b", "c", "x"})
	if err != nil || len(moved) != 0 {
		t.Fatalf("%v %v", moved, err)
	}
	moved, err = lock.MovedSections(in, []string{"c", "b", "a"}, []string{"a", "b", "c"})
	if err != nil || !reflect.DeepEqual(moved, []string{"c", "a"}) {
		t.Fatalf("%v %v", moved, err)
	}
	if _, err = lock.MovedSections([]byte("<!-- daypack:begin a -->"), nil, nil); err == nil {
		t.Fatal("accepted malformed document")
	}
}
