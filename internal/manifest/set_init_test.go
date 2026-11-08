package manifest

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/library"
)

func TestSetEntryGolden(t *testing.T) {
	isolated(t)
	before, want := mustRead(t, "testdata/set-before.toml"), mustRead(t, "testdata/set-after.toml")
	got, changed, err := SetEntry(before, library.MCP, Entry{Name: "docs", Harnesses: []string{"claude-code"}})
	if err != nil || !changed {
		t.Fatal(err, changed)
	}
	got, changed, err = SetEntry(got, library.MCP, Entry{Name: "web"})
	if err != nil || !changed || !bytes.Equal(got, want) {
		t.Fatalf("%v %v\n%s", err, changed, got)
	}
	again, changed, err := SetEntry(got, library.MCP, Entry{Name: "web"})
	if err != nil || changed || !bytes.Equal(again, got) {
		t.Fatal("no-op expected", err, changed)
	}
}

func TestSetEntryCases(t *testing.T) {
	isolated(t)
	base := header + "skills = [\"a\"] # c\n"
	out, changed, err := SetEntry([]byte(base), library.Skill, Entry{Name: "b"})
	if err != nil || !changed || string(out) != header+"skills = [\"a\", \"b\"] # c\n" {
		t.Fatalf("%v %s", err, out)
	}
	out, _, err = SetEntry(out, library.Skill, Entry{Name: "a", Harnesses: []string{"pi"}})
	if err != nil || string(out) != header+"skills = [{ name = \"a\", harnesses = [\"pi\"] }, \"b\"] # c\n" {
		t.Fatalf("%v %s", err, out)
	}
	out, _, err = SetEntry(out, library.Skill, Entry{Name: "a", Harnesses: []string{"claude-code", "pi"}})
	if err != nil || !strings.Contains(string(out), `{ name = "a", harnesses = ["claude-code", "pi"] }`) || strings.Count(string(out), `"a"`) != 1 {
		t.Fatalf("%v %s", err, out)
	}
	if _, _, err = SetEntry(out, library.Skill, Entry{Name: "a", Harnesses: []string{"bogus"}}); err == nil {
		t.Fatal("expected harness error")
	}
	inline := header + "[inline.mcp.local]\ntype = \"stdio\"\ncommand = \"echo\"\n"
	if _, _, err = SetEntry([]byte(inline), library.MCP, Entry{Name: "local"}); err == nil {
		t.Fatal("expected inline collision")
	}
	multi := header + "skills = [\n  \"a\",\n]\n"
	if _, _, err = SetEntry([]byte(multi), library.Skill, Entry{Name: "a", Harnesses: []string{"pi"}}); err == nil || !strings.Contains(err.Error(), "by hand") {
		t.Fatal(err)
	}
	m, err := parse([]byte(base), "x")
	if err != nil {
		t.Fatal(err)
	}
	if !HasEntry(m, library.Skill, "a") || HasEntry(m, library.MCP, "a") {
		t.Fatal("HasEntry")
	}
}

func TestInitSkeleton(t *testing.T) {
	isolated(t)
	dir := t.TempDir()
	path, err := Init(dir, InitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, want := mustRead(t, path), mustRead(t, "testdata/init-skeleton.toml")
	if !bytes.Equal(got, want) {
		t.Fatalf("got:\n%s", got)
	}
	m, err := LoadManifest(path)
	if err != nil || strings.Join(m.Harnesses, ",") != "claude-code,pi" {
		t.Fatal(err, m)
	}
	data := got
	for _, kind := range []library.ArtifactType{library.Skill, library.MCP, library.Fragment, library.Agent} {
		data, err = AddEntry(data, kind, Entry{Name: "x"})
		if err != nil {
			t.Fatal(kind, err)
		}
	}
	if strings.Count(string(data), "# ") != strings.Count(string(want), "# ") {
		t.Fatalf("comments lost:\n%s", data)
	}
	if _, err = parse(data, "x"); err != nil {
		t.Fatal(err)
	}
}

func TestInitHarnessesAndExists(t *testing.T) {
	isolated(t)
	dir := t.TempDir()
	if _, err := Init(dir, InitOptions{Harnesses: []string{"bogus"}}); err == nil {
		t.Fatal("expected unknown harness error")
	}
	path, err := Init(dir, InitOptions{Harnesses: []string{"pi"}})
	if err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Harnesses) != 1 || m.Harnesses[0] != "pi" {
		t.Fatal(m.Harnesses)
	}
	before := mustRead(t, path)
	if _, err = Init(dir, InitOptions{}); !errors.Is(err, ErrExists) {
		t.Fatal(err)
	}
	if !bytes.Equal(before, mustRead(t, path)) {
		t.Fatal("changed")
	}
	for _, mk := range []func(string) error{
		func(p string) error { return os.Mkdir(p, 0o755) },
		func(p string) error { return os.Symlink("nowhere", p) },
	} {
		d := t.TempDir()
		if err := mk(filepath.Join(d, "daypack.toml")); err != nil {
			t.Fatal(err)
		}
		if _, err := Init(d, InitOptions{}); !errors.Is(err, ErrExists) {
			t.Fatal(err)
		}
	}
}

func TestInitFrom(t *testing.T) {
	isolated(t)
	src := t.TempDir()
	body := header + "# note\nskills = [\"a\", { name = \"b\", harnesses = [\"pi\"] }]\n\n[inline.mcp.x]\ntype = \"stdio\"\ncommand = \"echo\"\n"
	srcFile := filepath.Join(src, "daypack.toml")
	if err := os.WriteFile(srcFile, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, from := range []string{src, srcFile} {
		dst := t.TempDir()
		path, err := Init(dst, InitOptions{From: from})
		if err != nil {
			t.Fatal(err)
		}
		if got := mustRead(t, path); string(got) != body {
			t.Fatalf("not verbatim:\n%s", got)
		}
	}
	dst := t.TempDir()
	if _, err := Init(dst, InitOptions{From: src, Harnesses: []string{"claude-code"}}); err == nil || !strings.Contains(err.Error(), "skill:b") {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "daypack.toml")); !os.IsNotExist(err) {
		t.Fatal("file created on conflict")
	}
	bad := t.TempDir()
	if err := os.WriteFile(filepath.Join(bad, "daypack.toml"), []byte("not toml ="), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, from := range []string{bad, filepath.Join(bad, "missing")} {
		dst := t.TempDir()
		if _, err := Init(dst, InitOptions{From: from}); err == nil {
			t.Fatal("expected error")
		}
		if _, err := os.Lstat(filepath.Join(dst, "daypack.toml")); !os.IsNotExist(err) {
			t.Fatal("file created")
		}
	}
	dst = t.TempDir()
	if _, err := Init(dst, InitOptions{From: src, Harnesses: []string{"pi"}}); err != nil {
		t.Fatal(err)
	}
	got := mustRead(t, filepath.Join(dst, "daypack.toml"))
	if !strings.Contains(string(got), `harnesses = ["pi"]`) || !strings.Contains(string(got), "# note") {
		t.Fatalf("%s", got)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
