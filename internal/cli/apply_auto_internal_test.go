package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/lock"
)

func mcpRows(t *testing.T, root string) []lock.Item {
	t.Helper()
	owned, err := lock.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var rows []lock.Item
	for _, i := range owned.Items() {
		if i.Entry == "mcp:docs" {
			rows = append(rows, i)
		}
	}
	return rows
}

func TestApplyAutoSkipsInlineKeepsLibrary(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		tty  bool
	}{
		{"plain", nil, false},
		{"tty", nil, true},
		{"yes", []string{"--yes"}, false},
		{"tty yes", []string{"--yes"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project, libraryDir, registry := inlineApplyFixture(t)
			const sentinel = "DAYPACK_SYNTHETIC_SECRET_2651_DO_NOT_COPY"
			t.Setenv("DAYPACK_TEST_SECRET", sentinel)
			code, stdout, stderr := runInlineApply(project, libraryDir, registry, append([]string{"--auto"}, tc.args...), unreadableInput{t}, tc.tty)
			if code != Success {
				t.Fatalf("exit %d: %s %s", code, stdout, stderr)
			}
			if _, err := os.Readlink(filepath.Join(project, ".test/skills/testing")); err != nil {
				t.Fatal("trusted library skill was not applied: ", err)
			}
			for _, p := range []string{".mcp.json", ".test/settings.json"} {
				if _, err := os.Lstat(filepath.Join(project, p)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("inline entry wrote %s", p)
				}
			}
			if len(mcpRows(t, project)) != 0 {
				t.Fatal("inline entry gained lock rows")
			}
			if n := strings.Count(stdout+stderr, "run 'daypack apply'"); n != 1 {
				t.Fatalf("notice count %d: %s %s", n, stdout, stderr)
			}
			if strings.Contains(stdout+stderr, sentinel) || strings.Contains(stdout+stderr, "[y/N]") {
				t.Fatal("secret or prompt in output")
			}
		})
	}
}

func TestApplyAutoJSONNoticeOnce(t *testing.T) {
	project, libraryDir, registry := inlineApplyFixture(t)
	code, stdout, stderr := runInlineApply(project, libraryDir, registry, []string{"--auto", "--json"}, unreadableInput{t}, true)
	value := object(t, stdout, code, "daypack apply")
	if code != Success || stderr != "" {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	notes := value["notes"].([]any)
	count := 0
	for _, n := range notes {
		if n.(map[string]any)["Code"] == "auto-inline-skipped" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("notes %v", notes)
	}
}

func TestApplyAutoPreservesApprovedInline(t *testing.T) {
	for _, changed := range []bool{false, true} {
		name := "unchanged"
		if changed {
			name = "changed"
		}
		t.Run(name, func(t *testing.T) {
			project, libraryDir, registry := inlineApplyFixture(t)
			t.Setenv("DAYPACK_TEST_SECRET", "x")
			if code, out, errOut := runInlineApply(project, libraryDir, registry, []string{"--yes"}, unreadableInput{t}, false); code != Success {
				t.Fatalf("seed: %d %s %s", code, out, errOut)
			}
			// Verified unchanged history must not make auto trust the entry.
			if !changed {
				if code, out, errOut := runInlineApply(project, libraryDir, registry, []string{"--json"}, unreadableInput{t}, false); code != Success {
					t.Fatalf("regate: %d %s %s", code, out, errOut)
				}
			} else {
				manifest := filepath.Join(project, "daypack.toml")
				data := readFile(t, manifest)
				if err := os.WriteFile(manifest, []byte(strings.Replace(string(data), `command = "server"`, `command = "other"`, 1)), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			mcp := readFile(t, filepath.Join(project, ".mcp.json"))
			approval := readFile(t, filepath.Join(project, ".test/settings.json"))
			rows := mcpRows(t, project)
			if len(rows) == 0 || len(mcp) == 0 {
				t.Fatal("seed did not create inline state")
			}
			code, stdout, stderr := runInlineApply(project, libraryDir, registry, []string{"--auto", "--yes"}, unreadableInput{t}, true)
			if code != Success {
				t.Fatalf("exit %d: %s %s", code, stdout, stderr)
			}
			mcpAfter := readFile(t, filepath.Join(project, ".mcp.json"))
			approvalAfter := readFile(t, filepath.Join(project, ".test/settings.json"))
			if string(mcp) != string(mcpAfter) || string(approval) != string(approvalAfter) {
				t.Fatal("auto changed generated inline config or approval")
			}
			if !reflect.DeepEqual(rows, mcpRows(t, project)) {
				t.Fatal("auto changed inline lock rows")
			}
			if n := strings.Count(stdout+stderr, "run 'daypack apply'"); n != 1 {
				t.Fatalf("notice count %d: %s", n, stdout)
			}
		})
	}
}

func TestApplyAutoDryRunInline(t *testing.T) {
	project, libraryDir, registry := inlineApplyFixture(t)
	code, stdout, stderr := runInlineApply(project, libraryDir, registry, []string{"--auto", "--dry-run"}, unreadableInput{t}, true)
	if code != Success || stderr != "" || !strings.Contains(stdout, "run 'daypack apply'") {
		t.Fatalf("exit %d: %s %s", code, stdout, stderr)
	}
	for _, p := range []string{".daypack", ".test", ".mcp.json"} {
		if _, err := os.Lstat(filepath.Join(project, p)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("dry-run wrote %s", p)
		}
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
