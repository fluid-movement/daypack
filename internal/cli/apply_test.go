package cli_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/cli"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/plan"
	"github.com/fluid-movement/daypack/internal/testutil"
)

const supportedApplyManifest = "version = 1\nharnesses = [\"claude-code\"]\nskills = [\"testing\"]\nagents = [\"reviewer\"]\n"

func writeApplyManifest(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "daypack.toml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func applyEnvelope(t *testing.T, result testutil.CLIResult, code int) map[string]any {
	t.Helper()
	if result.Code != code {
		t.Fatalf("exit = %d, want %d; stdout: %s; stderr: %s", result.Code, code, result.Stdout, result.Stderr)
	}
	decoder := json.NewDecoder(strings.NewReader(result.Stdout))
	var envelope map[string]any
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("expected exactly one JSON envelope: %v", err)
	}
	if !strings.HasSuffix(result.Stdout, "\n") || envelope["command"] != "daypack apply" || envelope["ok"] != (code == cli.Success) {
		t.Fatalf("incorrect envelope: %s", result.Stdout)
	}
	return envelope
}

func applyOperations(t *testing.T, envelope map[string]any) []any {
	t.Helper()
	for _, field := range []string{"operations", "summary", "outcomes", "notes"} {
		if envelope[field] == nil {
			t.Fatalf("missing report field %q", field)
		}
	}
	operations, ok := envelope["operations"].([]any)
	if !ok {
		t.Fatal("operations is not an array")
	}
	return operations
}

func applyExclude(t *testing.T, root string) string {
	t.Helper()
	data, err := exec.Command("git", "-C", root, "rev-parse", "--git-path", "info/exclude").Output()
	if err != nil {
		t.Fatal(err)
	}
	path := strings.TrimSpace(string(data))
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	return path
}

type applyFileState struct {
	Mode os.FileMode
	Time int64
}

func applyMetadata(t *testing.T, paths []string) map[string]applyFileState {
	t.Helper()
	states := make(map[string]applyFileState, len(paths))
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		states[path] = applyFileState{Mode: info.Mode(), Time: info.ModTime().UnixNano()}
	}
	return states
}

func TestApplyTwiceNoOp(t *testing.T) {
	for _, worktree := range []bool{false, true} {
		name := "project"
		if worktree {
			name = "worktree"
		}
		t.Run(name, func(t *testing.T) {
			f := testutil.NewFixture(t, worktree)
			if worktree {
				f.Project = f.Worktree
			}
			writeApplyManifest(t, f.Project, supportedApplyManifest)
			sentinel := testutil.PlantSentinel(t, f)
			libraryBefore := testutil.SnapshotTree(t, f.Library)
			homeBefore := testutil.SnapshotTree(t, f.Home)
			first := testutil.RunCLI(t, f, "apply", "--json")
			operations := applyOperations(t, applyEnvelope(t, first, cli.Success))
			if len(operations) != 2 {
				t.Fatalf("expected skill and agent: %s", first.Stdout)
			}
			for _, row := range operations {
				if row.(map[string]any)["outcome"] != string(plan.Applied) {
					t.Fatalf("operation was not applied: %v", row)
				}
			}
			link, err := os.Readlink(filepath.Join(f.Project, ".claude/skills/testing"))
			if err != nil || link != filepath.Join(f.Library, "skills/testing") {
				t.Fatalf("skill link = %q: %v", link, err)
			}
			agent, err := os.ReadFile(filepath.Join(f.Project, ".claude/agents/reviewer.md"))
			if err != nil || !strings.Contains(string(agent), "Review carefully.") {
				t.Fatalf("agent not copied: %v", err)
			}
			owned, err := lock.Load(f.Project)
			if err != nil {
				t.Fatal(err)
			}
			block, err := lock.ExcludeBlock(f.Project, owned.Items())
			if err != nil {
				t.Fatal(err)
			}
			excludePath := applyExclude(t, f.Project)
			exclude, err := os.ReadFile(excludePath)
			if err != nil || !strings.Contains(string(exclude), block) {
				t.Fatalf("missing exact exclusion block: %v", err)
			}
			before := testutil.SnapshotTree(t, f.Project)
			paths := []string{f.Project, filepath.Join(f.Project, ".claude"), filepath.Join(f.Project, ".claude/skills"), filepath.Join(f.Project, ".claude/agents"), filepath.Join(f.Project, ".claude/skills/testing"), filepath.Join(f.Project, ".claude/agents/reviewer.md"), filepath.Join(f.Project, ".daypack"), filepath.Join(f.Project, ".daypack/lock.toml"), excludePath}
			metadata := applyMetadata(t, paths)
			second := testutil.RunCLI(t, f, "apply", "--json")
			applyOperations(t, applyEnvelope(t, second, cli.Success))
			testutil.AssertTree(t, f.Project, before)
			if !reflect.DeepEqual(metadata, applyMetadata(t, paths)) {
				t.Fatal("second apply changed modes or mtimes")
			}
			excludeAfter, err := os.ReadFile(excludePath)
			if err != nil || string(excludeAfter) != string(exclude) {
				t.Fatal("second apply changed worktree exclude")
			}
			testutil.AssertTree(t, f.Library, libraryBefore)
			testutil.AssertTree(t, f.Home, homeBefore)
			testutil.AssertNoSecret(t, f.Project, sentinel, first.Stdout, first.Stderr, second.Stdout, second.Stderr)
		})
	}
}

