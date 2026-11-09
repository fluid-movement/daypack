package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/testadapter"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/plan"
	"github.com/spf13/cobra"
)

func inlineApplyFixture(t *testing.T) (string, string, *adapters.Registry) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	libraryParent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	libraryDir := filepath.Join(libraryParent, "library")
	if err := library.Init(libraryDir); err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(libraryDir, "skills/testing")
	if err := os.Mkdir(skill, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: testing\ndescription: Test skill\n---\nTest.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	body := "version = 1\nharnesses = [\"claude-code\"]\nskills = [\"testing\"]\n[inline.mcp.docs]\ntype = \"stdio\"\ncommand = \"server\"\nargs = [\"${DAYPACK_TEST_SECRET}\"]\n[inline.mcp.docs.env]\nTOKEN = \"${DAYPACK_TEST_SECRET}\"\n"
	if err := os.WriteFile(filepath.Join(root, "daypack.toml"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	a := &testadapter.Adapter{Harness: "claude-code", Declarations: []contract.Target{
		{ID: "test-skills", Root: ".test/skills", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Skill}},
		{ID: "test-mcp", Root: ".mcp.json", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.MCP}},
		{ID: "test-approval", Root: ".test/settings.json", Mode: contract.FileRoot, Types: []contract.ArtifactType{library.MCP}},
	}}
	a.Render = func(entry contract.Entry, _ contract.Project) ([]contract.Item, error) {
		if entry.Artifact.Type == library.Skill {
			return []contract.Item{{Target: "test-skills", Entry: entry.Ref, Kind: contract.Symlink, Path: ".test/skills/testing", Link: entry.Artifact.Path}}, nil
		}
		definition := entry.Artifact.MCP
		value, err := json.Marshal(map[string]any{"command": definition.Command, "args": definition.Args, "env": definition.Env})
		if err != nil {
			return nil, err
		}
		return []contract.Item{
			{Target: "test-mcp", Entry: entry.Ref, Kind: contract.JSONEntry, Path: ".mcp.json", Key: "mcpServers.docs", Value: value},
			{Target: "test-approval", Entry: entry.Ref, Kind: contract.JSONArrayElement, Path: ".test/settings.json", Key: "approved", Value: []byte(`"docs"`)},
		}, nil
	}
	registry, err := adapters.NewRegistry(a)
	if err != nil {
		t.Fatal(err)
	}
	return root, libraryDir, registry
}

func runInlineApply(root, libraryDir string, registry *adapters.Registry, args []string, input io.Reader, terminal bool) (int, string, string) {
	var stdout, stderr bytes.Buffer
	streams := IO{In: input, Out: &stdout, Err: &stderr, IsTerminal: func(io.Reader) bool { return terminal }}
	argv := append([]string{"--project", root, "--library", libraryDir, "apply"}, args...)
	code := run(context.Background(), argv, streams, func(command *cobra.Command) {
		for _, existing := range command.Commands() {
			if existing.Name() == "apply" {
				command.RemoveCommand(existing)
			}
		}
		command.AddCommand(applyCommandWithRegistry(streams, func() (*adapters.Registry, error) { return registry, nil }))
	})
	return code, stdout.String(), stderr.String()
}

