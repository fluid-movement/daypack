package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/status"
	"github.com/fluid-movement/daypack/internal/testutil"
)

const codexUserConfig = "# user configuration\nmodel = 'my-model' # preserve exactly\n[mcp_servers]\n# user container\n[mcp_servers.mine]\ncommand = 'my-command'\n\n[preferences]\ncustom = 'value'\n"

func codexWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func codexRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func codexRun(t *testing.T, f testutil.Fixture, code int, args ...string) testutil.CLIResult {
	t.Helper()
	r := testutil.RunCLI(t, f, args...)
	if r.Code != code {
		t.Fatalf("%v: %+v", args, r)
	}
	return r
}
func codexFixture(t *testing.T) (testutil.Fixture, string) {
	t.Helper()
	f := testutil.NewFixture(t, false)
	sentinel := testutil.PlantSentinel(t, f)
	codexWrite(t, filepath.Join(f.Library, "mcp/docs.toml"), "type='stdio'\ncommand='echo'\nargs=['hello']\n[env]\nDAYPACK_TEST_SECRET='${DAYPACK_TEST_SECRET}'\nMODE='offline'\n")
	setManifest(t, f, "version=1\nharnesses=['codex']\nskills=['testing']\nfragments=['conventions']\nmcp=['docs']\nagents=['reviewer']\n")
	return f, sentinel
}
func codexItems(t *testing.T, f testutil.Fixture) []lock.Item {
	t.Helper()
	l, err := lock.Load(f.Project)
	if err != nil {
		t.Fatal(err)
	}
	return l.Items()
}
func TestCodexApplyStatusLifecycle(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		t.Run(map[bool]string{false: "untracked", true: "tracked"}[tracked], func(t *testing.T) {
			f, sentinel := codexFixture(t)
			cfg := filepath.Join(f.Project, ".codex/config.toml")
			codexWrite(t, cfg, codexUserConfig)
			if tracked {
				if out, err := exec.Command("git", "-C", f.Project, "add", ".codex/config.toml").CombinedOutput(); err != nil {
					t.Fatal(err, string(out))
				}
			}
			// Exercise a library root symlink; links must continue to point at usable skills.
			alias := filepath.Join(t.TempDir(), "library-link")
			if err := os.Symlink(f.Library, alias); err != nil {
				t.Fatal(err)
			}
			f.Library = alias
			codexWrite(t, filepath.Join(f.Project, ".git/info/exclude"), "# user excludes\n/cache/\n")
			before := testutil.SnapshotTree(t, f.Project)
			excludes := codexRead(t, filepath.Join(f.Project, ".git/info/exclude"))
			r := codexRun(t, f, 3, "apply", "--dry-run", "--json")
			testutil.AssertTree(t, f.Project, before)
			if !bytes.Equal(excludes, codexRead(t, filepath.Join(f.Project, ".git/info/exclude"))) {
				t.Fatal("dry run changed excludes")
			}
			testutil.AssertNoSecret(t, f.Project, sentinel, r.Stdout, r.Stderr)
			r = codexRun(t, f, 0, "apply", "--json")
			testutil.AssertNoSecret(t, f.Project, sentinel, r.Stdout, r.Stderr)
			config := codexRead(t, cfg)
			if !bytes.HasPrefix(config, []byte(codexUserConfig)) {
				t.Fatal("unrelated TOML bytes changed")
			}
			var native map[string]any
			if _, err := toml.Decode(string(config), &native); err != nil {
				t.Fatal(err)
			}
			server := native["mcp_servers"].(map[string]any)["docs"].(map[string]any)
			if !reflect.DeepEqual(server["env_vars"], []any{"DAYPACK_TEST_SECRET"}) {
				t.Fatal(server)
			}
			if len(codexItems(t, f)) != 4 {
				t.Fatal(codexItems(t, f))
			}
			if _, err := os.Stat(filepath.Join(f.Project, ".agents/skills/testing/support.txt")); err != nil {
				t.Fatal("symlink library skill unavailable", err)
			}
			block := string(codexRead(t, filepath.Join(f.Project, ".git/info/exclude")))
			if strings.Contains(block, "/.codex/\n") || strings.Contains(block, "/.codex/config.toml\n") == tracked {
				t.Fatal("wrong excludes", block)
			}
			state := codexRun(t, f, 0, "status", "--json")
			if !strings.Contains(state.Stdout, "project-trust-required") || !strings.Contains(r.Stdout, "project-trust-required") {
				t.Fatal("trust note absent")
			}
			before = testutil.SnapshotTree(t, f.Project)
			r = codexRun(t, f, 0, "apply", "--json")
			testutil.AssertTree(t, f.Project, before)
			testutil.AssertNoSecret(t, f.Project, sentinel, r.Stdout, r.Stderr, state.Stdout, state.Stderr)
			// A library update replaces only our subtree and agent file.
			codexWrite(t, filepath.Join(f.Library, "mcp/docs.toml"), "type='stdio'\ncommand='new-command'\n[env]\nDAYPACK_TEST_SECRET='${DAYPACK_TEST_SECRET}'\n")
			codexWrite(t, filepath.Join(f.Library, "agents/reviewer.md"), "---\nname: reviewer\ndescription: Updated reviewer\nmodel: explicit\n---\nNew instructions.\n")
			codexRun(t, f, 0, "apply")
			if !bytes.HasPrefix(codexRead(t, cfg), []byte(codexUserConfig)) || !bytes.Contains(codexRead(t, cfg), []byte("new-command")) {
				t.Fatal("update failed")
			}
			// Eject by removing manifest references; preserve the existing container.
			setManifest(t, f, "version=1\nharnesses=['codex']\n")
			codexRun(t, f, 0, "apply")
			if !bytes.Equal(codexRead(t, cfg), []byte(codexUserConfig)) || len(codexItems(t, f)) != 0 {
				t.Fatal("removal touched user config")
			}
			if _, err := os.Lstat(filepath.Join(f.Project, ".codex/agents/reviewer.toml")); !os.IsNotExist(err) {
				t.Fatal("agent retained", err)
			}
		})
	}
}
func TestCodexCollisionsDriftForceAndEject(t *testing.T) {
	f, sentinel := codexFixture(t)
	cfg := filepath.Join(f.Project, ".codex/config.toml")
	agentPath := filepath.Join(f.Project, ".codex/agents/reviewer.toml")
	codexWrite(t, cfg, codexUserConfig+"[mcp_servers.docs]\ncommand='handwritten'\n")
	codexWrite(t, agentPath, "name='handwritten'\n")
	codexRun(t, f, 3, "apply")
	if !bytes.Contains(codexRead(t, cfg), []byte("handwritten")) {
		t.Fatal("collision overwritten")
	}
	codexRun(t, f, 0, "apply", "--adopt")
	edited := bytes.ReplaceAll(codexRead(t, cfg), []byte(`command = "echo"`), []byte(`command = "edited"`))
	codexWrite(t, cfg, string(edited))
	codexWrite(t, agentPath, "name='edited'\n")
	r := codexRun(t, f, 3, "status", "--json")
	if !strings.Contains(r.Stdout, `"drift"`) {
		t.Fatal(r.Stdout)
	}
	codexRun(t, f, 3, "apply")
	if !bytes.Equal(codexRead(t, cfg), edited) {
		t.Fatal("drift overwritten")
	}
	codexRun(t, f, 0, "apply", "--force")
	// Semantic TOML hashing tolerates harmless reformatting.
	formatted := bytes.ReplaceAll(codexRead(t, cfg), []byte(`command = "echo"`), []byte("command='echo' # formatting"))
	codexWrite(t, cfg, string(formatted))
	codexRun(t, f, 0, "status")
	codexRun(t, f, 0, "apply")
	if !bytes.Equal(codexRead(t, cfg), formatted) {
		t.Fatal("format-only edit rewritten")
	}
	// Exercise the existing core eject primitive; eject has no CLI command.
	owned, err := lock.Load(f.Project)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range owned.Items() {
		if err := owned.RemoveOwned(f.Project, i, []string{".codex/config.toml", ".codex/agents", ".agents/skills", "AGENTS.md"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := owned.Save(f.Project); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(codexRead(t, cfg), []byte(codexUserConfig)) || len(codexItems(t, f)) > 0 {
		t.Fatal("eject lost user bytes")
	}
	testutil.AssertNoSecret(t, f.Project, sentinel, r.Stdout, r.Stderr)
}
func TestCodexUnsafeConfigAndSymlinks(t *testing.T) {
	for _, layout := range []string{"malformed", "inline", "parent-link", "leaf-link"} {
		t.Run(layout, func(t *testing.T) {
			f, sentinel := codexFixture(t)
			cfg := filepath.Join(f.Project, ".codex/config.toml")
			external := t.TempDir()
			codexWrite(t, filepath.Join(external, "config.toml"), codexUserConfig)
			switch layout {
			case "malformed":
				codexWrite(t, cfg, "secret='unterminated")
			case "inline":
				codexWrite(t, cfg, "mcp_servers={docs={command='echo'}}")
			case "parent-link":
				if err := os.Symlink(external, filepath.Join(f.Project, ".codex")); err != nil {
					t.Fatal(err)
				}
			case "leaf-link":
				if err := os.MkdirAll(filepath.Dir(cfg), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(external, "config.toml"), cfg); err != nil {
					t.Fatal(err)
				}
			}
			before := testutil.SnapshotTree(t, f.Project)
			outside := testutil.SnapshotTree(t, external)
			r := testutil.RunCLI(t, f, "apply", "--adopt", "--force")
			if r.Code == 0 {
				t.Fatal("accepted unsafe target")
			}
			testutil.AssertTree(t, f.Project, before)
			testutil.AssertTree(t, external, outside)
			testutil.AssertNoSecret(t, f.Project, sentinel, r.Stdout, r.Stderr)
		})
	}
}
func TestCodexSharedSelectionAndRetention(t *testing.T) {
	f, _ := codexFixture(t)
	manifest := func(harnesses string) {
		setManifest(t, f, "version=1\nharnesses="+harnesses+"\nskills=['testing']\nfragments=['conventions']\n")
	}
	manifest("['codex','pi','claude-code']")
	codexRun(t, f, 0, "apply")
	if len(codexItems(t, f)) != 4 {
		t.Fatal("shared resources duplicated")
	}
	manifest("['codex','pi']")
	codexRun(t, f, 0, "apply")
	if len(codexItems(t, f)) != 2 {
		t.Fatal(codexItems(t, f))
	}
	if _, err := os.Stat(filepath.Join(f.Project, "CLAUDE.md")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	r := codexRun(t, f, 3, "status", "--json") // Pi trust remains an existing independent check.
	var envelope struct {
		Status status.Report `json:"status"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	for _, readers := range envelope.Status.Readers {
		if readers.Target == contract.AgentsMD || readers.Target == contract.AgentsSkillsDir {
			if !reflect.DeepEqual(readers.Selected, []string{"codex", "pi"}) {
				t.Fatal(readers)
			}
			found := false
			for _, reader := range readers.Readers {
				if reader == "codex" {
					found = true
				}
			}
			if !found {
				t.Fatal("native reader missing")
			}
		}
	}
	for _, h := range []string{"codex", "pi"} {
		manifest("['" + h + "']")
		codexRun(t, f, 0, "apply")
		if len(codexItems(t, f)) != 2 {
			t.Fatal("last selecting reader lost items")
		}
	}
	setManifest(t, f, "version=1\nharnesses=['codex']\n")
	codexRun(t, f, 0, "apply")
	if len(codexItems(t, f)) > 0 {
		t.Fatal("last-reader removal failed")
	}
	if _, err := os.Lstat(filepath.Join(f.Project, ".agents/skills/testing")); !os.IsNotExist(err) {
		t.Fatal("shared link retained", err)
	}
	if strings.Contains(string(codexRead(t, filepath.Join(f.Project, "AGENTS.md"))), "daypack:begin") {
		t.Fatal("shared section retained")
	}
}
func TestCodexInitAndOnlySelection(t *testing.T) {
	f := testutil.NewFixture(t, false)
	if err := os.Remove(manifestPath(f)); err != nil {
		t.Fatal(err)
	}
	codexRun(t, f, 0, "init", "--harness", "codex")
	codexRun(t, f, 0, "add", "skill:testing", "--only", "codex")
	// Updating an existing selection takes the manifest SetEntry validation path.
	codexRun(t, f, 0, "add", "skill:testing", "--only", "codex")
	if !strings.Contains(readManifest(t, f), `harnesses = ["codex"]`) {
		t.Fatal(readManifest(t, f))
	}
	codexRun(t, f, 0, "apply")
	if len(codexItems(t, f)) != 1 {
		t.Fatal(codexItems(t, f))
	}
}

func TestCodexUnsupportedUpdateRetainsManagedServer(t *testing.T) {
	f, sentinel := codexFixture(t)
	codexRun(t, f, 0, "apply")
	cfg := filepath.Join(f.Project, ".codex/config.toml")
	before := codexRead(t, cfg)
	codexWrite(t, filepath.Join(f.Library, "mcp/docs.toml"), "type='stdio'\ncommand='${COMMAND}'\nargs=['${ARG}']\n[env]\nTOKEN='${DAYPACK_TEST_SECRET}'\n")
	r := codexRun(t, f, 3, "apply", "--json")
	if !strings.Contains(r.Stdout, "unsupported fields: args[0], command, env.TOKEN") {
		t.Fatal(r.Stdout)
	}
	if !bytes.Equal(before, codexRead(t, cfg)) {
		t.Fatal("unsupported update deleted old server")
	}
	testutil.AssertNoSecret(t, f.Project, sentinel, r.Stdout, r.Stderr)
}
