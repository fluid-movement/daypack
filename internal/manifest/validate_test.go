package manifest

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/library"
)

func testLibrary(t *testing.T) *library.Library {
	t.Helper()
	dir := isolated(t)
	write(t, filepath.Join(dir, "mcp", "shared.toml"), "type = 'stdio'\ncommand = 'echo'\n")
	write(t, filepath.Join(dir, "mcp", "broken.toml"), "type = 'stdio'\n")
	for _, name := range []string{"cat", "cot"} {
		write(t, filepath.Join(dir, "fragments", name+".md"), "---\ndescription: Test\n---\nContent\n")
	}
	lib, err := library.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return lib
}
func TestValidation(t *testing.T) {
	lib := testLibrary(t)
	cases := []struct{ name, text, want string }{
		{"unknown harness", "version=1\nharnesses=['unknown']", "unknown harness"},
		{"duplicate harness", "version=1\nharnesses=['pi','pi']", "duplicate harness"},
		{"empty selection", "version=1\nharnesses=[]", "must not be empty"},
		{"subset", "version=1\nharnesses=['pi']\nmcp=[{name='shared',harnesses=['claude-code']}]", "mcp:shared.harnesses"},
		{"unknown restriction", header + "mcp=[{name='shared',harnesses=['unknown']}]", "unknown harness"},
		{"empty restriction", header + "mcp=[{name='shared',harnesses=[]}]", "must not be empty"},
		{"duplicate restriction", header + "mcp=[{name='shared',harnesses=['pi','pi']}]", "duplicate harness"},
		{"name", header + "mcp=['Bad_Name']", "1-64"},
		{"duplicate", header + "mcp=['shared','shared']", "duplicate artifact"},
		{"missing tie", header + "fragments=['cut']", "fragment:cut not found (closest: cat)"},
		{"absent type", header + "skills=['absent']", "skill:absent not found"},
		{"invalid artifact", header + "mcp=['broken']", "mcp:broken has library validation errors"},
		{"collision unselected", header + "[inline.mcp.shared]\ntype='stdio'\ncommand='echo'", "collides"},
		{"inline subset", "version=1\nharnesses=['pi']\n[inline.mcp.local]\ntype='stdio'\ncommand='echo'\nharnesses=['claude-code']", "inline.mcp.local.harnesses"},
		{"inline empty", header + "[inline.mcp.local]\ntype='stdio'\ncommand='echo'\nharnesses=[]", "must not be empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := loadText(t, tc.text)
			problems := m.Validate(lib)
			if !strings.Contains(fmt.Sprint(problems), tc.want) {
				t.Fatalf("want %q in %v", tc.want, problems)
			}
			if !reflect.DeepEqual(problems, m.Validate(lib)) {
				t.Fatal("nondeterministic diagnostics")
			}
		})
	}
	m := loadText(t, header+"mcp=['shared']\nfragments=['cot','cat']")
	if p := m.Validate(lib); len(p) != 0 {
		t.Fatal(p)
	}
	if p := m.Validate(nil); !strings.Contains(fmt.Sprint(p), "library is required") {
		t.Fatal(p)
	}
	// Capabilities belong to adapters: an agent aimed at Pi is not a manifest
	// configuration error (the empty library still reports its missing name).
	m = loadText(t, "version=1\nharnesses=['pi']\nagents=['reviewer']")
	if p := m.Validate(lib); len(p) != 1 || !strings.Contains(p[0].Message, "not found") {
		t.Fatal(p)
	}
}