func TestApplyInlineConsentDispatch(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		args           []string
		terminal       bool
		approve, dry   bool
		prompt         bool
	}{
		{name: "TTY yes", response: "yes\n", terminal: true, approve: true, prompt: true},
		{name: "TTY no", response: "no\n", terminal: true, prompt: true},
		{name: "TTY EOF", terminal: true, prompt: true},
		{name: "non TTY"},
		{name: "JSON overrides TTY", args: []string{"--json"}, terminal: true},
		{name: "yes flag", args: []string{"--yes", "--json"}, approve: true},
		{name: "dry run", args: []string{"--dry-run"}, terminal: true, dry: true},
		{name: "JSON dry run", args: []string{"--dry-run", "--yes", "--json"}, terminal: true, dry: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project, libraryDir, registry := inlineApplyFixture(t)
			const sentinel = "DAYPACK_SYNTHETIC_SECRET_2651_DO_NOT_COPY"
			t.Setenv("DAYPACK_TEST_SECRET", sentinel)
			if err := os.WriteFile(filepath.Join(libraryDir, ".env"), []byte("DAYPACK_TEST_SECRET="+sentinel+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			var input io.Reader = unreadableInput{t}
			if tc.prompt {
				input = strings.NewReader(tc.response)
			}
			code, stdout, stderr := runInlineApply(project, libraryDir, registry, tc.args, input, tc.terminal)
			wantCode := Attention
			if tc.approve {
				wantCode = Success
			}
			if code != wantCode {
				t.Fatalf("code %d, want %d: %s; %s", code, wantCode, stdout, stderr)
			}
			if strings.Contains(stderr, "[y/N]") != tc.prompt {
				t.Fatalf("unexpected prompt: %s", stderr)
			}
			if strings.Contains(stdout+stderr, sentinel) {
				t.Fatal("secret appeared in output")
			}
			owned, err := lock.Load(project)
			if err != nil {
				t.Fatal(err)
			}
			if tc.dry {
				for _, path := range []string{".daypack", ".test", ".mcp.json"} {
					if _, err := os.Lstat(filepath.Join(project, path)); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("dry-run wrote %s: %v", path, err)
					}
				}
			} else {
				if _, err := os.Readlink(filepath.Join(project, ".test/skills/testing")); err != nil {
					t.Fatal("non-gated skill was not applied: ", err)
				}
				for _, path := range []string{".mcp.json", ".test/settings.json"} {
					data, err := os.ReadFile(filepath.Join(project, path))
					if tc.approve {
						if err != nil || !strings.Contains(string(data), "docs") || strings.Contains(string(data), sentinel) {
							t.Fatalf("invalid approved output %s: %v", path, err)
						}
					} else if !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("declined server wrote %s: %v", path, err)
					}
				}
				for _, item := range owned.Items() {
					if item.Entry == "mcp:docs" && (!tc.approve || item.MCPIdentity == "") {
						t.Fatal("invalid executable consent history")
					}
				}
			}
			if strings.Contains(strings.Join(tc.args, " "), "--json") {
				value := object(t, stdout, code, "daypack apply")
				rows := value["operations"].([]any)
				for _, row := range rows {
					op := row.(map[string]any)
					if op["entry"] == "mcp:docs" && op["gated"] != true {
						t.Fatal("report lost original gate")
					}
				}
			}
			if tc.approve {
				code, stdout, stderr = runInlineApply(project, libraryDir, registry, []string{"--json"}, unreadableInput{t}, true)
				if code != Success || stderr != "" {
					t.Fatalf("verified unchanged server regated: %s; %s", stdout, stderr)
				}
			}
		})
	}
}

func TestApplyResultExitCodes(t *testing.T) {
	for _, tc := range []struct {
		action  plan.Action
		outcome plan.Outcome
		dry     bool
		want    int
	}{
		{plan.Create, plan.Planned, true, Attention},
		{plan.Adopt, plan.Planned, true, Attention},
		{plan.Update, plan.Unchanged, true, Success},
		{plan.Create, plan.Applied, false, Success},
		{plan.Update, plan.Skipped, false, Attention},
		{plan.Remove, plan.Failed, false, Error},
		{plan.Unsupported, plan.Skipped, true, Attention},
	} {
		p := plan.Plan{Operations: []plan.Operation{{Action: tc.action, Outcome: tc.outcome}}}
		if got := applyExitCode(p, tc.dry); got != tc.want {
			t.Fatalf("%s/%s dry=%v: code %d, want %d", tc.action, tc.outcome, tc.dry, got, tc.want)
		}
	}
}
