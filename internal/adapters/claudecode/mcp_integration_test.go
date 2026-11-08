package claudecode_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/claudecode"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func writeMCPFixture(t *testing.T, root, path, body string) {
	t.Helper()
	p := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func fixtureMCPEntries(t *testing.T, f testutil.Fixture) ([]contract.Entry, *manifest.Manifest) {
	t.Helper()
	lib, err := library.Load(f.Library)
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.LoadManifest(filepath.Join(f.Project, "daypack.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if problems := m.Validate(lib); len(problems) != 0 {
		t.Fatal(problems)
	}
	var entries []contract.Entry
	for _, e := range m.Entries() {
		var artifact library.Artifact
		if e.Inline == nil {
			artifact, err = lib.Resolve(string(e.Type) + ":" + e.Name)
			if err != nil {
				t.Fatal(err)
			}
		}
		entries = append(entries, contract.FromManifest(e, artifact, nil))
	}
	return entries, m
}

func TestMCPFixturePipeline(t *testing.T) {
	for _, tc := range []struct {
		name, harnesses, selection string
		count, unsupported         int
	}{
		{"inherited", `["claude-code","pi"]`, `"docs"`, 6, 3},
		{"claude", `["claude-code"]`, `"docs"`, 6, 0},
		{"pi", `["pi"]`, `"docs"`, 0, 3},
		{"restricted", `["claude-code","pi"]`, `{name="docs",harnesses=["pi"]}`, 4, 3},
		{"claude-restricted", `["claude-code","pi"]`, `{name="docs",harnesses=["claude-code"]}`, 6, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := testutil.NewFixture(t, false)
			sentinel := testutil.PlantSentinel(t, f)
			writeMCPFixture(t, f.Library, "mcp/remote.toml", "type='http'\nurl='https://example.invalid/${RESOURCE:-docs}'\n[headers]\nAuthorization='Bearer ${DAYPACK_TEST_SECRET}'\n")
			writeMCPFixture(t, f.Project, "daypack.toml", "version=1\nharnesses="+tc.harnesses+"\nmcp=["+tc.selection+",'remote']\n[inline.mcp.local]\ntype='stdio'\ncommand='echo'\n[inline.mcp.local.env]\nTOKEN='${DAYPACK_TEST_SECRET}'\n")
			before := testutil.SnapshotTree(t, f.Project)
			libBefore := testutil.SnapshotTree(t, f.Library)
			entries, m := fixtureMCPEntries(t, f)
			if len(entries) != 3 || entries[0].Inline || !entries[2].Inline || entries[2].Artifact.Path != "" {
				t.Fatal("inline provenance lost")
			}
			r, err := adapters.NewRegistry(claudecode.New(), testadapter.New("pi", false))
			if err != nil {
				t.Fatal(err)
			}
			got, err := r.Desired(entries, contract.Project{Root: f.Project, Harnesses: m.Harnesses})
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Items) != tc.count || len(got.Unsupported) != tc.unsupported {
				t.Fatalf("items=%d unsupported=%d", len(got.Items), len(got.Unsupported))
			}
			counts := map[string]int{}
			for _, i := range got.Items {
				counts[i.Entry]++
			}
			for _, n := range counts {
				if n != 2 {
					t.Fatal("server and approval separated")
				}
			}
			// The same schema renders identically for library and inline provenance.
			a := claudecode.New()
			left, err := a.Desired(entries[0], contract.Project{})
			if err != nil {
				t.Fatal(err)
			}
			inline := contract.FromManifest(manifest.Entry{Type: library.MCP, Name: "docs", Inline: entries[0].Artifact.MCP}, library.Artifact{}, nil)
			right, err := a.Desired(inline, contract.Project{})
			if err != nil || !reflect.DeepEqual(left, right) {
				t.Fatal("inline rendering differs")
			}
			empty, err := r.Desired(nil, contract.Project{Harnesses: m.Harnesses})
			if err != nil || len(empty.Items) != 0 {
				t.Fatal("empty render")
			}
			data, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			outputs := []string{string(data)}
			for _, i := range got.Items {
				// Item JSON base64-encodes []byte; scan the payload itself too.
				outputs = append(outputs, string(i.Value))
			}
			testutil.AssertNoSecret(t, f.Project, sentinel, outputs...)
			testutil.AssertTree(t, f.Project, before)
			testutil.AssertTree(t, f.Library, libBefore)
			// Inline names never silently shadow library names.
			writeMCPFixture(t, f.Project, "daypack.toml", "version=1\nharnesses=['claude-code']\n[inline.mcp.docs]\ntype='stdio'\ncommand='echo'\n")
			lib, err := library.Load(f.Library)
			if err != nil {
				t.Fatal(err)
			}
			bad, err := manifest.LoadManifest(filepath.Join(f.Project, "daypack.toml"))
			if err != nil {
				t.Fatal(err)
			}
			if len(bad.Validate(lib)) == 0 {
				t.Fatal("accepted inline collision")
			}
		})
	}
}

