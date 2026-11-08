package cli_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/cli"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/testutil"
)

const (
	alphaSource = "---\ndescription: Alpha\n---\nAlpha rules.\n"
	betaSource  = "Beta rules.\n"
)

// fragmentFixture lays out a library with the fixture's "conventions" plus
// "alpha" (sorts before it) and "beta", so manifest order differs from lexical.
func fragmentFixture(t *testing.T, worktree bool, harnesses string, fragments string) testutil.Fixture {
	t.Helper()
	f := testutil.NewFixture(t, worktree)
	if worktree {
		f.Project = f.Worktree
	}
	fragWriteSource(t, f, "alpha", alphaSource)
	fragWriteSource(t, f, "beta", betaSource)
	setFragments(t, f, harnesses, fragments)
	return f
}

func fragWriteSource(t *testing.T, f testutil.Fixture, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.Library, "fragments", name+".md"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func setFragments(t *testing.T, f testutil.Fixture, harnesses, fragments string) {
	t.Helper()
	writeApplyManifest(t, f.Project, "version = 1\nharnesses = "+harnesses+"\nfragments = "+fragments+"\n")
}

func fragRead(t *testing.T, root, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func fragWrite(t *testing.T, root, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func fragExists(root, name string) bool {
	_, err := os.Lstat(filepath.Join(root, name))
	return err == nil
}

func fragSection(name, body string) string {
	return "<!-- daypack:begin " + name + " -->\n" + body + "\n<!-- daypack:end " + name + " -->\n"
}

const bothHarnesses = `["claude-code", "pi"]`

func fragApply(t *testing.T, f testutil.Fixture, code int, args ...string) []any {
	t.Helper()
	result := testutil.RunCLI(t, f, append([]string{"apply", "--json"}, args...)...)
	return applyOperations(t, applyEnvelope(t, result, code))
}

func fragLockRows(t *testing.T, root string) map[string]lock.Item {
	t.Helper()
	l, err := lock.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]lock.Item{}
	for _, i := range l.Items() {
		rows[i.Path+"#"+i.Marker] = i
	}
	return rows
}

func fragExclude(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(applyExclude(t, root))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

func TestFragmentsCreateAndConverge(t *testing.T) {
	for _, worktree := range []bool{false, true} {
		name := "project"
		if worktree {
			name = "worktree"
		}
		t.Run(name, func(t *testing.T) {
			f := fragmentFixture(t, worktree, bothHarnesses, `["conventions", "alpha"]`)
			sentinel := testutil.PlantSentinel(t, f)
			ops := fragApply(t, f, cli.Success)
			if len(ops) != 3 {
				t.Fatalf("expected two sections and one bridge: %v", ops)
			}
			agents := fragRead(t, f.Project, "AGENTS.md")
			testutil.AssertGolden(t, filepath.Join("testdata", "fragments", "agents-create.golden"), []byte(agents))
			// Manifest order, not lexical order.
			if strings.Index(agents, "fragment:conventions") > strings.Index(agents, "fragment:alpha") {
				t.Fatal("sections not in manifest order")
			}
			testutil.AssertGolden(t, filepath.Join("testdata", "fragments", "claude-stub.golden"), []byte(fragRead(t, f.Project, "CLAUDE.md")))
			rows := fragLockRows(t, f.Project)
			if len(rows) != 3 {
				t.Fatalf("rows: %v", rows)
			}
			for _, entry := range []struct{ marker, body string }{{"fragment:conventions", "\nKeep tests offline.\n"}, {"fragment:alpha", "\nAlpha rules.\n"}} {
				row, ok := rows["AGENTS.md#"+entry.marker]
				if !ok || row.Kind != lock.MDSection || row.Target != contract.AgentsMD || row.Entry != entry.marker || row.Hash != lock.HashBytes([]byte(entry.body)) || !row.Tracked {
					t.Fatalf("bad row for %s: %+v", entry.marker, row)
				}
			}
			stub := rows["CLAUDE.md#"]
			if stub.Kind != lock.File || stub.Target != contract.ClaudeMD || stub.Entry != contract.BridgeEntry || stub.Hash != lock.HashBytes([]byte("@AGENTS.md\n")) || stub.Tracked {
				t.Fatalf("bad stub row: %+v", stub)
			}
			// Committed exception: AGENTS.md is never excluded, even before git add;
			// a newly owned CLAUDE.md stub follows the normal exclusion policy.
			exclude := fragExclude(t, f.Project)
			if strings.Contains(exclude, "/AGENTS.md") || !strings.Contains(exclude, "/CLAUDE.md\n") || !strings.Contains(exclude, "/.daypack/\n") {
				t.Fatalf("unexpected exclude block:\n%s", exclude)
			}
			// Second apply converges: no byte or mtime change anywhere.
			before := testutil.SnapshotTree(t, f.Project)
			paths := []string{filepath.Join(f.Project, "AGENTS.md"), filepath.Join(f.Project, "CLAUDE.md"), filepath.Join(f.Project, ".daypack/lock.toml"), applyExclude(t, f.Project)}
			metadata := applyMetadata(t, paths)
			fragApply(t, f, cli.Success)
			testutil.AssertTree(t, f.Project, before)
			if !reflect.DeepEqual(metadata, applyMetadata(t, paths)) {
				t.Fatal("second apply changed mtimes")
			}
			testutil.AssertNoSecret(t, f.Project, sentinel, agents, exclude)
		})
	}
}

func TestFragmentsNothingListedCreatesNothing(t *testing.T) {
	f := fragmentFixture(t, false, bothHarnesses, `[]`)
	fragApply(t, f, cli.Success)
	if fragExists(f.Project, "AGENTS.md") || fragExists(f.Project, "CLAUDE.md") {
		t.Fatal("created instruction files without fragments")
	}
}

func TestFragmentsHarnessSelection(t *testing.T) {
	t.Run("pi only", func(t *testing.T) {
		f := fragmentFixture(t, false, `["pi"]`, `["conventions"]`)
		fragApply(t, f, cli.Success)
		if !fragExists(f.Project, "AGENTS.md") || fragExists(f.Project, "CLAUDE.md") {
			t.Fatal("pi-only must write AGENTS.md and no Claude bridge")
		}
	})
	t.Run("restricted entries", func(t *testing.T) {
		f := fragmentFixture(t, false, bothHarnesses, `[{name = "conventions", harnesses = ["pi"]}]`)
		fragApply(t, f, cli.Success)
		if !fragExists(f.Project, "AGENTS.md") || fragExists(f.Project, "CLAUDE.md") {
			t.Fatal("pi-restricted fragment must not create a bridge")
		}
		setFragments(t, f, bothHarnesses, `[{name = "conventions", harnesses = ["pi"]}, {name = "alpha", harnesses = ["claude-code"]}]`)
		fragApply(t, f, cli.Success)
		if !strings.Contains(fragRead(t, f.Project, "AGENTS.md"), "fragment:alpha") || fragRead(t, f.Project, "CLAUDE.md") != "@AGENTS.md\n" {
			t.Fatal("claude-restricted fragment must render once and add the bridge")
		}
	})
	t.Run("harness change removes bridge", func(t *testing.T) {
		f := fragmentFixture(t, false, bothHarnesses, `["conventions"]`)
		fragApply(t, f, cli.Success)
		setFragments(t, f, `["pi"]`, `["conventions"]`)
		fragApply(t, f, cli.Success)
		if fragExists(f.Project, "CLAUDE.md") || !strings.Contains(fragRead(t, f.Project, "AGENTS.md"), "fragment:conventions") {
			t.Fatal("bridge must go, AGENTS.md must stay")
		}
	})
	t.Run("claude only", func(t *testing.T) {
		f := fragmentFixture(t, false, `["claude-code"]`, `["conventions"]`)
		fragApply(t, f, cli.Success)
		if !fragExists(f.Project, "AGENTS.md") || fragRead(t, f.Project, "CLAUDE.md") != "@AGENTS.md\n" {
			t.Fatal("claude-only needs AGENTS.md and the bridge")
		}
	})
}

func TestFragmentsHandwrittenClaudeAndAgents(t *testing.T) {
	f := fragmentFixture(t, false, bothHarnesses, `["conventions", "alpha"]`)
	fragWrite(t, f.Project, "CLAUDE.md", "# Mine\n@AGENTS.md\nnotes\n")
	fragWrite(t, f.Project, "AGENTS.md", "intro\n\nmiddle\n")
	fragApply(t, f, cli.Success)
	testutil.AssertGolden(t, filepath.Join("testdata", "fragments", "claude-handwritten.golden"), []byte(fragRead(t, f.Project, "CLAUDE.md")))
	agents := fragRead(t, f.Project, "AGENTS.md")
	if !strings.HasPrefix(agents, "intro\n\nmiddle\n") {
		t.Fatalf("handwritten prefix changed: %q", agents)
	}
	rows := fragLockRows(t, f.Project)
	bridge := rows["CLAUDE.md#agents-md-import"]
	if bridge.Kind != lock.MDSection || bridge.Entry != contract.BridgeEntry || !bridge.Tracked || len(rows) != 3 {
		t.Fatalf("bad bridge row: %+v (%d rows)", bridge, len(rows))
	}
	exclude := fragExclude(t, f.Project)
	if strings.Contains(exclude, "/CLAUDE.md") || strings.Contains(exclude, "/AGENTS.md") {
		t.Fatalf("committed sections must not be excluded:\n%s", exclude)
	}
	// Removing every fragment restores both handwritten files byte for byte.
	setFragments(t, f, bothHarnesses, `[]`)
	fragApply(t, f, cli.Success)
	if fragRead(t, f.Project, "CLAUDE.md") != "# Mine\n@AGENTS.md\nnotes\n" || fragRead(t, f.Project, "AGENTS.md") != "intro\n\nmiddle\n" {
		t.Fatalf("handwritten bytes changed after removal:\n%q\n%q", fragRead(t, f.Project, "CLAUDE.md"), fragRead(t, f.Project, "AGENTS.md"))
	}
	if len(fragLockRows(t, f.Project)) != 0 {
		t.Fatal("lock rows remain")
	}
}

func TestFragmentsUpdateReorderRemove(t *testing.T) {
	f := fragmentFixture(t, false, bothHarnesses, `["conventions", "alpha", "beta"]`)
	fragApply(t, f, cli.Success)
	original := fragRead(t, f.Project, "AGENTS.md")
	// Hand-written prose between sections is never touched.
	edited := strings.Replace(original, "<!-- daypack:begin fragment:alpha -->", "between one\n<!-- daypack:begin fragment:alpha -->", 1)
	edited = strings.Replace(edited, "<!-- daypack:begin fragment:beta -->", "between two\n<!-- daypack:begin fragment:beta -->", 1)
	fragWrite(t, f.Project, "AGENTS.md", "top\n"+edited+"bottom")
	paths := []string{filepath.Join(f.Project, "AGENTS.md"), filepath.Join(f.Project, "CLAUDE.md")}

	t.Run("source update", func(t *testing.T) {
		fragWriteSource(t, f, "alpha", "---\ndescription: Alpha\n---\nAlpha changed.\n")
		ops := fragApply(t, f, cli.Success)
		if len(ops) != 1 || ops[0].(map[string]any)["action"] != "update" || ops[0].(map[string]any)["section"] != "fragment:alpha" {
			t.Fatalf("ops: %v", ops)
		}
		got := fragRead(t, f.Project, "AGENTS.md")
		if !strings.Contains(got, "Alpha changed.") || !strings.Contains(got, "top\n") || !strings.HasSuffix(got, "bottom") || !strings.Contains(got, "between one\n") {
			t.Fatalf("update broke handwritten text: %q", got)
		}
		if fragLockRows(t, f.Project)["AGENTS.md#fragment:alpha"].Hash != lock.HashBytes([]byte("\nAlpha changed.\n")) {
			t.Fatal("hash not updated")
		}
	})
	t.Run("reorder alone", func(t *testing.T) {
		setFragments(t, f, bothHarnesses, `["beta", "alpha", "conventions"]`)
		before := testutil.SnapshotTree(t, f.Project)
		dry := testutil.RunCLI(t, f, "apply", "--dry-run", "--json")
		ops := applyOperations(t, applyEnvelope(t, dry, cli.Attention))
		if len(ops) == 0 {
			t.Fatal("reorder was not reported")
		}
		for _, op := range ops {
			row := op.(map[string]any)
			if row["action"] != "update" || row["reason"] != "reorder" || row["outcome"] != "planned" {
				t.Fatalf("not a reorder update: %v", row)
			}
		}
		testutil.AssertTree(t, f.Project, before)
		fragApply(t, f, cli.Success)
		got := fragRead(t, f.Project, "AGENTS.md")
		testutil.AssertGolden(t, filepath.Join("testdata", "fragments", "agents-reordered.golden"), []byte(got))
		order := []int{strings.Index(got, "fragment:beta"), strings.Index(got, "fragment:alpha"), strings.Index(got, "fragment:conventions")}
		if order[0] >= order[1] || order[1] >= order[2] || !strings.HasPrefix(got, "top\n") || !strings.HasSuffix(got, "bottom") {
			t.Fatalf("not reordered or handwritten text lost: %q", got)
		}
		// Written once: a repeat apply changes neither bytes nor mtimes.
		snapshot := testutil.SnapshotTree(t, f.Project)
		metadata := applyMetadata(t, paths)
		fragApply(t, f, cli.Success)
		testutil.AssertTree(t, f.Project, snapshot)
		if !reflect.DeepEqual(metadata, applyMetadata(t, paths)) {
			t.Fatal("repeat apply rewrote files")
		}
	})
	t.Run("remove first keeps bridge", func(t *testing.T) {
		setFragments(t, f, bothHarnesses, `["alpha", "conventions"]`)
		ops := fragApply(t, f, cli.Success)
		got := fragRead(t, f.Project, "AGENTS.md")
		if strings.Contains(got, "fragment:beta") || strings.Contains(got, "Beta rules") || !strings.Contains(got, "between two\n") {
			t.Fatalf("beta not cleanly removed: %q", got)
		}
		for _, op := range ops {
			if op.(map[string]any)["entry"] == contract.BridgeEntry {
				t.Fatalf("bridge ownership changed: %v", op)
			}
		}
		if fragRead(t, f.Project, "CLAUDE.md") != "@AGENTS.md\n" {
			t.Fatal("bridge lost")
		}
	})
	t.Run("remove all", func(t *testing.T) {
		setFragments(t, f, bothHarnesses, `[]`)
		fragApply(t, f, cli.Success)
		got := fragRead(t, f.Project, "AGENTS.md")
		if strings.Contains(got, "daypack:") || !strings.HasPrefix(got, "top\n") || !strings.HasSuffix(got, "bottom") || !strings.Contains(got, "between one\n") {
			t.Fatalf("markers remain or handwritten text lost: %q", got)
		}
		if fragExists(f.Project, "CLAUDE.md") {
			t.Fatal("clean owned stub must be deleted")
		}
		if len(fragLockRows(t, f.Project)) != 0 {
			t.Fatal("lock rows remain")
		}
	})
}

func TestFragmentsFirstFragmentRemovedBridgeStable(t *testing.T) {
	f := fragmentFixture(t, false, bothHarnesses, `["conventions", "alpha"]`)
	fragApply(t, f, cli.Success)
	stub := applyMetadata(t, []string{filepath.Join(f.Project, "CLAUDE.md")})
	setFragments(t, f, bothHarnesses, `["alpha"]`)
	fragApply(t, f, cli.Success)
	if !reflect.DeepEqual(stub, applyMetadata(t, []string{filepath.Join(f.Project, "CLAUDE.md")})) || len(fragLockRows(t, f.Project)) != 2 {
		t.Fatal("bridge ownership moved when the first fragment was removed")
	}
}

func TestFragmentsRemoveAllKeepsEditedStubAndAgents(t *testing.T) {
	f := fragmentFixture(t, false, bothHarnesses, `["conventions"]`)
	fragApply(t, f, cli.Success)
	fragWrite(t, f.Project, "CLAUDE.md", "@AGENTS.md\nmine\n")
	setFragments(t, f, bothHarnesses, `[]`)
	fragApply(t, f, cli.Attention)
	// The edited stub is kept; the sections in the other file are still removed.
	if fragRead(t, f.Project, "CLAUDE.md") != "@AGENTS.md\nmine\n" || strings.Contains(fragRead(t, f.Project, "AGENTS.md"), "daypack:") {
		t.Fatal("edited stub must be kept while sections are removed")
	}
	fragApply(t, f, cli.Success, "--force")
	if fragExists(f.Project, "CLAUDE.md") {
		t.Fatal("forced removal should delete the owned stub")
	}
	if got := fragRead(t, f.Project, "AGENTS.md"); got != "" {
		t.Fatalf("AGENTS.md created by daypack should be left empty, got %q", got)
	}
}

func TestFragmentsDriftAndCollision(t *testing.T) {
	t.Run("drift without and with force", func(t *testing.T) {
		f := fragmentFixture(t, false, `["pi"]`, `["conventions", "alpha"]`)
		fragApply(t, f, cli.Success)
		drifted := strings.Replace(fragRead(t, f.Project, "AGENTS.md"), "Keep tests offline.", "Edited by hand.", 1)
		fragWrite(t, f.Project, "AGENTS.md", drifted)
		// A pending change to another section in the same file is held back too.
		fragWriteSource(t, f, "alpha", "---\ndescription: Alpha\n---\nAlpha changed.\n")
		before := testutil.SnapshotTree(t, f.Project)
		ops := fragApply(t, f, cli.Attention)
		if len(ops) == 0 {
			t.Fatal("drift not reported")
		}
		testutil.AssertTree(t, f.Project, before)
		fragApply(t, f, cli.Success, "--force")
		got := fragRead(t, f.Project, "AGENTS.md")
		if !strings.Contains(got, "Keep tests offline.") || strings.Contains(got, "Edited by hand.") || !strings.Contains(got, "Alpha changed.") {
			t.Fatalf("force did not restore: %q", got)
		}
	})
	t.Run("collision with and without adopt", func(t *testing.T) {
		f := fragmentFixture(t, false, `["pi"]`, `["conventions"]`)
		fragWrite(t, f.Project, "AGENTS.md", "top\n"+fragSection("fragment:conventions", "Hand written conflicting text.")+"bottom\n")
		before := testutil.SnapshotTree(t, f.Project)
		fragApply(t, f, cli.Attention)
		testutil.AssertTree(t, f.Project, before)
		fragApply(t, f, cli.Success, "--adopt")
		got := fragRead(t, f.Project, "AGENTS.md")
		if !strings.HasPrefix(got, "top\n") || !strings.HasSuffix(got, "bottom\n") || !strings.Contains(got, "Keep tests offline.") || strings.Contains(got, "conflicting") {
			t.Fatalf("adopt result: %q", got)
		}
	})
	t.Run("adopt converged section", func(t *testing.T) {
		f := fragmentFixture(t, false, `["pi"]`, `["conventions"]`)
		fragWrite(t, f.Project, "AGENTS.md", fragSection("fragment:conventions", "Keep tests offline."))
		before := fragRead(t, f.Project, "AGENTS.md")
		fragApply(t, f, cli.Success, "--adopt")
		if fragRead(t, f.Project, "AGENTS.md") != before || len(fragLockRows(t, f.Project)) != 1 {
			t.Fatal("adopting identical content must only record ownership")
		}
	})
}

func TestFragmentsUnresolvedPreservesPreviousState(t *testing.T) {
	f := fragmentFixture(t, false, bothHarnesses, `["conventions", "alpha"]`)
	fragApply(t, f, cli.Success)
	if err := os.Remove(filepath.Join(f.Library, "fragments", "conventions.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.Library, "fragments", "alpha.md")); err != nil {
		t.Fatal(err)
	}
	before := testutil.SnapshotTree(t, f.Project)
	ops := fragApply(t, f, cli.Attention)
	for _, op := range ops {
		if op.(map[string]any)["action"] == "remove" {
			t.Fatalf("unresolved fragments must keep their sections: %v", ops)
		}
	}
	testutil.AssertTree(t, f.Project, before)
}

func TestFragmentsMalformedExistingMarkersRefuse(t *testing.T) {
	for name, body := range map[string]string{
		"unclosed":  "intro\n<!-- daypack:begin fragment:conventions -->\ntext\n",
		"unmatched": "intro\n<!-- daypack:end fragment:conventions -->\n",
		"duplicate": fragSection("fragment:x", "a") + fragSection("fragment:x", "b"),
		"nested":    "<!-- daypack:begin a -->\n<!-- daypack:begin b -->\n<!-- daypack:end b -->\n<!-- daypack:end a -->\n",
	} {
		t.Run(name, func(t *testing.T) {
			f := fragmentFixture(t, false, bothHarnesses, `["conventions"]`)
			fragWrite(t, f.Project, "AGENTS.md", body)
			before := testutil.SnapshotTree(t, f.Project)
			result := testutil.RunCLI(t, f, "apply", "--json")
			if result.Code == cli.Success {
				t.Fatalf("accepted malformed markers: %s", result.Stdout)
			}
			testutil.AssertTree(t, f.Project, before)
		})
	}
}

func TestFragmentsMalformedSourcesWriteNothing(t *testing.T) {
	for name, source := range map[string]string{
		"unterminated frontmatter": "---\ndescription: x\nbody without end\n",
		"invalid frontmatter":      "---\n: : :\n  - [\n---\nbody\n",
		"embedded marker":          "text <!-- daypack:end fragment:conventions --> more\n",
	} {
		t.Run(name, func(t *testing.T) {
			f := fragmentFixture(t, false, bothHarnesses, `["conventions"]`)
			fragWriteSource(t, f, "conventions", source)
			before := testutil.SnapshotTree(t, f.Project)
			result := testutil.RunCLI(t, f, "apply", "--json")
			if result.Code == cli.Success {
				t.Fatalf("accepted bad fragment: %s", result.Stdout)
			}
			testutil.AssertTree(t, f.Project, before)
		})
	}
}

func TestFragmentsFramingEdgeCases(t *testing.T) {
	cases := map[string]string{"empty": "", "plain": "Plain text.\n", "crlf": "line one\r\nline two\r\n", "no-final-newline": "no newline"}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			f := fragmentFixture(t, false, `["pi"]`, `["conventions"]`)
			fragWriteSource(t, f, "conventions", source)
			fragApply(t, f, cli.Success)
			got := fragRead(t, f.Project, "AGENTS.md")
			want := "<!-- daypack:begin fragment:conventions -->\n" + source
			if source != "" && !strings.HasSuffix(source, "\n") {
				want += "\n"
			}
			want += "<!-- daypack:end fragment:conventions -->\n"
			if got != want {
				t.Fatalf("got %q want %q", got, want)
			}
			fragApply(t, f, cli.Success)
		})
	}
}

func TestFragmentsSymlinkRefused(t *testing.T) {
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		t.Run(name, func(t *testing.T) {
			f := fragmentFixture(t, false, bothHarnesses, `["conventions"]`)
			outside := filepath.Join(t.TempDir(), "outside.md")
			if err := os.WriteFile(outside, []byte("outside\n"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(f.Project, name)); err != nil {
				t.Fatal(err)
			}
			before := testutil.SnapshotTree(t, f.Project)
			result := testutil.RunCLI(t, f, "apply", "--json")
			if result.Code == cli.Success {
				t.Fatalf("followed symlink: %s", result.Stdout)
			}
			testutil.AssertTree(t, f.Project, before)
			if b, err := os.ReadFile(outside); err != nil || string(b) != "outside\n" {
				t.Fatal("symlink target modified")
			}
		})
	}
}

func TestFragmentsDryRunReports(t *testing.T) {
	f := fragmentFixture(t, false, bothHarnesses, `["conventions", "alpha"]`)
	excludeBefore := fragExclude(t, f.Project)
	before := testutil.SnapshotTree(t, f.Project)
	result := testutil.RunCLI(t, f, "apply", "--dry-run", "--json")
	ops := applyOperations(t, applyEnvelope(t, result, cli.Attention))
	if len(ops) != 3 {
		t.Fatalf("ops: %v", ops)
	}
	testutil.AssertTree(t, f.Project, before)
	if fragExclude(t, f.Project) != excludeBefore || fragExists(f.Project, ".daypack/lock.toml") {
		t.Fatal("dry run wrote lock or exclude")
	}
	testutil.AssertGolden(t, filepath.Join("testdata", "fragments", "dry-run-create.golden"), testutil.NormalizeFixturePaths([]byte(result.Stdout), f.Project))
	// Handwritten CLAUDE.md: the import is reported as a section, not a file.
	fragWrite(t, f.Project, "CLAUDE.md", "mine\n")
	before = testutil.SnapshotTree(t, f.Project)
	result = testutil.RunCLI(t, f, "apply", "--dry-run", "--json")
	applyOperations(t, applyEnvelope(t, result, cli.Attention))
	if !strings.Contains(result.Stdout, `"section":"agents-md-import"`) {
		t.Fatalf("import section not reported: %s", result.Stdout)
	}
	testutil.AssertTree(t, f.Project, before)
	// Removal is reported without writes.
	fragWrite(t, f.Project, "CLAUDE.md", "mine\n")
	fragApply(t, f, cli.Success)
	setFragments(t, f, bothHarnesses, `["alpha"]`)
	before = testutil.SnapshotTree(t, f.Project)
	result = testutil.RunCLI(t, f, "apply", "--dry-run", "--json")
	applyOperations(t, applyEnvelope(t, result, cli.Attention))
	if !strings.Contains(result.Stdout, `"action":"remove"`) || !strings.Contains(result.Stdout, `fragment:conventions`) {
		t.Fatalf("removal not reported: %s", result.Stdout)
	}
	testutil.AssertTree(t, f.Project, before)
}
