package importer_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/importer"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/plan"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func fixture(t *testing.T, worktree bool) (testutil.Fixture, importer.Options) {
	t.Helper()
	f := testutil.NewFixture(t, worktree)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	var err error
	f.Library, err = filepath.EvalSymlinks(f.Library)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(f.Library, ".gitignore"), "/secrets.env\n")
	project := f.Project
	if worktree {
		project = f.Worktree
	}
	return f, importer.Options{HomeDir: f.Home, LibraryDir: f.Library, ProjectDir: project}
}
func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func skill(t *testing.T, path, name string) {
	t.Helper()
	write(t, filepath.Join(path, "SKILL.md"), "---\nname: "+name+"\ndescription: Synthetic native skill\ncustom: preserve me\n---\nNative body.\n")
	write(t, filepath.Join(path, "bin", "tool"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(path, "bin", "tool"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(path, "empty"), 0755); err != nil {
		t.Fatal(err)
	}
}
func link(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}
func run(t *testing.T, opts importer.Options) importer.Result {
	t.Helper()
	r, err := importer.Import(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func item(t *testing.T, r importer.Result, name string) importer.Item {
	t.Helper()
	for _, i := range r.Items {
		if i.Name == name {
			return i
		}
	}
	t.Fatalf("missing item %s", name)
	return importer.Item{}
}
func TestImportCompleteNativeAndMCP(t *testing.T) {
	f, opts := fixture(t, true)
	native := filepath.Join(f.Home, ".claude", "skills", "native")
	skill(t, native, "native")
	link(t, "native", filepath.Join(f.Home, ".claude", "skills", "alias"))
	link(t, native, filepath.Join(f.Home, ".agents", "skills", "nested", "shared"))
	// SKILL.md aliases are materialized while preserving native bytes.
	if err := os.Rename(filepath.Join(native, "SKILL.md"), filepath.Join(native, "metadata.md")); err != nil {
		t.Fatal(err)
	}
	link(t, "metadata.md", filepath.Join(native, "SKILL.md"))
	write(t, filepath.Join(f.Home, ".claude", "agents", "nested", "different.md"), "---\nname: native-agent\ndescription: Synthetic agent\nunknown: keep\n---\nReview.\n")
	state := `{"mcpServers":{"local":{"command":"echo","args":["hello"]}},"projects":{"/not/real~project":{"mcpServers":{"local":{"args":["hello"],"command":"echo"},"remote":{"type":"http","url":"https://example.invalid/mcp"}}},"/another":{"mcpServers":{"third":{"command":"other"}}}}}`
	write(t, filepath.Join(f.Home, ".claude.json"), state)
	write(t, filepath.Join(f.Home, ".pi", "agent", "mcp.json"), `{"mcpServers":{"local":{"command":"echo","args":["hello"]},"pi-remote":{"url":"https://pi.invalid/mcp","headers":{"X-Test":"$UNSET"}}}}`)
	// Project files and settings/auth are outside the scanner allowlist.
	write(t, filepath.Join(f.Project, ".mcp.json"), `{"mcpServers":{"project":{"command":"no"}}}`)
	write(t, filepath.Join(f.Home, ".claude", "settings.json"), "not JSON")
	write(t, filepath.Join(f.Home, ".pi", "agent", "auth.json"), "do not open")
	beforeHome := testutil.SnapshotTree(t, f.Home)
	beforeProject := testutil.SnapshotTree(t, f.Project)
	beforeWorktree := testutil.SnapshotTree(t, f.Worktree)
	beforeLibrary := testutil.SnapshotTree(t, f.Library)
	opts.DryRun = true
	r := run(t, opts)
	if r.Attention() || len(r.Items) != 6 {
		t.Fatalf("unexpected preview: %+v", r.Items)
	}
	for _, i := range r.Items {
		if i.SafeToRemove {
			t.Fatal("preview cleanup eligibility")
		}
	}
	if len(item(t, r, "native").Sources) != 3 || len(item(t, r, "local").Sources) != 3 {
		t.Fatal("source aliases lost")
	}
	foundPointer := false
	for _, s := range item(t, r, "local").Sources {
		if s.Key == "/projects/~1not~1real~0project/mcpServers/local" {
			foundPointer = true
		}
	}
	if !foundPointer {
		t.Fatal("escaped source pointer missing")
	}
	testutil.AssertTree(t, f.Home, beforeHome)
	testutil.AssertTree(t, f.Library, beforeLibrary)
	opts.DryRun = false
	r = run(t, opts)
	if r.Attention() {
		t.Fatalf("import attention: %+v", r.Items)
	}
	for _, i := range r.Items {
		if !i.SafeToRemove || i.Outcome != plan.Applied {
			t.Fatalf("incomplete import: %+v", i)
		}
	}
	testutil.AssertTree(t, f.Home, beforeHome)
	testutil.AssertTree(t, f.Project, beforeProject)
	testutil.AssertTree(t, f.Worktree, beforeWorktree)
	sourceTree := testutil.SnapshotTree(t, native)
	sourceTree["SKILL.md"] = sourceTree["metadata.md"]
	testutil.AssertTree(t, filepath.Join(f.Library, "skills", "native"), sourceTree)
	st, err := os.Stat(filepath.Join(f.Library, "skills", "native", "bin", "tool"))
	if err != nil || st.Mode().Perm()&0111 != 0111 {
		t.Fatal("executable bits lost")
	}
	after := testutil.SnapshotTree(t, f.Library)
	r = run(t, opts)
	for _, i := range r.Items {
		if i.Outcome != plan.Unchanged || !i.SafeToRemove {
			t.Fatalf("repeat: %+v", i)
		}
	}
	testutil.AssertTree(t, f.Library, after)
	for _, path := range []string{filepath.Join(f.Home, ".claude", "skills"), filepath.Join(f.Home, ".claude", "agents"), filepath.Join(f.Home, ".agents", "skills"), filepath.Join(f.Home, ".claude.json"), filepath.Join(f.Home, ".pi", "agent", "mcp.json")} {
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
	}
	opts.DryRun = true
	if r = run(t, opts); len(r.Items) != 0 {
		t.Fatal("removed originals still discovered")
	}
}

func TestSecretsSentinelAndRepeat(t *testing.T) {
	f, opts := fixture(t, false)
	sentinel := testutil.PlantSentinel(t, f)
	config := map[string]any{"mcpServers": map[string]any{
		"tokens": map[string]any{"command": "echo", "env": map[string]string{"API_TOKEN": sentinel, "KEEP_TOKEN": "${UNRESOLVED}"}},
		"remote": map[string]any{"type": "http", "url": "https://example.invalid", "headers": map[string]string{"Authorization": "Bearer " + sentinel}},
	}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(f.Home, ".claude.json"), string(data))
	before := testutil.SnapshotTree(t, f.Library)
	opts.DryRun = true
	r := run(t, opts)
	report, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	testutil.AssertNoSecret(t, f.Project, sentinel, string(report))
	testutil.AssertTree(t, f.Library, before)
	opts.DryRun = false
	r = run(t, opts)
	if r.Attention() {
		t.Fatalf("secret import failed: %+v", r.Items)
	}
	report, err = json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	testutil.AssertNoSecret(t, f.Project, sentinel, string(report))
	testutil.AssertNoSecret(t, filepath.Join(f.Library, "mcp"), sentinel)
	stored, err := os.ReadFile(filepath.Join(f.Library, "secrets.env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stored), "TOKENS_API_TOKEN='"+sentinel+"'") || !strings.Contains(string(stored), "REMOTE_AUTHORIZATION='Bearer "+sentinel+"'") {
		t.Fatal("complete original values not stored")
	}
	st, err := os.Stat(filepath.Join(f.Library, "secrets.env"))
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("store is not private")
	}
	lib, err := library.Load(f.Library)
	if err != nil {
		t.Fatal(err)
	}
	a, err := lib.Resolve("mcp:tokens")
	if err != nil {
		t.Fatal(err)
	}
	if a.MCP.Env["API_TOKEN"] != "${TOKENS_API_TOKEN}" || a.MCP.Env["KEEP_TOKEN"] != "${UNRESOLVED}" {
		t.Fatal("references not preserved")
	}
	after := testutil.SnapshotTree(t, f.Library)
	r = run(t, opts)
	if r.Attention() {
		t.Fatal("repeat requires attention")
	}
	testutil.AssertTree(t, f.Library, after)
}

func TestIndependentInvalidAndExcluded(t *testing.T) {
	f, opts := fixture(t, false)
	skill(t, filepath.Join(f.Home, ".claude", "skills", "valid"), "valid")
	for _, root := range []string{filepath.Join(f.Home, ".claude", "plugins", "installed"), filepath.Join(f.Home, ".claude", "skills", "synced", "installed"), filepath.Join(f.Home, ".pi", "agent", "packages", "installed")} {
		skill(t, root, "excluded")
		link(t, root, filepath.Join(f.Home, ".agents", "skills", fmt.Sprintf("alias-%d", len(root))))
	}
	link(t, "missing", filepath.Join(f.Home, ".claude", "skills", "broken"))
	link(t, "loop", filepath.Join(f.Home, ".claude", "skills", "loop"))
	skill(t, filepath.Join(f.Home, ".claude", "skills", "escape"), "escape")
	link(t, "/nonexistent", filepath.Join(f.Home, ".claude", "skills", "escape", "bad"))
	write(t, filepath.Join(f.Home, ".claude", "agents", "bad.md"), "---\nname: INVALID\ndescription: no\n---\n")
	write(t, filepath.Join(f.Home, ".claude.json"), `{"mcpServers":{"bad-type":{"command":123},"sse":{"type":"sse","url":"https://example.invalid"},"helper":{"command":"!read-secret"},"cwd":{"command":"echo","cwd":"/tmp"},"bad-map":{"command":"echo","env":{"TOKEN":42}}}}`)
	write(t, filepath.Join(f.Home, ".pi", "agent", "mcp.json"), `{"mcpServers":{"lossy":{"url":"https://example.invalid","oauth":{"clientSecret":"ignored"}},"pi-helper":{"command":"echo","env":{"TOKEN":"!secret"}}}}`)
	before := testutil.SnapshotTree(t, f.Home)
	r := run(t, opts)
	if !r.Attention() || item(t, r, "valid").Outcome != plan.Applied {
		t.Fatal("invalid independent item suppressed valid import")
	}
	lossy := item(t, r, "lossy")
	if lossy.SafeToRemove || lossy.Outcome != plan.Applied {
		t.Fatalf("lossy import: %+v", lossy)
	}
	for _, i := range r.Items {
		if i.Name != "valid" && i.SafeToRemove {
			t.Fatal("unsafe cleanup", i.Name)
		}
	}
	testutil.AssertTree(t, f.Home, before)
}

func TestSameNameConflictsAndFullNativeEquality(t *testing.T) {
	for _, difference := range []string{"support", "executable", "mcp", "invalid-library"} {
		t.Run(difference, func(t *testing.T) {
			f, opts := fixture(t, false)
			name := "collide"
			if difference == "mcp" {
				write(t, filepath.Join(f.Home, ".claude.json"), `{"mcpServers":{"collide":{"command":"a"}}}`)
				write(t, filepath.Join(f.Home, ".pi", "agent", "mcp.json"), `{"mcpServers":{"collide":{"command":"b"}}}`)
			} else {
				a := filepath.Join(f.Home, ".claude", "skills", name)
				skill(t, a, name)
				b := filepath.Join(f.Home, ".agents", "skills", name)
				if difference == "invalid-library" {
					b = filepath.Join(f.Library, "skills", name)
				}
				skill(t, b, name)
				switch difference {
				case "support":
					write(t, filepath.Join(b, "extra"), "different")
				case "executable":
					if err := os.Chmod(filepath.Join(b, "bin", "tool"), 0644); err != nil {
						t.Fatal(err)
					}
				case "invalid-library":
					write(t, filepath.Join(b, "SKILL.md"), "invalid")
				}
			}
			before := testutil.SnapshotTree(t, f.Library)
			opts.DryRun = true
			r := run(t, opts)
			if !r.Attention() {
				t.Fatal("preview missed conflict")
			}
			testutil.AssertTree(t, f.Library, before)
			opts.DryRun = false
			r = run(t, opts)
			for _, i := range r.Items {
				if i.SafeToRemove || i.Outcome != plan.Skipped {
					t.Fatal("conflict was imported")
				}
			}
			testutil.AssertTree(t, f.Library, before)
		})
	}
}

func TestMCPContentDedupeAcrossNames(t *testing.T) {
	f, opts := fixture(t, false)
	write(t, filepath.Join(f.Home, ".claude.json"), `{"mcpServers":{"aaa":{"command":"echo","env":{"TOKEN":"literal"}},"zzz":{"env":{"TOKEN":"literal"},"command":"echo","args":[]}}}`)
	r := run(t, opts)
	if len(r.Items) != 1 || r.Items[0].Name != "aaa" || len(r.Items[0].Sources) != 2 {
		t.Fatalf("dedupe: %+v", r.Items)
	}
	if _, err := os.Stat(filepath.Join(f.Library, "mcp", "zzz.toml")); !os.IsNotExist(err) {
		t.Fatal("duplicate published")
	}
	r = run(t, opts)
	if len(r.Items) != 1 || r.Items[0].Outcome != plan.Unchanged {
		t.Fatalf("repeat dedupe: %+v", r.Items)
	}
}

func TestSecretRejectionsAreAtomicAndRedacted(t *testing.T) {
	cases := map[string]string{
		"args":        `{"command":"echo","args":["sk-SENTINEL-value"]}`,
		"url":         `{"type":"http","url":"https://example.invalid?token=SENTINEL"}`,
		"mixed":       `{"command":"echo","env":{"TOKEN":"SENTINEL${REF}"}}`,
		"default":     `{"command":"echo","env":{"TOKEN":"${REF:-SENTINEL}"}}`,
		"intra-batch": `{"command":"echo","env":{"API-TOKEN":"SENTINEL","API_TOKEN":"different"}}`,
		"reserved":    `{"command":"echo","env":{"TOKEN":"SENTINEL"}}`,
		"malformed":   `{"command":"echo","env":{"TOKEN":["SENTINEL"]}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			f, opts := fixture(t, false)
			server := "rejected"
			if name == "reserved" {
				server = "npm"
			}
			write(t, filepath.Join(f.Home, ".claude.json"), `{"mcpServers":{"`+server+`":`+body+`}}`)
			before := testutil.SnapshotTree(t, f.Library)
			for _, dry := range []bool{true, false} {
				opts.DryRun = dry
				r := run(t, opts)
				if !r.Attention() {
					t.Fatal("unsafe server accepted")
				}
				out, err := json.Marshal(r)
				if err != nil {
					t.Fatal(err)
				}
				testutil.AssertNoSecret(t, f.Project, "SENTINEL", string(out))
				testutil.AssertTree(t, f.Library, before)
			}
		})
	}
}

func TestSecretStorageFailures(t *testing.T) {
	for _, failure := range []string{"different", "syntax", "permissions", "symlink", "git-hygiene", "project-store", "cross-batch"} {
		t.Run(failure, func(t *testing.T) {
			f, opts := fixture(t, false)
			write(t, filepath.Join(f.Home, ".claude.json"), `{"mcpServers":{"collision":{"command":"echo","env":{"TOKEN":"SENTINEL","NEW_TOKEN":"new"}}}}`)
			store := filepath.Join(f.Library, "secrets.env")
			switch failure {
			case "different":
				write(t, store, "COLLISION_TOKEN=other\n")
			case "syntax":
				write(t, store, "export BAD=unsupported\n")
			case "permissions":
				write(t, store, "OTHER=value\n")
				if err := os.Chmod(store, 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				write(t, filepath.Join(f.Home, "store"), "OTHER=value\n")
				link(t, filepath.Join(f.Home, "store"), store)
			case "git-hygiene":
				write(t, filepath.Join(f.Library, ".gitignore"), "")
			case "project-store":
				opts.ProjectDir = filepath.Dir(f.Library)
			case "cross-batch":
				write(t, filepath.Join(f.Home, ".claude.json"), `{"mcpServers":{"a-b":{"command":"echo","env":{"TOKEN":"SENTINEL"}},"a":{"command":"echo","env":{"B_TOKEN":"different"}}}}`)
			}
			before := testutil.SnapshotTree(t, f.Library)
			for _, dry := range []bool{true, false} {
				opts.DryRun = dry
				r := run(t, opts)
				if !r.Attention() {
					t.Fatal("storage failure accepted")
				}
				for _, i := range r.Items {
					if i.SafeToRemove || i.Outcome != plan.Skipped {
						t.Fatal("batch published")
					}
				}
				testutil.AssertTree(t, f.Library, before)
			}
		})
	}
}

func TestOverridesAndMalformedJSON(t *testing.T) {
	f, opts := fixture(t, false)
	opts.ClaudeConfigDir = filepath.Join(f.Home, "custom-claude")
	opts.PiAgentDir = filepath.Join(f.Home, "custom-pi")
	write(t, filepath.Join(opts.ClaudeConfigDir, ".claude.json"), `{"mcpServers":{"override":{"command":"echo"}}}`)
	write(t, filepath.Join(opts.PiAgentDir, "mcp.json"), `{"mcpServers":{"pi-override":{"url":"https://override.invalid"}}}`)
	write(t, filepath.Join(f.Home, ".claude.json"), `{"mcpServers":{"ignored":{"command":"no"}}}`)
	skill(t, filepath.Join(f.Home, ".agents", "skills", "shared"), "shared")
	r := run(t, opts)
	if len(r.Items) != 3 || r.Attention() {
		t.Fatalf("overrides: %+v", r.Items)
	}
	for _, raw := range []string{`{"mcpServers":{"broken":"SENTINEL"}} garbage`, `{"mcpServers":[]}`, `{"projects":{"/a":42}}`, `{"projects":[]}`, `{"mcpServers":{"broken":{"env":"SENTINEL"}}}`} {
		write(t, filepath.Join(opts.ClaudeConfigDir, ".claude.json"), raw)
		r = run(t, opts)
		if !r.Attention() {
			t.Fatal("malformed source accepted")
		}
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		testutil.AssertNoSecret(t, f.Project, "SENTINEL", string(data))
	}
}

func TestGeneratedLibraryInstructionsExcluded(t *testing.T) {
	f, opts := fixture(t, false)
	if err := library.Init(f.Library); err != nil {
		t.Fatal(err)
	}
	if r := run(t, opts); len(r.Items) != 0 {
		t.Fatalf("root guide became import result: %+v", r.Items)
	}
	// A native agent alias must retain the existing source-confinement guard.
	link(t, filepath.Join(f.Library, "AGENTS.md"), filepath.Join(f.Home, ".claude", "agents", "guide.md"))
	r := run(t, opts)
	if len(r.Items) != 1 || r.Items[0].Outcome != plan.Skipped || !strings.Contains(r.Items[0].Reason, "library") {
		t.Fatalf("library source link not excluded: %+v", r.Items)
	}
	// AGENTS.md remains a legitimate skill support file.
	native := filepath.Join(f.Home, ".claude", "skills", "review")
	skill(t, native, "review")
	write(t, filepath.Join(native, "AGENTS.md"), "handwritten support instructions\r\n")
	r = run(t, opts)
	found := false
	for _, i := range r.Items {
		if i.Name == "review" && i.Outcome == plan.Applied {
			found = true
		}
	}
	if !found {
		t.Fatalf("skill not imported: %+v", r.Items)
	}
	data, err := os.ReadFile(filepath.Join(f.Library, "skills", "review", "AGENTS.md"))
	if err != nil || string(data) != "handwritten support instructions\r\n" {
		t.Fatalf("support guide: %q %v", data, err)
	}
}