var mcpRoots = []string{".mcp.json", ".claude/settings.local.json"}

// MCP composition below exercises the existing lock APIs, not a production
// apply/status command. Array values are decoded to raw membership names.
func mcpRow(t *testing.T, root string, i contract.Item) lock.Item {
	t.Helper()
	row := lock.Item{Target: i.Target, Entry: i.Entry, Kind: i.Kind, Path: i.Path, Key: i.Key}
	var err error
	row.Tracked, err = lock.IsTracked(root, i.Path)
	if err != nil {
		t.Fatal(err)
	}
	if i.Kind == contract.JSONArrayElement {
		if err := json.Unmarshal(i.Value, &row.Value); err != nil {
			t.Fatal(err)
		}
	} else {
		row.Hash, err = lock.HashJSON(i.Value)
		if err != nil {
			t.Fatal(err)
		}
	}
	return row
}

func composeMCP(t *testing.T, root string, l *lock.Lock, i contract.Item) error {
	t.Helper()
	row := mcpRow(t, root, i)
	if old, ok := l.Find(row); ok {
		row.Tracked = old.Tracked
	}
	obs, err := l.Status(root, row)
	if err != nil {
		return err
	}
	if obs.State == lock.Collision || obs.State == lock.Drifted {
		return fmt.Errorf("refuse %s", obs.State)
	}
	row.Preserve = obs.Preserve
	data, err := os.ReadFile(filepath.Join(root, i.Path))
	if os.IsNotExist(err) {
		data = []byte("{}")
	} else if err != nil {
		return err
	}
	next, err := lock.SetJSON(data, row, i.Value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, i.Path)), 0700); err != nil {
		return err
	}
	if err := lock.WriteFile(root, i.Path, mcpRoots, next, 0600); err != nil {
		return err
	}
	if err := l.Put(row); err != nil {
		return err
	}
	if err := l.Save(root); err != nil {
		return err
	}
	return lock.UpdateExclude(root, l.Items())
}

