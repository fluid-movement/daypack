package library

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func sandbox(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("DAYPACK_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	return root
}
func write(t *testing.T, root, path, body string) string {
	t.Helper()
	p := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func load(t *testing.T, root string) *Library {
	t.Helper()
	l, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return l
}
func hasProblem(p []Problem, field string, severity Severity) bool {
	for _, v := range p {
		if v.Field == field && v.Severity == severity {
			return true
		}
	}
	return false
}
func TestPaths(t *testing.T) {
	root := sandbox(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, daypack, xdg, want string }{
		{"home", "", "", filepath.Join(root, ".config/daypack")},
		{"xdg", "", filepath.Join(root, "xdg"), filepath.Join(root, "xdg/daypack")},
		{"override", filepath.Join(root, "lib"), filepath.Join(root, "xdg"), filepath.Join(root, "lib")},
		{"relative", "relative/../library", "", filepath.Join(cwd, "library")},
		{"tilde", "~/library", "", filepath.Join(cwd, "~/library")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DAYPACK_HOME", tc.daypack)
			t.Setenv("XDG_CONFIG_HOME", tc.xdg)
			got, err := DefaultPath()
			if err != nil || got != tc.want {
				t.Fatalf("path got %q, err %v", got, err)
			}
			if filepath.IsAbs(tc.daypack) || tc.daypack == "" {
				if err := os.MkdirAll(tc.want, 0700); err != nil {
					t.Fatal(err)
				}
				if l := load(t, ""); l.root != tc.want {
					t.Fatal("Load did not use default")
				}
			}
		})
	}
	t.Setenv("DAYPACK_HOME", "/does-not-exist")
	t.Setenv("HOME", "")
	if load(t, root).root != root {
		t.Fatal("explicit path lost")
	}
	if _, err := DefaultPath(); err != nil {
		t.Fatal("override attempted home lookup")
	}
	for _, p := range []string{filepath.Join(root, "missing"), write(t, root, "file", "")} {
		_, err := Load(p)
		if err == nil || !strings.Contains(err.Error(), p) || !strings.Contains(err.Error(), "daypack library init") {
			t.Fatal("missing init guidance")
		}
	}
	if got := load(t, root).List(Skill); got == nil || len(got) != 0 {
		t.Fatal("missing type directory must be empty")
	}
}
func TestFixtureLibrary(t *testing.T) {
	root := sandbox(t)
	before := map[string][]byte{}
	err := filepath.WalkDir("testdata/valid", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel("testdata/valid", path)
		if err != nil {
			return err
		}
		p := write(t, root, rel, string(data))
		before[p] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	l := load(t, root)
	if p := l.Validate(); len(p) != 0 {
		t.Fatalf("valid fixture problems: %v", p)
	}
	for _, tc := range []struct{ ref, path, description string }{
		{"skill:svelte", "skills/svelte", "Svelte guidance"}, {"mcp:docs", "mcp/docs.toml", "Documentation server"}, {"agent:reviewer", "agents/reviewer.md", "Review changes"}, {"fragment:style", "fragments/style.md", ""},
	} {
		a, err := l.Resolve(tc.ref)
		if err != nil {
			t.Fatal(err)
		}
		if a.Path != filepath.Join(root, tc.path) || a.Description != tc.description {
			t.Fatal("incorrect artifact metadata")
		}
		if (a.MCP != nil) != (a.Type == MCP) {
			t.Fatal("MCP pointer type")
		}
	}
	for p, want := range before {
		got, err := os.ReadFile(p)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("native file modified")
		}
	}
	a, _ := l.Resolve("mcp:docs")
	a.MCP.Env["API_KEY"] = "changed"
	a, _ = l.Resolve("mcp:docs")
	if a.MCP.Env["API_KEY"] != "${API_KEY}" {
		t.Fatal("caller mutated stored definition")
	}
}
func TestResolve(t *testing.T) {
	root := sandbox(t)
	for _, n := range []string{"cat", "bat"} {
		write(t, root, "fragments/"+n+".md", "")
	}
	l := load(t, root)
	for _, ref := range []string{"", "skill:", "skills:cat", "fragment:Bad", "fragment:cat:extra", "fragment:a_b"} {
		_, err := l.Resolve(ref)
		var nf *NotFoundError
		if err == nil || errors.As(err, &nf) {
			t.Fatal("malformed reference accepted or not-found")
		}
	}
	for _, tc := range []struct{ ref, closest string }{{"fragment:hat", "bat"}, {"skill:hat", ""}, {"fragment:catt", "cat"}} {
		_, err := l.Resolve(tc.ref)
		var nf *NotFoundError
		if !errors.As(err, &nf) || nf.Closest != tc.closest {
			t.Fatal("incorrect typed suggestion")
		}
	}
}
func TestNativeValidation(t *testing.T) {
	for _, tc := range []struct{ name, kind, body, field string }{
		{"missing metadata", "skills", "plain", "frontmatter"},
		{"malformed", "skills", "---\nname: [\n---\n", "frontmatter"},
		{"unclosed", "skills", "---\nname: docs\n", "frontmatter"},
		{"mismatch", "skills", "---\nname: other\ndescription: fine\n---", "frontmatter.name"},
		{"blank", "skills", "---\nname: docs\ndescription: '  '\n---", "frontmatter.description"},
		{"agent mismatch", "agents", "---\nname: other\ndescription: fine\n---", "frontmatter.name"},
		{"agent blank", "agents", "---\nname: docs\n---", "frontmatter.description"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := sandbox(t)
			path := tc.kind + "/docs.md"
			ref := "agent:docs"
			if tc.kind == "skills" {
				path = "skills/docs/SKILL.md"
				ref = "skill:docs"
			}
			source := write(t, root, path, tc.body)
			l := load(t, root)
			if !hasProblem(l.Validate(), tc.field, Error) {
				t.Fatal("missing metadata error")
			}
			for _, p := range l.Validate() {
				if p.File != source {
					t.Fatal("wrong problem source")
				}
			}
			_, err := l.Resolve(ref)
			var validation *ValidationError
			if !errors.As(err, &validation) {
				t.Fatal("invalid artifact resolved")
			}
		})
	}
	root := sandbox(t)
	if err := os.MkdirAll(filepath.Join(root, "skills/docs"), 0700); err != nil {
		t.Fatal(err)
	}
	l := load(t, root)
	if !hasProblem(l.Validate(), "frontmatter", Error) {
		t.Fatal("missing SKILL.md not reported")
	}
	write(t, root, "fragments/plain.md", "Plain markdown")
	write(t, root, "fragments/meta.md", "---\ndescription: useful\nother: true\n---\nBody")
	a, err := load(t, root).Resolve("fragment:meta")
	if err != nil || a.Description != "useful" {
		t.Fatal("optional fragment metadata")
	}
}
func TestNamesAndDuplicates(t *testing.T) {
	root := sandbox(t)
	for _, name := range []string{"Bad", "a_b", strings.Repeat("a", 65)} {
		write(t, root, "fragments/"+name+".md", "")
	}
	for _, name := range []string{"-", "docs", strings.Repeat("a", 64)} {
		write(t, root, "fragments/"+name+".md", "")
	}
	write(t, root, "mcp/docs.toml", "type='stdio'\ncommand='run'\n")
	l := load(t, root)
	count := 0
	for _, p := range l.Validate() {
		if p.Field == "name" {
			count++
			if p.File == "" {
				t.Fatal("missing source")
			}
		}
	}
	if count != 3 {
		t.Fatalf("got %d name errors", count)
	}
	for _, ref := range []string{"fragment:-", "fragment:docs", "mcp:docs"} {
		if _, err := l.Resolve(ref); err != nil {
			t.Fatal(err)
		}
	}
	l = &Library{artifacts: []Artifact{{Type: Fragment, Name: "docs", Path: "/one"}, {Type: Fragment, Name: "docs", Path: "/two"}}}
	l.buildIndex()
	if len(l.Validate()) != 2 {
		t.Fatal("duplicates overwritten")
	}
	if _, err := l.Resolve("fragment:docs"); err == nil {
		t.Fatal("duplicate resolved")
	}
}
func TestUnknownAndDeterminism(t *testing.T) {
	root := sandbox(t)
	for _, p := range []string{"README", "mcp/unknown.json", "agents/sub/file", "skills/unknown.txt"} {
		write(t, root, p, "")
	}
	for _, p := range []string{".git/config", ".gitignore", "secrets.env", "skills/docs/support/nested"} {
		write(t, root, p, "opaque")
	}
	write(t, root, "skills/docs/SKILL.md", "---\nname: docs\ndescription: fine\n---")
	a := load(t, root)
	b := load(t, root)
	if !reflect.DeepEqual(a.Validate(), b.Validate()) || !reflect.DeepEqual(a.List(Skill), b.List(Skill)) {
		t.Fatal("nondeterministic results")
	}
	if len(a.Validate()) != 4 {
		t.Fatal("unexpected unknown entries")
	}
	for _, p := range a.Validate() {
		if p.Severity != Warning {
			t.Fatal("unknown entry was error")
		}
	}
}
