package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/cli"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func importFixture(t *testing.T) testutil.Fixture {
	t.Helper()
	f := testutil.NewFixture(t, false)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	root, err := filepath.EvalSymlinks(f.Library)
	if err != nil {
		t.Fatal(err)
	}
	f.Library = root
	importWrite(t, filepath.Join(root, ".gitignore"), "/secrets.env\n")
	return f
}
func importWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestLibraryImportCLI(t *testing.T) {
	f := importFixture(t)
	sentinel := testutil.PlantSentinel(t, f)
	importWrite(t, filepath.Join(f.Home, ".claude.json"), `{"mcpServers":{"cli":{"command":"echo","env":{"TOKEN":"`+sentinel+`"}}}}`)
	before := testutil.SnapshotTree(t, f.Library)
	for _, mode := range [][]string{{}, {"--json"}, {"--quiet"}} {
		args := append([]string{"library", "import", "--dry-run"}, mode...)
		r := testutil.RunCLIWithInput(t, f, strings.NewReader(""), true, args...)
		if r.Code != 0 || r.Stderr != "" {
			t.Fatalf("preview: %+v", r)
		}
		testutil.AssertNoSecret(t, f.Project, sentinel, r.Stdout, r.Stderr)
		testutil.AssertTree(t, f.Library, before)
		if strings.Contains(r.Stdout, "removed manually") {
			t.Fatal("preview advertises cleanup")
		}
		if len(mode) > 0 && mode[0] == "--quiet" && r.Stdout != "" {
			t.Fatal("quiet preview prose")
		}
	}
	r := testutil.RunCLIWithInput(t, f, strings.NewReader(""), true, "library", "import")
	if r.Code != 0 || !strings.Contains(r.Stdout, "original item may be removed manually") || !strings.Contains(r.Stdout, "/mcpServers/cli") {
		t.Fatalf("cleanup report missing: %+v", r)
	}
	testutil.AssertNoSecret(t, f.Project, sentinel, r.Stdout, r.Stderr)
	r = testutil.RunCLI(t, f, "library", "import", "--json")
	var envelope struct {
		OK      bool   `json:"ok"`
		Command string `json:"command"`
		Items   []struct {
			Name    string `json:"name"`
			Outcome string `json:"outcome"`
			Safe    bool   `json:"safe_to_remove"`
		} `json:"items"`
	}
	dec := json.NewDecoder(strings.NewReader(r.Stdout))
	if err := dec.Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatal("multiple JSON documents")
	}
	if !envelope.OK || envelope.Command != "daypack library import" || len(envelope.Items) != 1 || envelope.Items[0].Outcome != "unchanged" || !envelope.Items[0].Safe {
		t.Fatal("invalid JSON envelope")
	}
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{"library", "import", "extra"}, 2}, {[]string{"library", "import", "--unknown"}, 2}, {[]string{"--library", filepath.Join(f.Home, "missing"), "library", "import"}, 1},
	} {
		r = testutil.RunCLI(t, f, tc.args...)
		if r.Code != tc.code {
			t.Fatalf("status %d want %d", r.Code, tc.code)
		}
	}
	importWrite(t, filepath.Join(f.Home, ".claude.json"), `{"mcpServers":{"cli":{"type":"sse","url":"https://example.invalid?token=`+sentinel+`"}}}`)
	for _, mode := range [][]string{{}, {"--json"}, {"--quiet"}} {
		r = testutil.RunCLI(t, f, append([]string{"library", "import"}, mode...)...)
		if r.Code != 3 || r.Stdout == "" {
			t.Fatalf("attention diagnostic: %+v", r)
		}
		testutil.AssertNoSecret(t, f.Project, sentinel, r.Stdout, r.Stderr)
	}
	importWrite(t, filepath.Join(f.Home, ".claude.json"), `{"mcpServers": "`+sentinel+`"}`)
	r = testutil.RunCLI(t, f, "library", "import", "--json")
	if r.Code != 3 {
		t.Fatal("malformed status")
	}
	testutil.AssertNoSecret(t, f.Project, sentinel, r.Stdout, r.Stderr)
}