func renderMCP(t *testing.T, name string, d library.MCPDefinition) []contract.Item {
	t.Helper()
	items, err := claudecode.New().Desired(mcp(name, d), contract.Project{})
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func readMCPJSON(t *testing.T, root, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMCPOwnershipLifecycle(t *testing.T) {
	f := testutil.NewFixture(t, false)
	sentinel := testutil.PlantSentinel(t, f)
	writeMCPFixture(t, f.Project, ".mcp.json", `{"mcpServers":{"hand":{"command":"hand"}},"version":7}`)
	writeMCPFixture(t, f.Project, ".claude/settings.local.json", `{"permissions":{"allow":["Read"]},"enabledMcpjsonServers":["docs","hand"]}`)
	d := library.MCPDefinition{Type: "stdio", Command: "echo", Env: map[string]string{"TOKEN": "${DAYPACK_TEST_SECRET}"}}
	items := append(renderMCP(t, "docs", d), renderMCP(t, "new", library.MCPDefinition{
		Type: "http", URL: "https://example.invalid/${RESOURCE:-docs}",
		Headers: map[string]string{"Authorization": "Bearer ${DAYPACK_TEST_SECRET}"},
	})...)
	l := lock.New()
	for _, i := range items {
		row := mcpRow(t, f.Project, i)
		obs, err := l.Status(f.Project, row)
		if err != nil {
			t.Fatal(err)
		}
		if i.Entry == "mcp:docs" && i.Kind == contract.JSONArrayElement {
			if !obs.Adopt || !obs.Preserve || !obs.Converged {
				t.Fatal("preapproval not adopted")
			}
		} else if obs.State != lock.Missing {
			t.Fatal("expected missing item")
		}
		if err := composeMCP(t, f.Project, l, i); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := lock.Load(f.Project)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(l.Items(), loaded.Items()) {
		t.Fatal("ownership persistence mismatch")
	}
	l = loaded
	tree := testutil.SnapshotTree(t, f.Project)
	for _, i := range items {
		obs, err := l.Status(f.Project, mcpRow(t, f.Project, i))
		if err != nil || !obs.Converged || obs.State != lock.OK {
			t.Fatal("not converged")
		}
		path := filepath.Join(f.Project, i.Path)
		before, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := composeMCP(t, f.Project, l, i); err != nil {
			t.Fatal(err)
		}
		after, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
			t.Fatal("no-op replaced file")
		}
	}
	testutil.AssertTree(t, f.Project, tree)
	d.Args = []string{"changed", "${OPTION:-default}"}
	updated := renderMCP(t, "docs", d)[0]
	obs, err := l.Status(f.Project, mcpRow(t, f.Project, updated))
	if err != nil || obs.State != lock.OK || obs.Converged {
		t.Fatal("update not recognized")
	}
	if err := composeMCP(t, f.Project, l, updated); err != nil {
		t.Fatal(err)
	}
	config := readMCPJSON(t, f.Project, ".mcp.json")
	if config["version"] != float64(7) || config["mcpServers"].(map[string]any)["hand"].(map[string]any)["command"] != "hand" {
		t.Fatal("handwritten config lost")
	}
	docs := config["mcpServers"].(map[string]any)["docs"].(map[string]any)
	if docs["env"].(map[string]any)["TOKEN"] != "${DAYPACK_TEST_SECRET}" || !reflect.DeepEqual(docs["args"], []any{"changed", "${OPTION:-default}"}) {
		t.Fatal("written references were expanded or changed")
	}
	settings := readMCPJSON(t, f.Project, ".claude/settings.local.json")
	if settings["permissions"] == nil {
		t.Fatal("settings lost")
	}
	// Editing one managed entry refuses both update and cleanup, without writes.
	row := mcpRow(t, f.Project, updated)
	data, err := os.ReadFile(filepath.Join(f.Project, row.Path))
	if err != nil {
		t.Fatal(err)
	}
	edited, err := lock.SetJSON(data, row, []byte(`{"command":"hand edit"}`))
	if err != nil {
		t.Fatal(err)
	}
	writeMCPFixture(t, f.Project, row.Path, string(edited))
	tree = testutil.SnapshotTree(t, f.Project)
	obs, err = l.Status(f.Project, row)
	if err != nil || obs.State != lock.Drifted {
		t.Fatal("drift missed")
	}
	if err := composeMCP(t, f.Project, l, updated); err == nil {
		t.Fatal("overwrote drift")
	}
	if err := l.RemoveOwned(f.Project, row, mcpRoots); err == nil {
		t.Fatal("removed drift")
	}
	testutil.AssertTree(t, f.Project, tree)
	writeMCPFixture(t, f.Project, row.Path, string(data))
	for _, row := range l.Items() {
		if err := l.RemoveOwned(f.Project, row, mcpRoots); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Save(f.Project); err != nil {
		t.Fatal(err)
	}
	settings = readMCPJSON(t, f.Project, ".claude/settings.local.json")
	if !reflect.DeepEqual(settings["enabledMcpjsonServers"], []any{"docs", "hand"}) {
		t.Fatal("cleanup lost preapproval or retained managed approval")
	}
	config = readMCPJSON(t, f.Project, ".mcp.json")
	if len(config["mcpServers"].(map[string]any)) != 1 {
		t.Fatal("cleanup changed siblings")
	}
	// A different unowned same-name definition is a collision.
	collision := renderMCP(t, "hand", d)[0]
	tree = testutil.SnapshotTree(t, f.Project)
	obs, err = l.Status(f.Project, mcpRow(t, f.Project, collision))
	if err != nil || obs.State != lock.Collision {
		t.Fatal("collision missed")
	}
	if err := composeMCP(t, f.Project, l, collision); err == nil {
		t.Fatal("overwrote collision")
	}
	if err := l.RemoveOwned(f.Project, mcpRow(t, f.Project, collision), mcpRoots); err == nil {
		t.Fatal("removed unowned collision")
	}
	testutil.AssertTree(t, f.Project, tree)
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	testutil.AssertNoSecret(t, f.Project, sentinel, string(raw))
}

func TestMCPIdenticalServerAdoption(t *testing.T) {
	f := testutil.NewFixture(t, false)
	sentinel := testutil.PlantSentinel(t, f)
	i := renderMCP(t, "docs", library.MCPDefinition{Type: "stdio", Command: "echo"})[0]
	writeMCPFixture(t, f.Project, i.Path, `{"mcpServers":{"docs":{"command":"echo"}},"keep":true}`)
	l := lock.New()
	row := mcpRow(t, f.Project, i)
	obs, err := l.Status(f.Project, row)
	if err != nil || !obs.Adopt || !obs.Converged || obs.Preserve {
		t.Fatal("identical server not adoptable")
	}
	before, err := os.ReadFile(filepath.Join(f.Project, i.Path))
	if err != nil {
		t.Fatal(err)
	}
	if err := composeMCP(t, f.Project, l, i); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(f.Project, i.Path))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("adoption changed server file")
	}
	testutil.AssertNoSecret(t, f.Project, sentinel)
}

func TestMCPMalformedContainers(t *testing.T) {
	for _, tc := range []struct {
		path, body string
		index      int
	}{
		{".mcp.json", `{`, 0}, {".mcp.json", `{"mcpServers":[]}`, 0}, {".mcp.json", `{"mcpServers":null}`, 0},
		{".claude/settings.local.json", `{"enabledMcpjsonServers":{}}`, 1},
		{".claude/settings.local.json", `{"enabledMcpjsonServers":[7]}`, 1},
		{".claude/settings.local.json", `{"enabledMcpjsonServers":["docs","docs"]}`, 1},
		{".claude/settings.local.json", `{"enabledMcpjsonServers":null}`, 1},
	} {
		t.Run(tc.body, func(t *testing.T) {
			f := testutil.NewFixture(t, false)
			sentinel := testutil.PlantSentinel(t, f)
			writeMCPFixture(t, f.Project, tc.path, tc.body)
			items := renderMCP(t, "docs", library.MCPDefinition{Type: "stdio", Command: "echo"})
			before := testutil.SnapshotTree(t, f.Project)
			err := composeMCP(t, f.Project, lock.New(), items[tc.index])
			if err == nil {
				t.Fatal("accepted malformed container")
			}
			testutil.AssertTree(t, f.Project, before)
			testutil.AssertNoSecret(t, f.Project, sentinel, err.Error())
		})
	}
}

func mcpGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root, "-c", "core.hooksPath=" + os.DevNull}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture git: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestMCPTrackingAndWorktreeExclusion(t *testing.T) {
	for _, worktree := range []bool{false, true} {
		for _, trackedPath := range []string{"", ".mcp.json", ".claude/settings.local.json"} {
			t.Run(fmt.Sprintf("worktree=%v/tracked=%s", worktree, trackedPath), func(t *testing.T) {
				f := testutil.NewFixture(t, true)
				sentinel := testutil.PlantSentinel(t, f)
				root := f.Project
				if worktree {
					root = f.Worktree
				}
				if trackedPath != "" {
					writeMCPFixture(t, root, trackedPath, `{}`)
					mcpGit(t, root, "add", "-f", "--", trackedPath)
				}
				exclude := mcpGit(t, root, "rev-parse", "--git-path", "info/exclude")
				if !filepath.IsAbs(exclude) {
					exclude = filepath.Join(root, exclude)
				}
				old, err := os.ReadFile(exclude)
				if err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				old = append(old, []byte("\n# handwritten\n/private-cache\n")...)
				if err := os.MkdirAll(filepath.Dir(exclude), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(exclude, old, 0600); err != nil {
					t.Fatal(err)
				}
				l := lock.New()
				var items []contract.Item
				for _, name := range []string{"docs", "other"} {
					items = append(items, renderMCP(t, name, library.MCPDefinition{Type: "stdio", Command: "echo", Env: map[string]string{"TOKEN": "${DAYPACK_TEST_SECRET}"}})...)
				}
				for _, i := range items {
					if err := composeMCP(t, root, l, i); err != nil {
						t.Fatal(err)
					}
				}
				for _, row := range l.Items() {
					if row.Tracked != (row.Path == trackedPath) {
						t.Fatal("incorrect per-file tracking")
					}
				}
				block, err := lock.ExcludeBlock(root, l.Items())
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range mcpRoots {
					included := strings.Contains(block, "/"+p+"\n")
					if included == (p == trackedPath) {
						t.Fatal("wrong exclude classification")
					}
				}
				after, err := os.ReadFile(exclude)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.HasPrefix(after, old) {
					t.Fatal("handwritten excludes changed")
				}
				loaded, err := lock.Load(root)
				if err != nil || !reflect.DeepEqual(l.Items(), loaded.Items()) {
					t.Fatal("tracking persistence mismatch")
				}
				if worktree {
					info, err := os.Stat(filepath.Join(root, ".git"))
					if err != nil || !info.Mode().IsRegular() {
						t.Fatal("worktree git file changed")
					}
				}
				raw, err := json.Marshal(items)
				if err != nil {
					t.Fatal(err)
				}
				testutil.AssertNoSecret(t, root, sentinel, string(raw), string(after))
				testutil.AssertNoSecret(t, f.Project, sentinel)
			})
		}
	}
}
