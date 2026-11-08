package pi_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/pi"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/fluid-movement/daypack/internal/plan"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func fixtureEntries(t *testing.T, f testutil.Fixture, root string) ([]contract.Entry, contract.Project) {
	t.Helper()
	lib, err := library.Load(f.Library)
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.LoadManifest(filepath.Join(root, "daypack.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if problems := m.Validate(lib); len(problems) != 0 {
		t.Fatal(problems)
	}
	var entries []contract.Entry
	for _, e := range m.Entries() {
		var a library.Artifact
		if e.Inline == nil {
			a, err = lib.Resolve(string(e.Type) + ":" + e.Name)
			if err != nil {
				t.Fatal(err)
			}
		}
		ce := contract.FromManifest(e, a, nil)
		if ce.Inline && ce.Artifact.Path != "" {
			t.Fatal("inline unexpectedly has a source path")
		}
		entries = append(entries, ce)
	}
	return entries, contract.Project{Root: root, Harnesses: m.Harnesses}
}
func piNotes(notes []contract.Note) []contract.Note {
	var out []contract.Note
	for _, n := range notes {
		if n.Harness == "pi" {
			out = append(out, n)
		}
	}
	return out
}

func productionRegistry(t *testing.T) *adapters.Registry {
	t.Helper()
	r, err := adapters.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestMCPFixturePipeline(t *testing.T) {
	for _, tc := range []struct {
		name, harnesses, selection string
		items, unsupported         int
	}{
		{"pi", `["pi"]`, `["docs", "remote"]`, 4, 0},
		{"both", `["claude-code", "pi"]`, `["docs", "remote"]`, 12, 0},
		{"restricted", `["claude-code", "pi"]`, `[{name="docs",harnesses=["claude-code"]}, {name="remote",harnesses=["pi"]}]`, 9, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := testutil.NewFixture(t, false)
			sentinel := testutil.PlantSentinel(t, f)
			writeTestFile(t, filepath.Join(f.Library, "mcp/remote.toml"), []byte("type='http'\nurl='https://example.invalid/mcp'\ndescription='Remote'\n[headers]\nAuthorization='Bearer ${DAYPACK_TEST_SECRET}'\n"))
			body := "version=1\nharnesses=" + tc.harnesses + "\nmcp=" + tc.selection + ` 
[inline.mcp.local]
type='stdio'
command='not-a-real-executable'
args=['second', 'first']
env={TOKEN='${DAYPACK_TEST_SECRET}'}
[inline.mcp.web]
type='http'
url='https://example.invalid/inline'
headers={Authorization='Bearer ${DAYPACK_TEST_SECRET}'}
`
			writeTestFile(t, filepath.Join(f.Project, "daypack.toml"), []byte(body))
			projectBefore := testutil.SnapshotTree(t, f.Project)
			libraryBefore := testutil.SnapshotTree(t, f.Library)
			homeBefore := testutil.SnapshotTree(t, f.Home)
			entries, p := fixtureEntries(t, f, f.Project)
			result, err := productionRegistry(t).Desired(entries, p)
			if err != nil || len(result.Items) != tc.items || len(result.Unsupported) != tc.unsupported {
				t.Fatalf("pipeline counts: %d items, %d unsupported, error %v", len(result.Items), len(result.Unsupported), err)
			}
			if pn := piNotes(result.Notes); len(pn) != 1 || pn[0].Code != "project-trust-required" {
				t.Fatal("missing trust requirement")
			}
			output, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range result.Items {
				if item.Kind == contract.JSONArrayElement {
					continue // Claude approval membership holds only the server name
				}
				if !bytes.Contains(item.Value, []byte("${DAYPACK_TEST_SECRET}")) {
					t.Fatal("reference lost")
				}
				testutil.AssertNoSecret(t, f.Project, sentinel, string(item.Value))
			}
			testutil.AssertNoSecret(t, f.Project, sentinel, string(output))
			testutil.AssertTree(t, f.Project, projectBefore)
			testutil.AssertTree(t, f.Library, libraryBefore)
			testutil.AssertTree(t, f.Home, homeBefore)
		})
	}
}

func TestMCPMixedCapabilitiesAndPlan(t *testing.T) {
	f := testutil.NewFixture(t, false)
	sentinel := testutil.PlantSentinel(t, f)
	// A hypothetical command is never executed, even when rendering fails.
	entries := []contract.Entry{
		{Ref: "mcp:bad", Artifact: library.Artifact{Type: library.MCP, Name: "bad", MCP: &library.MCPDefinition{Type: "stdio", Command: "${PRIVATE_COMMAND}", Args: []string{"$PRIVATE_ARG"}}}},
		mcpEntry(library.MCPDefinition{Type: "stdio", Command: "not-a-real-command", Env: map[string]string{"TOKEN": "${DAYPACK_TEST_SECRET}"}}),
		{Ref: "skill:testing", Artifact: library.Artifact{Type: library.Skill, Name: "testing", Path: filepath.Join(f.Library, "skills/testing")}, Harnesses: []string{"claude-code"}},
	}
	before := testutil.SnapshotTree(t, f.Project)
	libBefore := testutil.SnapshotTree(t, f.Library)
	r := productionRegistry(t)
	result, err := r.Desired(entries, contract.Project{Root: f.Project, Harnesses: []string{"pi", "claude-code"}})
	if err != nil || len(result.Items) != 6 {
		t.Fatalf("other entries did not survive: %d items, err %v", len(result.Items), err)
	}
	var diagnostic contract.Unsupported
	for _, u := range result.Unsupported {
		if u.Harness == "pi" {
			diagnostic = u
		}
	}
	if !reflect.DeepEqual(diagnostic, contract.Unsupported{Harness: "pi", Entry: "mcp:bad", Type: library.MCP, Fields: []string{"args[0]", "command"}}) {
		t.Fatal("missing field diagnostics")
	}
	// The plan package is a presentation seam, not a reconciliation engine.
	p := plan.Plan{Notes: result.Notes, Operations: []plan.Operation{{Harness: diagnostic.Harness, Entry: diagnostic.Entry, Action: plan.Unsupported, Reason: "unsupported fields: " + strings.Join(diagnostic.Fields, ", ")}}}
	var text, encoded bytes.Buffer
	if err := plan.RenderText(&text, p); err != nil {
		t.Fatal(err)
	}
	if err := plan.RenderJSON(&encoded, p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "x unsupported mcp:bad") || !strings.Contains(text.String(), "args[0], command") || !strings.Contains(text.String(), "project-trust-required") {
		t.Fatal("plan projection lost diagnostic")
	}
	var report plan.Report
	if err := json.Unmarshal(encoded.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Summary.Unresolved != 1 || len(report.Notes) != 2 || report.Operations[0].Glyph != "x" {
		t.Fatal("JSON projection lost diagnostic")
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	testutil.AssertNoSecret(t, f.Project, sentinel, string(data), text.String(), encoded.String())
	if bytes.Contains(data, []byte("PRIVATE_COMMAND")) || bytes.Contains(data, []byte("PRIVATE_ARG")) {
		t.Fatal("diagnostics exposed payload")
	}
	bad := mcpEntry(library.MCPDefinition{Type: "stdio", Command: "echo", Env: map[string]string{"TOKEN": sentinel}})
	failed, err := r.Desired([]contract.Entry{bad}, contract.Project{Harnesses: []string{"pi"}})
	if err == nil || len(failed.Items) != 0 {
		t.Fatal("invalid definition accepted")
	}
	testutil.AssertNoSecret(t, f.Project, sentinel, err.Error())
	testutil.AssertTree(t, f.Project, before)
	testutil.AssertTree(t, f.Library, libBefore)
}

func TestPiProjectTrustNote(t *testing.T) {
	r := productionRegistry(t)
	for _, tc := range []struct {
		names []string
		count int
	}{
		{nil, 0}, {[]string{"claude-code"}, 1}, {[]string{"pi"}, 1}, {[]string{"pi", "pi", "claude-code"}, 2},
	} {
		result, err := r.Desired(nil, contract.Project{Harnesses: tc.names})
		if err != nil || len(result.Notes) != tc.count {
			t.Fatal("incorrect selected-project notes")
		}
		if len(piNotes(result.Notes)) == 1 {
			expected := []contract.Note{{Harness: "pi", Code: "project-trust-required", Message: "Pi loads project .agents/skills and .pi/mcp.json only after project trust is granted (/trust); untrusted project config may be silently skipped in non-interactive (headless) mode."}}
			if !reflect.DeepEqual(piNotes(result.Notes), expected) {
				t.Fatal("trust note changed")
			}
		}
	}
	if notes := pi.New().Notes(contract.Project{}); len(notes) != 0 {
		t.Fatal("unselected Pi emitted note")
	}
}

// Compose only existing ownership APIs; production reconciliation is a separate issue.
func TestPiMCPOwnershipLifecycle(t *testing.T) {
	for _, worktree := range []bool{false, true} {
		t.Run(map[bool]string{false: "project", true: "worktree"}[worktree], func(t *testing.T) {
			f := testutil.NewFixture(t, worktree)
			sentinel := testutil.PlantSentinel(t, f)
			root := f.Project
			if worktree {
				root = f.Worktree
			}
			mainBefore := testutil.SnapshotTree(t, f.Project)
			homeBefore := testutil.SnapshotTree(t, f.Home)
			defer testutil.AssertNoSecret(t, root, sentinel)
			defer testutil.AssertTree(t, f.Home, homeBefore)
			if worktree {
				defer testutil.AssertTree(t, f.Project, mainBefore)
			}
			writeTestFile(t, filepath.Join(root, "daypack.toml"), []byte("version=1\nharnesses=['pi']\nmcp=['docs']\n"))
			render := func() contract.Item {
				t.Helper()
				es, p := fixtureEntries(t, f, root)
				result, err := productionRegistry(t).Desired(es, p)
				if err != nil || len(result.Items) != 1 {
					t.Fatal("render failed")
				}
				return result.Items[0]
			}
			item := render()
			var roots []string
			for _, target := range productionRegistry(t).Targets() {
				if target.ID == item.Target {
					roots = append(roots, target.Root)
				}
			}
			if len(roots) != 1 {
				t.Fatal("missing trusted Pi target")
			}
			owned := func(i contract.Item) lock.Item {
				t.Helper()
				hash, err := lock.HashJSON(i.Value)
				if err != nil {
					t.Fatal(err)
				}
				return lock.Item{Target: i.Target, Entry: i.Entry, Kind: i.Kind, Path: i.Path, Key: i.Key, Hash: hash}
			}
			l := lock.New()
			status := func(i lock.Item, state lock.State, converged bool) {
				t.Helper()
				before := testutil.SnapshotTree(t, root)
				obs, err := l.Status(root, i)
				if err != nil || obs.State != state || obs.Converged != converged {
					t.Fatalf("status: %+v %v, wanted %s", obs, err, state)
				}
				testutil.AssertTree(t, root, before)
			}
			persist := func() {
				t.Helper()
				if err := l.Save(root); err != nil {
					t.Fatal(err)
				}
				loaded, err := lock.Load(root)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(l.Items(), loaded.Items()) {
					t.Fatal("lock persistence mismatch")
				}
				l = loaded
			}
			path := filepath.Join(root, item.Path)
			// These exact unknown spans must survive every controlled write and removal.
			sibling := `"handwritten" : { "url" : "https://example.invalid", "auth.provider" : "custom", "enabled": false }`
			unknown := `"future-root" : { "non-schema" : [1, 2] }`
			seed := []byte("{\n  " + unknown + ",\n  \"mcpServers\": { " + sibling + " }\n}\n")
			preserved := func() {
				t.Helper()
				data := readTestFile(t, path)
				if !bytes.Contains(data, []byte(sibling)) || !bytes.Contains(data, []byte(unknown)) {
					t.Fatal("handwritten spans changed")
				}
			}
			row := owned(item)
			status(row, lock.Missing, false)
			writeTestFile(t, path, seed)
			status(row, lock.Missing, false)
			apply := func(i contract.Item) {
				t.Helper()
				o := owned(i)
				obs, err := l.Status(root, o)
				if err != nil || obs.State == lock.Drifted || obs.State == lock.Collision {
					t.Fatal("refused unsafe update")
				}
				data, err := lock.SetJSON(readTestFile(t, path), o, i.Value)
				if err != nil {
					t.Fatal(err)
				}
				if err := lock.WriteFile(root, i.Path, roots, data, 0600); err != nil {
					t.Fatal(err)
				}
				if err := l.Put(o); err != nil {
					t.Fatal(err)
				}
				persist()
				preserved()
			}
			apply(item)
			status(row, lock.OK, true)
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			before := testutil.SnapshotTree(t, root)
			apply(render())
			after, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(info, after) || !info.ModTime().Equal(after.ModTime()) {
				t.Fatal("identical write replaced file")
			}
			testutil.AssertTree(t, root, before)
			// Update the source and use the same loader/manifest/registry route again.
			writeTestFile(t, filepath.Join(f.Library, "mcp/docs.toml"), []byte("type='stdio'\ncommand='echo'\nargs=['updated']\n[env]\nTOKEN='${DAYPACK_TEST_SECRET}'\n"))
			item = render()
			status(owned(item), lock.OK, false)
			apply(item)
			row = owned(item)
			status(row, lock.OK, true)
			managed := readTestFile(t, path)
			// Unknown fields within the owned server count as drift.
			drift, err := lock.SetJSON(managed, lock.Item{Kind: lock.JSONEntry, Key: "mcpServers.docs.user-added"}, []byte("true"))
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, path, drift)
			status(row, lock.Drifted, false)
			before = testutil.SnapshotTree(t, root)
			if err := l.RemoveOwned(root, row, roots); err == nil {
				t.Fatal("removed drift")
			}
			testutil.AssertTree(t, root, before)
			preserved()
			writeTestFile(t, path, managed)
			if err := l.RemoveOwned(root, row, roots); err != nil {
				t.Fatal(err)
			}
			persist()
			preserved()
			if len(l.Items()) != 0 {
				t.Fatal("removal retained ownership")
			}
			if bytes.Contains(readTestFile(t, path), []byte(`"docs"`)) {
				t.Fatal("removal retained managed entry")
			}
			// A different handwritten object at the desired name is a collision.
			collision, err := lock.SetJSON(readTestFile(t, path), row, []byte(`{"command":"handwritten","future":true}`))
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, path, collision)
			status(row, lock.Collision, false)
			before = testutil.SnapshotTree(t, root)
			if err := l.RemoveOwned(root, row, roots); err == nil {
				t.Fatal("removed unowned collision")
			}
			testutil.AssertTree(t, root, before)
			testutil.AssertNoSecret(t, root, sentinel, string(item.Value))
		})
	}
}

func TestPiMCPOwnershipErrors(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"malformed", `{"mcpServers":`},
		{"duplicate", `{"mcpServers":{},"mcpServers":{}}`},
		{"wrong-type", `{"mcpServers":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := testutil.NewFixture(t, false)
			sentinel := testutil.PlantSentinel(t, f)
			i := lock.Item{Target: contract.PiMCPJSON, Entry: "mcp:docs", Kind: lock.JSONEntry, Path: ".pi/mcp.json", Key: "mcpServers.docs"}
			hash, err := lock.HashJSON([]byte(`{"command":"echo"}`))
			if err != nil {
				t.Fatal(err)
			}
			i.Hash = hash
			writeTestFile(t, filepath.Join(f.Project, i.Path), []byte(tc.body))
			before := testutil.SnapshotTree(t, f.Project)
			if _, err := lock.New().Status(f.Project, i); err == nil {
				t.Fatal("invalid container accepted")
			}
			if _, err := lock.SetJSON([]byte(tc.body), i, []byte(`{"command":"echo"}`)); err == nil {
				t.Fatal("invalid container changed")
			}
			testutil.AssertTree(t, f.Project, before)
			testutil.AssertNoSecret(t, f.Project, sentinel)
		})
	}
	t.Run("paths", func(t *testing.T) {
		f := testutil.NewFixture(t, false)
		sentinel := testutil.PlantSentinel(t, f)
		external := t.TempDir()
		if err := os.Symlink(external, filepath.Join(f.Project, ".pi")); err != nil {
			t.Fatal(err)
		}
		before := testutil.SnapshotTree(t, f.Project)
		outside := testutil.SnapshotTree(t, external)
		i := lock.Item{Target: contract.PiMCPJSON, Entry: "mcp:docs", Kind: lock.JSONEntry, Path: ".pi/mcp.json", Key: "mcpServers.docs", Hash: lock.HashBytes(nil)}
		if _, err := lock.New().Status(f.Project, i); err == nil {
			t.Fatal("symlink parent accepted")
		}
		for _, path := range []string{".pi/mcp.json", "../escape.json", ".other/mcp.json"} {
			if err := lock.WriteFile(f.Project, path, []string{".pi/mcp.json"}, []byte("{}"), 0600); err == nil {
				t.Fatal("unsafe path written")
			}
		}
		testutil.AssertTree(t, f.Project, before)
		testutil.AssertTree(t, external, outside)
		testutil.AssertNoSecret(t, f.Project, sentinel)
	})
}

func TestPiMCPSymlinkFileRefused(t *testing.T) {
	f := testutil.NewFixture(t, false)
	sentinel := testutil.PlantSentinel(t, f)
	external := filepath.Join(t.TempDir(), "mcp.json")
	writeTestFile(t, external, []byte(`{"mcpServers":{"handwritten":{"future":true}}}`))
	if err := os.MkdirAll(filepath.Join(f.Project, ".pi"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(f.Project, ".pi/mcp.json")); err != nil {
		t.Fatal(err)
	}
	before := testutil.SnapshotTree(t, f.Project)
	original := readTestFile(t, external)
	if err := lock.WriteFile(f.Project, ".pi/mcp.json", []string{".pi/mcp.json"}, []byte("{}"), 0600); err == nil {
		t.Fatal("symlink file overwritten")
	}
	testutil.AssertTree(t, f.Project, before)
	if !bytes.Equal(original, readTestFile(t, external)) {
		t.Fatal("external file changed")
	}
	testutil.AssertNoSecret(t, f.Project, sentinel)
}