func TestInlineMCPParity(t *testing.T) {
	lib := testLibrary(t)
	cases := []string{
		"type='stdio'\ncommand='echo'\nenv={API_KEY='${API_KEY}'}",
		"type='stdio'\ncommand='echo'\nenv={API_KEY='never-echo-this'}",
		"type='http'\nurl='https://example.test'\nheaders={Authorization='never-echo-this'}",
		"type='stdio'\ncommand='echo'\nurl=''",
		"type='stdio'\ncommand='echo'\nheaders={}",
		"type='http'\nurl='https://example.test'\ncommand=''\nargs=[]\nenv={}",
		"type='stdio'\ncommand='echo'\nextra='never-echo-this'",
		"type='stdio'\ncommand=17",
		"type='stdio'\ncommand='echo'\nargs=['sk-example-secret']",
		"type='http'\nurl='https://example.test/?token=never-echo-this'",
		"type='stdio'\ncommand='echo'\nenv={VALUE='${broken'}",
	}
	for i, payload := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			m := loadText(t, header+"[inline.mcp.local]\n"+payload)
			_, want := library.ParseMCP([]byte(payload), m.path)
			for i := range want {
				want[i].Field = "inline.mcp.local." + want[i].Field
			}
			got := m.Validate(lib)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v; want %v", got, want)
			}
			if strings.Contains(fmt.Sprint(got), "never-echo-this") {
				t.Fatal("secret leaked in diagnostics")
			}
		})
	}
}

func TestInlineSharedValidation(t *testing.T) {
	for _, value := range []string{"${TOKEN}", "Bearer ${TOKEN:-nonsecret}", "${ONE:-}-${TWO}", "${TOKEN:-${NESTED}}", "${TOKEN:-private{bad}}", "private-literal"} {
		definition := library.MCPDefinition{Type: "http", URL: "https://example.test/${HOST:-local}", Headers: map[string]string{"Authorization": value}}
		m := &Manifest{Version: 1, Harnesses: []string{"claude-code"}, InlineMCP: []Entry{{Type: library.MCP, Name: "local", Inline: &definition}}}
		before := m.Entries()
		expected := library.ValidateMCP(definition, "")
		for i := range expected {
			expected[i].Field = "inline.mcp.local." + expected[i].Field
		}
		for _, env := range []string{"one", "two"} {
			t.Setenv("TOKEN", env)
			actual := m.Validate(nil)
			if !reflect.DeepEqual(actual, expected) || !reflect.DeepEqual(actual, m.Validate(nil)) {
				t.Fatal("inline and library validation differ")
			}
			if !reflect.DeepEqual(before, m.Entries()) {
				t.Fatal("validation mutated definition")
			}
		}
	}
	// Parse-only errors must survive additional validation without duplication.
	m := loadText(t, header+"[inline.mcp.local]\ntype='stdio'\ncommand=7\nextra='private-literal'\nenv={TOKEN='${TOKEN:-${NESTED}}'}")
	p := m.Validate(nil)
	seen := map[Problem]bool{}
	for _, problem := range p {
		if seen[problem] {
			t.Fatal("duplicate inline diagnostic")
		}
		seen[problem] = true
	}
	if !reflect.DeepEqual(p, m.Validate(nil)) {
		t.Fatal("repeated validation changed diagnostics")
	}
}

func TestValidationHints(t *testing.T) {
	lib := testLibrary(t)
	cases := []struct{ name, text, hint string }{
		{"unknown name", header + "fragments=['ca']", "did you mean fragment:cat?"},
		{"unknown harness", "version=1\nharnesses=['claud-code']", `did you mean "claude-code"?`},
		{"unknown entry harness", header + "mcp=[{name='shared',harnesses=['p']}]", `did you mean "pi"?`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found := false
			for _, p := range loadText(t, tc.text).Validate(lib) {
				found = found || p.Hint == tc.hint
			}
			if !found {
				t.Fatalf("missing hint %q", tc.hint)
			}
		})
	}
	m := loadText(t, "version=1\nharnesses=['claude-code']\nmcp=[{name='shared',harnesses=['pi']}]")
	if p := m.Validate(lib); len(p) != 1 || !strings.Contains(p[0].Message, "not selected") {
		t.Fatal(p)
	}
}