func TestApplyDryRun(t *testing.T) {
	f := testutil.NewFixture(t, false)
	writeApplyManifest(t, f.Project, supportedApplyManifest)
	sentinel := testutil.PlantSentinel(t, f)
	before := testutil.SnapshotTree(t, f.Project)
	libraryBefore := testutil.SnapshotTree(t, f.Library)
	var previous string
	for _, flags := range [][]string{nil, {"--yes"}, {"--force", "--adopt", "--yes"}} {
		args := append([]string{"apply", "--dry-run", "--json"}, flags...)
		result := testutil.RunCLIWithInput(t, f, unreadable{t}, true, args...)
		applyOperations(t, applyEnvelope(t, result, cli.Attention))
		if previous != "" && previous != result.Stdout {
			t.Fatal("preview changed for equivalent flags")
		}
		previous = result.Stdout
		if result.Stderr != "" {
			t.Fatalf("dry-run prompted: %s", result.Stderr)
		}
		testutil.AssertTree(t, f.Project, before)
		testutil.AssertTree(t, f.Library, libraryBefore)
		testutil.AssertNoSecret(t, f.Project, sentinel, result.Stdout, result.Stderr)
	}
	applyEnvelope(t, testutil.RunCLI(t, f, "apply", "--json"), cli.Success)
	before = testutil.SnapshotTree(t, f.Project)
	applyEnvelope(t, testutil.RunCLI(t, f, "apply", "--dry-run", "--json"), cli.Success)
	testutil.AssertTree(t, f.Project, before)
}

func TestApplyCollisionPermissions(t *testing.T) {
	f := testutil.NewFixture(t, false)
	writeApplyManifest(t, f.Project, supportedApplyManifest)
	path := filepath.Join(f.Project, ".claude/agents/reviewer.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("Handwritten agent.\n"), 0640); err != nil {
		t.Fatal(err)
	}
	for _, flags := range [][]string{nil, {"--force"}, {"--yes"}, {"--force", "--yes"}} {
		args := append([]string{"apply", "--json"}, flags...)
		result := testutil.RunCLI(t, f, args...)
		applyOperations(t, applyEnvelope(t, result, cli.Attention))
		if !strings.Contains(result.Stdout, ".claude/agents/reviewer.md") || !strings.Contains(result.Stdout, "conflict") {
			t.Fatal("collision report lost path/reason")
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "Handwritten agent.\n" {
			t.Fatal("unadopted collision was overwritten")
		}
	}
	applyEnvelope(t, testutil.RunCLI(t, f, "apply", "--adopt", "--json"), cli.Success)
	if err := os.WriteFile(path, []byte("Edited managed agent.\n"), 0640); err != nil {
		t.Fatal(err)
	}
	applyEnvelope(t, testutil.RunCLI(t, f, "apply", "--adopt", "--yes", "--json"), cli.Attention)
	applyEnvelope(t, testutil.RunCLI(t, f, "apply", "--force", "--json"), cli.Success)
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "Review carefully.") {
		t.Fatal("authorized force did not restore the managed agent")
	}
}

func TestApplyUnresolvedPreservesManagedContent(t *testing.T) {
	f := testutil.NewFixture(t, false)
	writeApplyManifest(t, f.Project, supportedApplyManifest)
	applyEnvelope(t, testutil.RunCLI(t, f, "apply", "--json"), cli.Success)
	if err := os.Remove(filepath.Join(f.Library, "agents/reviewer.md")); err != nil {
		t.Fatal(err)
	}
	before := testutil.SnapshotTree(t, f.Project)
	result := testutil.RunCLI(t, f, "apply", "--json")
	applyOperations(t, applyEnvelope(t, result, cli.Attention))
	if !strings.Contains(result.Stdout, "unresolved") || !strings.Contains(result.Stdout, "agent:reviewer") {
		t.Fatal("missing unresolved entry report")
	}
	testutil.AssertTree(t, f.Project, before)
}

func TestApplyDispatchAndErrors(t *testing.T) {
	f := testutil.NewFixture(t, false)
	writeApplyManifest(t, f.Project, supportedApplyManifest)
	nested := filepath.Join(f.Project, "nested", "directory")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	nestedFixture := f
	nestedFixture.Project = nested
	applyEnvelope(t, testutil.RunCLI(t, nestedFixture, "apply", "--json"), cli.Success)
	quiet := testutil.RunCLI(t, f, "apply", "--quiet", "--no-color")
	if quiet.Code != cli.Success || quiet.Stdout != "" || quiet.Stderr != "" {
		t.Fatalf("quiet apply: %+v", quiet)
	}
	for _, args := range [][]string{{"apply", "extra", "--json"}, {"apply", "--unknown", "--json"}, {"apply", "--force=invalid", "--json"}} {
		applyEnvelope(t, testutil.RunCLI(t, f, args...), cli.Usage)
	}
	writeApplyManifest(t, f.Project, "version = 1\nharnesses = [\"claude-code\"]\nskills = [\"missing\"]\n")
	applyEnvelope(t, testutil.RunCLI(t, f, "apply", "--json"), cli.Attention)
	writeApplyManifest(t, f.Project, "not valid TOML [")
	before := testutil.SnapshotTree(t, f.Project)
	applyEnvelope(t, testutil.RunCLI(t, f, "apply", "--json"), cli.Error)
	testutil.AssertTree(t, f.Project, before)
	writeApplyManifest(t, f.Project, supportedApplyManifest)
	if err := os.WriteFile(filepath.Join(f.Project, ".daypack/lock.toml"), []byte("invalid ["), 0600); err != nil {
		t.Fatal(err)
	}
	applyEnvelope(t, testutil.RunCLI(t, f, "apply", "--json"), cli.Error)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := cli.Run(ctx, []string{"--project", f.Project, "--library", f.Library, "apply"}, cli.IO{}); code != cli.Error {
		t.Fatalf("canceled invocation returned %d", code)
	}
}