func TestLibraryImportSelectedStorageAndOverrides(t *testing.T) {
	for _, selection := range []string{"explicit", "DAYPACK_HOME", "XDG_CONFIG_HOME", "HOME"} {
		t.Run(selection, func(t *testing.T) {
			f := importFixture(t)
			if selection == "HOME" || selection == "XDG_CONFIG_HOME" {
				home, err := filepath.EvalSymlinks(f.Home)
				if err != nil {
					t.Fatal(err)
				}
				f.Home = home
				t.Setenv("HOME", home)
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
				target := filepath.Join(f.Home, ".config", "daypack")
				if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
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
			switch selection {
			case "explicit":
				t.Setenv("DAYPACK_HOME", filepath.Join(f.Home, "unused"))
			case "DAYPACK_HOME":
				t.Setenv("DAYPACK_HOME", f.Library)
			case "XDG_CONFIG_HOME":
				t.Setenv("DAYPACK_HOME", "")
			case "HOME":
				t.Setenv("DAYPACK_HOME", "")
				t.Setenv("XDG_CONFIG_HOME", "")
			}
			// Even an explicit override equal to the default directory relocates state.
			claude := filepath.Join(f.Home, ".claude")
			pi := filepath.Join(f.Home, "custom-pi")
			t.Setenv("CLAUDE_CONFIG_DIR", claude)
			t.Setenv("PI_CODING_AGENT_DIR", pi)
			importWrite(t, filepath.Join(claude, ".claude.json"), `{"mcpServers":{"selected":{"command":"echo","env":{"TOKEN":"synthetic"}}}}`)
			importWrite(t, filepath.Join(f.Home, ".claude.json"), `{"mcpServers":{"ignored":{"command":"no"}}}`)
			importWrite(t, filepath.Join(pi, "mcp.json"), `{"mcpServers":{"remote":{"url":"https://example.invalid"}}}`)
			args := []string{"--project", f.Project, "library", "import", "--json"}
			if selection == "explicit" {
				args = append([]string{"--library", f.Library}, args...)
			}
			var out, stderr bytes.Buffer
			code := cli.Run(context.Background(), args, cli.IO{Out: &out, Err: &stderr})
			if code != 0 {
				t.Fatalf("selected import failed (%d): %s %s", code, out.String(), stderr.String())
			}
			if _, err := os.Stat(filepath.Join(f.Library, "secrets.env")); err != nil {
				t.Fatal("selected store missing")
			}
			if _, err := os.Stat(filepath.Join(f.Library, "mcp", "selected.toml")); err != nil {
				t.Fatal("override state not scanned")
			}
			if _, err := os.Stat(filepath.Join(f.Library, "mcp", "remote.toml")); err != nil {
				t.Fatal("Pi override not scanned")
			}
			if strings.Contains(out.String(), "mcp:ignored") {
				t.Fatal("default state scanned during override")
			}
		})
	}
}

func TestLibraryImportGoldens(t *testing.T) {
	f := importFixture(t)
	importWrite(t, filepath.Join(f.Home, ".claude.json"), `{"mcpServers":{"preview":{"command":"echo"},"unsafe":{"command":"echo","args":["sk-REDACTED"]}}}`)
	for _, jsonMode := range []bool{false, true} {
		args := []string{"library", "import", "--dry-run"}
		if jsonMode {
			args = append(args, "--json")
		}
		r := testutil.RunCLI(t, f, args...)
		if r.Code != 3 {
			t.Fatal("golden status")
		}
		got := testutil.NormalizeFixturePaths([]byte(r.Stdout), f.Home, f.Library, f.Project)
		if jsonMode {
			testutil.AssertJSONGolden(t, "testdata/import.json", got)
		} else {
			testutil.AssertGolden(t, "testdata/import.txt", got)
		}
	}
}

func TestLibraryImportNoPrompts(t *testing.T) {
	f := importFixture(t)
	importWrite(t, filepath.Join(f.Home, ".claude.json"), `{"mcpServers":{"no-exec":{"command":"./nonexistent-executable"}}}`)
	r := testutil.RunCLIWithInput(t, f, strings.NewReader(""), true, "library", "import")
	if r.Code != 0 || r.Stderr != "" {
		t.Fatalf("import prompted or executed MCP: %+v", r)
	}
}
