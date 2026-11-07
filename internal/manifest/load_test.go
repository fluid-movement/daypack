package manifest

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/library"
)

const header = "version = 1\nharnesses = [\"claude-code\", \"pi\"]\n"

func isolated(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, key := range []string{"HOME", "DAYPACK_HOME", "XDG_CONFIG_HOME"} {
		t.Setenv(key, dir)
	}
	return dir
}
func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func loadText(t *testing.T, text string) *Manifest {
	t.Helper()
	path := filepath.Join(isolated(t), "daypack.toml")
	write(t, path, text)
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestLoadSchema(t *testing.T) {
	cases := []struct{ name, text, want string }{
		{"missing version", "harnesses = []", "version"},
		{"version type", "version = 'one'\nharnesses = []", "version"},
		{"unsupported", "version = 2\nharnesses = []", "unsupported"},
		{"missing harnesses", "version = 1", "harnesses"},
		{"harness type", "version = 1\nharnesses = 'pi'", "harnesses"},
		{"harness element", "version = 1\nharnesses = [1]", "harnesses"},
		{"unknown root", header + "templates = []", "templates"},
		{"array type", header + "skills = 'test'", "skills"},
		{"entry type", header + "skills = [1]", "skills[0]"},
		{"missing name", header + "skills = [{}]", "name"},
		{"name type", header + "skills = [{name = 1}]", "name"},
		{"entry unknown", header + "skills = [{name = 'test', bogus = true}]", "skills[0].bogus"},
		{"target type", header + "skills = [{name = 'test', harnesses = 'pi'}]", "harnesses"},
		{"inline type", header + "inline = []", "inline"},
		{"inline non mcp", header + "[inline.skills.test]\nname = 'test'", "inline.skills"},
		{"mcp type", header + "[inline]\nmcp = []", "inline.mcp"},
		{"server type", header + "[inline.mcp]\nserver = 'x'", "inline.mcp.server"},
		{"inline harness type", header + "[inline.mcp.test]\nharnesses = [false]", "inline.mcp.test.harnesses"},
		{"syntax secret", header + "[inline.mcp.test]\ncommand = \"never-echo-this", "syntax"},
	}
	dir := isolated(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, "daypack.toml")
			write(t, path, tc.text)
			_, err := LoadManifest(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
			if strings.Contains(err.Error(), "never-echo-this") {
				t.Fatal("syntax error leaked input")
			}
		})
	}
	if _, err := LoadManifest(filepath.Join(dir, "missing")); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing I/O error: %v", err)
	}
}

func TestEntriesOrderAndCopies(t *testing.T) {
	m := loadText(t, header+`skills = ["test", {name = "browser", harnesses = ["claude-code"]}]
mcp = ["shared"]
fragments = ["second", "first"]
agents = ["reviewer"]
[inline.mcp.zed]
type = "stdio"
command = "echo"
args = ["hello"]
env = {NORMAL = "value"}
[inline.mcp.alpha]
type = "http"
url = "https://example.test"
headers = {NORMAL = "value"}
`)
	before := m.Entries()
	names := []string{}
	for _, e := range before {
		names = append(names, e.Name)
	}
	if !reflect.DeepEqual(names, []string{"test", "browser", "shared", "second", "first", "reviewer", "alpha", "zed"}) {
		t.Fatal(names)
	}
	if !reflect.DeepEqual(before[0].Harnesses, m.Harnesses) || !reflect.DeepEqual(before[1].Harnesses, []string{"claude-code"}) {
		t.Fatal(before)
	}
	changed := m.Entries()
	changed[0].Harnesses[0] = "bad"
	changed[1].Harnesses[0] = "bad"
	changed[6].Inline.Headers["NORMAL"] = "bad"
	changed[7].Inline.Env["NORMAL"] = "bad"
	changed[7].Inline.Args[0] = "bad"
	if !reflect.DeepEqual(before, m.Entries()) {
		t.Fatal("Entries aliases manifest storage")
	}
	if before[0].Type != library.Skill || before[3].Type != library.Fragment || before[5].Type != library.Agent {
		t.Fatal("wrong artifact types")
	}
}
