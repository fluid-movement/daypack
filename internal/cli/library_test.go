package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/library"
	"github.com/spf13/cobra"
)

func libraryEnvironment(t *testing.T) string {
	t.Helper()
	base := environment(t)
	base, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "native library ; $(punctuation)")
	if err = library.Init(root); err != nil {
		t.Fatal(err)
	}
	return root
}
func libraryInvoke(t *testing.T, root string, args ...string) (int, string, string) {
	t.Helper()
	args = append([]string{"--library", root, "--project", filepath.Dir(root), "library"}, args...)
	var out, stderr bytes.Buffer
	code := Run(context.Background(), args, IO{In: strings.NewReader(""), Out: &out, Err: &stderr})
	return code, out.String(), stderr.String()
}
func TestLibraryCommands(t *testing.T) {
	root := libraryEnvironment(t)
	t.Setenv("DAYPACK_HOME", filepath.Join(filepath.Dir(root), "wrong"))
	for _, kind := range []string{"skill", "mcp", "fragment", "agent"} {
		code, out, stderr := libraryInvoke(t, root, "add", kind, "example", "--json")
		if code != Success || stderr != "" {
			t.Fatalf("%d %s", code, stderr)
		}
		value := object(t, out, code, "daypack library add")
		if value["items"] == nil {
			t.Fatal("missing typed item")
		}
		code, _, _ = libraryInvoke(t, root, "add", kind, "example")
		if code != Error {
			t.Fatal("accepted duplicate")
		}
		code, out, _ = libraryInvoke(t, root, "rename", kind, "example", "renamed", "--json")
		if code != Success {
			t.Fatalf("rename failed: %s", out)
		}
		value = object(t, out, code, "daypack library rename")
		if value["problems"] == nil || !strings.Contains(out, "will fail until updated") {
			t.Fatal("missing JSON warning")
		}

		code, out, _ = libraryInvoke(t, root, "rename", kind, "renamed", "again", "--quiet")
		if code != Success || !strings.Contains(out, "will fail until updated") {
			t.Fatalf("missing human rename warning: %s", out)
		}
		code, out, _ = libraryInvoke(t, root, "remove", kind, "again", "--json")
		if code != Success || !strings.Contains(out, "will fail until updated") {
			t.Fatalf("missing JSON removal warning: %s", out)
		}
		object(t, out, code, "daypack library remove")
		code, _, stderr = libraryInvoke(t, root, "add", kind, "renamed")
		if code != Success {
			t.Fatal(stderr)
		}
		code, out, _ = libraryInvoke(t, root, "remove", kind, "renamed", "--quiet")
		if code != Success || !strings.Contains(out, "will fail until updated") {
			t.Fatalf("missing quiet warning: %s", out)
		}
	}
	code, out, stderr := libraryInvoke(t, root, "doctor", "--json")
	if code != Success || stderr != "" {
		t.Fatalf("%d %s %s", code, out, stderr)
	}
	object(t, out, code, "daypack library doctor")
}
func TestLibraryUsageDoesNotMutate(t *testing.T) {
	root := libraryEnvironment(t)
	for _, args := range [][]string{{"init", "extra"}, {"add", "skill"}, {"add", "invalid", "x"}, {"add", "agent", "../outside"}, {"remove", "agent"}, {"rename", "skill", "old", "Bad"}} {
		code, _, _ := libraryInvoke(t, root, args...)
		if code != Usage {
			t.Fatalf("%v: got %d", args, code)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "skills"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("usage mutated: %v %v", entries, err)
	}
}
func TestLibraryCLIInitAndImport(t *testing.T) {
	root := libraryEnvironment(t)
	newRoot := filepath.Join(filepath.Dir(root), "fresh")
	code, _, stderr := libraryInvoke(t, newRoot, "init")
	if code != Success {
		t.Fatal(stderr)
	}
	code, _, _ = libraryInvoke(t, newRoot, "init")
	if code != Error {
		t.Fatal("init overwrote library")
	}
	a, err := library.Add(root, library.AddOptions{Type: library.Skill, Name: "imported"})
	if err != nil {
		t.Fatal(err)
	}
	code, out, stderr := libraryInvoke(t, newRoot, "add", "skill", "imported", "--from", a.Path, "--json")
	if code != Success {
		t.Fatal(stderr)
	}
	object(t, out, code, "daypack library add")
	if requestedJSON([]string{"library", "add", "fragment", "a", "--from", "--json"}) {
		t.Fatal("source argument interpreted as JSON flag")
	}
}
func TestLibraryDoctorOutcomes(t *testing.T) {
	root := libraryEnvironment(t)
	if err := os.WriteFile(filepath.Join(root, "unknown.txt"), []byte("unknown"), 0644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := libraryInvoke(t, root, "doctor", "--json")
	if code != Success || !strings.Contains(out, "unknown library entry") {
		t.Fatalf("%d %s", code, out)
	}
	if err := os.WriteFile(filepath.Join(root, "agents", "broken.md"), []byte("invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	code, out, _ = libraryInvoke(t, root, "doctor", "--json")
	value := object(t, out, code, "daypack library doctor")
	problems, ok := value["problems"].([]any)
	if code != Error || !ok || len(problems) < 2 {
		t.Fatalf("%d %s", code, out)
	}
	code, out, _ = libraryInvoke(t, root, "doctor", "--quiet")
	if code != Error || !strings.Contains(out, "broken.md") {
		t.Fatalf("%d %s", code, out)
	}
}
func TestLibraryEditor(t *testing.T) {
	root := libraryEnvironment(t)
	a, err := library.Add(root, library.AddOptions{Type: library.Agent, Name: "editable"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, editor                 string
		terminal, json, fail, called bool
		code                         int
	}{
		{name: "unset", terminal: true, code: Error},
		{name: "success", editor: "/editor path/with spaces", terminal: true, called: true, code: Success},
		{name: "failure", editor: "editor", terminal: true, fail: true, called: true, code: Error},
		{name: "json", editor: "editor", terminal: true, json: true, code: Error},
		{name: "pipe", editor: "editor", code: Error},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("EDITOR", tt.editor)
			called := false
			args := []string{"--library", root, "--project", filepath.Dir(root), "library", "edit", "agent", "editable"}
			if tt.json {
				args = append(args, "--json")
			}
			code, out, stderr := invoke(t, args, tt.terminal, func(rootCmd *cobra.Command) {
				old, _, err := rootCmd.Find([]string{"library"})
				if err != nil {
					t.Fatal(err)
				}
				rootCmd.RemoveCommand(old)
				rootCmd.AddCommand(libraryCommand(IO{In: strings.NewReader("input"), Out: io.Discard, Err: io.Discard}, func(_ context.Context, editor, path string, streams IO) error {
					called = true
					if editor != tt.editor || path != a.Path || streams.In == nil || streams.Out == nil || streams.Err == nil {
						t.Fatal("incorrect editor invocation")
					}
					if tt.fail {
						return errors.New("editor failed")
					}
					return nil
				}))
			})
			if code != tt.code || called != tt.called {
				t.Fatalf("code %d called %v: %s", code, called, stderr)
			}
			if tt.json {
				object(t, out, code, "daypack library edit")
				if !strings.Contains(out, a.Path) {
					t.Fatal("native path missing")
				}
			}
			if tt.name == "unset" && !strings.Contains(stderr, "EDITOR is unset") {
				t.Fatal("unclear editor error")
			}
		})
	}
}

func TestProductionEditorPassesLiteralPath(t *testing.T) {
	base := environment(t)
	executable := filepath.Join(base, "editor with spaces")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s' \"$1\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "literal ; $(false) file.md")
	var out bytes.Buffer
	if err := runEditor(context.Background(), executable, path, IO{In: strings.NewReader(""), Out: &out, Err: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if out.String() != path {
		t.Fatalf("path was not passed literally: %q", out.String())
	}
}
