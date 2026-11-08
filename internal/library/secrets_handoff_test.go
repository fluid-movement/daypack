package library_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/fluid-movement/daypack/internal/secrets"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func TestSecretsImporterHandoff(t *testing.T) {
	for _, worktree := range []bool{false, true} {
		for _, selection := range []string{"explicit", "DAYPACK_HOME", "XDG_CONFIG_HOME", "HOME"} {
			t.Run(fmt.Sprintf("%t/%s", worktree, selection), func(t *testing.T) {
				f := testutil.NewFixture(t, worktree)
				canonical, err := filepath.EvalSymlinks(f.Library)
				if err != nil {
					t.Fatal(err)
				}
				f.Library = canonical
				if err := os.WriteFile(filepath.Join(f.Library, ".gitignore"), []byte("/secrets.env\n"), 0600); err != nil {
					t.Fatal(err)
				}
				sentinel := testutil.PlantSentinel(t, f)
				project := f.Project
				if worktree {
					project = f.Worktree
				}
				selected := f.Library
				switch selection {
				case "explicit":
					t.Setenv("DAYPACK_HOME", filepath.Join(f.Home, "unused"))
				case "DAYPACK_HOME":
					t.Setenv("DAYPACK_HOME", f.Library)
				case "XDG_CONFIG_HOME":
					t.Setenv("DAYPACK_HOME", "")
					t.Setenv("XDG_CONFIG_HOME", filepath.Dir(f.Library))
					target := filepath.Join(filepath.Dir(f.Library), "daypack")
					if err := os.Rename(f.Library, target); err != nil {
						t.Fatal(err)
					}
					f.Library = target
				case "HOME":
					t.Setenv("DAYPACK_HOME", "")
					t.Setenv("XDG_CONFIG_HOME", "")
					target := filepath.Join(f.Home, ".config", "daypack")
					if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(f.Library, target); err != nil {
						t.Fatal(err)
					}
					f.Library, err = filepath.EvalSymlinks(target)
					if err != nil {
						t.Fatal(err)
					}
				}
				if selection != "explicit" {
					selected, err = library.DefaultPath()
					if err != nil {
						t.Fatal(err)
					}
					selected, err = filepath.EvalSymlinks(selected)
					if err != nil {
						t.Fatal(err)
					}
				}
				// Synthetic importer: publish sanitized definitions only after a complete
				// per-server storage batch succeeds. No harness scanning or allocation.
				publish := func(values map[string]string, name string) error {
					if err := library.WriteSecrets(selected, project, values); err != nil {
						return err
					}
					ref, err := secrets.EnvReference("SYNTHETIC_TOKEN")
					if err != nil {
						return err
					}
					body := "type='stdio'\ncommand='echo'\nenv={TOKEN='" + ref + "'}\n"
					if _, p := library.ParseMCP([]byte(body), "synthetic"); len(p) != 0 {
						return fmt.Errorf("synthetic definition invalid")
					}
					path := filepath.Join(project, name+".toml")
					return os.WriteFile(path, []byte("version=1\nharnesses=['claude-code']\n[inline.mcp.synthetic]\n"+body), 0600)
				}
				if err := publish(map[string]string{"SYNTHETIC_TOKEN": sentinel, "SYNTHETIC_EMPTY": ""}, "sanitized"); err != nil {
					t.Fatal(err)
				}
				stored, err := os.ReadFile(filepath.Join(selected, "secrets.env"))
				if err != nil || !bytes.Contains(stored, []byte("SYNTHETIC_TOKEN='"+sentinel+"'")) || !bytes.Contains(stored, []byte("SYNTHETIC_EMPTY=''")) {
					t.Fatal("complete batch was not stored")
				}
				info, err := os.Stat(filepath.Join(selected, "secrets.env"))
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatal("handoff file is not private")
				}
				m, err := manifest.LoadManifest(filepath.Join(project, "sanitized.toml"))
				if err != nil || len(m.Validate(nil)) != 0 {
					t.Fatal("inline handoff failed validation")
				}
				err = publish(map[string]string{"SYNTHETIC_TOKEN": "different", "SYNTHETIC_NEW": sentinel}, "conflict")
				if err == nil {
					t.Fatal("conflict accepted")
				}
				if _, err := os.Stat(filepath.Join(project, "conflict.toml")); !os.IsNotExist(err) {
					t.Fatal("definition published after conflict")
				}
				testutil.AssertNoSecret(t, project, sentinel, errString(err))
				after, readErr := os.ReadFile(filepath.Join(selected, "secrets.env"))
				if readErr != nil || !bytes.Equal(stored, after) {
					t.Fatal("conflicting batch changed storage")
				}
				// Validate against changing environment and malformed private storage:
				// neither is an input to validation, and validation never modifies them.
				file := filepath.Join(selected, "secrets.env")
				if err := os.WriteFile(file, []byte("malformed-private-file"), 0600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("SYNTHETIC_TOKEN", "changed")
				if len(m.Validate(nil)) != 0 {
					t.Fatal("validation depended on private file or environment")
				}
				testutil.AssertNoSecret(t, project, sentinel)
			})
		}
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
