package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func environment(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("HOME", base)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("DAYPACK_HOME", "")
	return base
}

func invoke(t *testing.T, args []string, terminal bool, configure func(*cobra.Command)) (int, string, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	code := run(context.Background(), args, IO{
		In: strings.NewReader(""), Out: &out, Err: &stderr,
		IsTerminal: func(io.Reader) bool { return terminal },
	}, configure)
	return code, out.String(), stderr.String()
}

func object(t *testing.T, output string, code int, command string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(output))
	var value map[string]any
	if err := dec.Decode(&value); err != nil {
		t.Fatalf("invalid JSON %q: %v", output, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
	if value["ok"] != (code == Success) || value["command"] != command {
		t.Fatalf("unexpected envelope: %#v", value)
	}
	if !strings.HasSuffix(output, "\n") {
		t.Fatal("missing newline")
	}
	return value
}

func TestOutcomes(t *testing.T) {
	environment(t)
	tests := []struct {
		name, command string
		result        Result
		err           error
		code          int
	}{
		{name: "success", command: "status", result: Result{Items: []string{"clean"}}, code: Success},
		{name: "pending status", command: "status", result: Result{Code: Attention, Operations: []string{"update"}}, code: Attention},
		{name: "gated apply", command: "apply", result: Result{Code: Attention, Problems: []string{"gated entry skipped"}}, code: Attention},
		{name: "invalid manifest", command: "apply", err: errors.New("invalid manifest"), code: Error},
		{name: "doctor validation", command: "library doctor", err: &Failure{Code: Error, Cause: errors.New("invalid library\nentry"), Hint: "Repair the entry.\nRetry."}, code: Error},
		{name: "typed usage", command: "apply", err: &Failure{Code: Usage, Cause: errors.New("invalid argument"), Hint: "Use a supported argument."}, code: Usage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := strings.Fields(tt.command)
			code, out, stderr := invoke(t, append(parts, "--json"), false, func(root *cobra.Command) {
				leaf := &cobra.Command{Use: parts[len(parts)-1], Args: cobra.NoArgs}
				SetHandler(leaf, func(context.Context, Options, []string) (Result, error) { return tt.result, tt.err })
				parent := root
				if len(parts) > 1 {
					var err error
					parent, _, err = root.Find([]string{parts[0]})
					if err != nil {
						t.Fatal(err)
					}
					old, _, err := parent.Find([]string{parts[1]})
					if err != nil {
						t.Fatal(err)
					}
					parent.RemoveCommand(old)
				}
				for _, existing := range parent.Commands() {
					if existing.Name() == leaf.Name() {
						parent.RemoveCommand(existing)
					}
				}
				parent.AddCommand(leaf)
			})
			if code != tt.code {
				t.Fatalf("code %d, want %d; %s", code, tt.code, stderr)
			}
			object(t, out, code, "daypack "+tt.command)
			if tt.err != nil && stderr == "" {
				t.Fatal("missing error diagnostic")
			}
			if strings.Contains(stderr, "\nentry") || strings.Contains(stderr, "\nRetry") {
				t.Fatal("multiline diagnostic")
			}
		})
	}
}

func TestJSONUsageAndHelp(t *testing.T) {
	environment(t)
	for _, args := range [][]string{
		{"--json"}, {"--json", "--help"}, {"help", "--json"},
		{"--json", "--unknown"}, {"--unknown", "--json=true"},
		{"unknown", "--json=1"}, {"--json", "--quiet=invalid"},
		{"--json", "--project="}, {"--json", "--library="},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, out, stderr := invoke(t, args, false, nil)
			command := "daypack"
			if len(args) > 0 && args[0] == "help" {
				command += " help"
			}
			value := object(t, out, code, command)
			if value["help"] != nil {
				if code != Success || strings.Contains(out, "completion") {
					t.Fatalf("bad help: %s", out)
				}
			} else if code != Usage || stderr == "" {
				t.Fatalf("expected usage error, got %d %s", code, stderr)
			}
		})
	}
}

func TestNestedOptionsAndDirectoryPrecedence(t *testing.T) {
	home := environment(t)
	cwd, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct{ name, daypack, xdg, library, project, wantLibrary string }{
		{name: "home", wantLibrary: filepath.Join(home, ".config", "daypack")},
		{name: "xdg", xdg: filepath.Join(home, "xdg"), wantLibrary: filepath.Join(home, "xdg", "daypack")},
		{name: "environment", daypack: "environment-library", xdg: filepath.Join(home, "xdg"), wantLibrary: filepath.Join(cwd, "environment-library")},
		{name: "explicit", daypack: "environment-library", library: "custom-library", project: "custom-project", wantLibrary: filepath.Join(cwd, "custom-library")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DAYPACK_HOME", tt.daypack)
			t.Setenv("XDG_CONFIG_HOME", tt.xdg)
			args := []string{"library", "doctor", "--json", "--quiet", "--no-color"}
			if tt.library != "" {
				args = append(args, "--library", tt.library)
			}
			if tt.project != "" {
				args = append(args, "--project", tt.project)
			}
			called := false
			code, out, stderr := invoke(t, args, true, func(root *cobra.Command) {
				group, _, err := root.Find([]string{"library"})
				if err != nil {
					t.Fatal(err)
				}
				old, _, err := group.Find([]string{"doctor"})
				if err != nil {
					t.Fatal(err)
				}
				group.RemoveCommand(old)
				leaf := &cobra.Command{Use: "doctor", Args: cobra.NoArgs}
				SetHandler(leaf, func(_ context.Context, opts Options, _ []string) (Result, error) {
					called = true
					wantProject := cwd
					if tt.project != "" {
						wantProject = filepath.Join(cwd, tt.project)
					}
					if opts.LibraryDir != tt.wantLibrary || opts.ProjectDir != wantProject || !opts.JSON || !opts.Quiet || !opts.NoColor || opts.Interactive {
						t.Fatalf("unexpected options: %+v", opts)
					}
					return Result{Message: "optional chatter"}, nil
				})
				group.AddCommand(leaf)
			})
			if code != Success || !called || stderr != "" {
				t.Fatalf("code=%d called=%v stderr=%s", code, called, stderr)
			}
			object(t, out, code, "daypack library doctor")
		})
	}
}

func TestInteractivity(t *testing.T) {
	environment(t)
	for _, terminal := range []bool{false, true} {
		for _, jsonMode := range []bool{false, true} {
			for _, yes := range []bool{false, true} {
				args := []string{"apply"}
				if jsonMode {
					args = append(args, "--json")
				}
				if yes {
					args = append(args, "--yes")
				}
				requestedConfirmation := false
				code, out, _ := invoke(t, args, terminal, func(root *cobra.Command) {
					leaf := &cobra.Command{Use: "apply"}
					var assumeYes bool
					leaf.Flags().BoolVar(&assumeYes, "yes", false, "Accept gated changes")
					SetHandler(leaf, func(_ context.Context, opts Options, _ []string) (Result, error) {
						requestedConfirmation = opts.Interactive && !assumeYes
						if !opts.Interactive && !assumeYes {
							return Result{Code: Attention}, nil
						}
						return Result{}, nil
					})
					for _, existing := range root.Commands() {
						if existing.Name() == leaf.Name() {
							root.RemoveCommand(existing)
						}
					}
					root.AddCommand(leaf)
				})
				if requestedConfirmation != (terminal && !jsonMode && !yes) {
					t.Fatalf("unexpected confirmation: tty=%v json=%v yes=%v", terminal, jsonMode, yes)
				}
				wantCode := Success
				if (!terminal || jsonMode) && !yes {
					wantCode = Attention
				}
				if code != wantCode {
					t.Fatalf("code %d want %d", code, wantCode)
				}
				if jsonMode {
					object(t, out, code, "daypack apply")
				}
			}
		}
	}
}

func TestDefaultNonFileInputAndHumanOutput(t *testing.T) {
	environment(t)
	for _, quiet := range []bool{false, true} {
		var out, stderr bytes.Buffer
		args := []string{"status", "--no-color"}
		if quiet {
			args = append(args, "--quiet")
		}
		code := run(context.Background(), args, IO{In: strings.NewReader(""), Out: &out, Err: &stderr}, func(root *cobra.Command) {
			leaf := &cobra.Command{Use: "status", Args: cobra.NoArgs}
			SetHandler(leaf, func(_ context.Context, opts Options, _ []string) (Result, error) {
				if opts.Interactive {
					t.Fatal("non-file reader treated as TTY")
				}
				return Result{Message: "clean"}, nil
			})
			root.AddCommand(leaf)
		})
		if code != Success || strings.Contains(out.String(), "\x1b") || (quiet && out.Len() != 0) || (!quiet && out.String() != "clean\n") {
			t.Fatalf("unexpected human output %q code=%d", out.String(), code)
		}
	}
	code, _, stderr := invoke(t, []string{"--quiet", "--unknown"}, false, nil)
	if code != Usage || stderr == "" {
		t.Fatal("quiet suppressed error")
	}
}

func TestNoCommandExecutionOnUsageError(t *testing.T) {
	environment(t)
	for _, suffix := range [][]string{{"extra", "--json"}, {"--bad", "--json"}} {
		code, out, _ := invoke(t, append([]string{"status"}, suffix...), false, func(root *cobra.Command) {
			leaf := &cobra.Command{Use: "status", Args: cobra.NoArgs}
			SetHandler(leaf, func(context.Context, Options, []string) (Result, error) {
				t.Fatal("handler ran on usage error")
				return Result{}, nil
			})
			root.AddCommand(leaf)
		})
		if code != Usage {
			t.Fatalf("got %d", code)
		}
		object(t, out, code, "daypack status")
	}
}

func TestRunBuildsFreshInvocations(t *testing.T) {
	environment(t)
	for _, args := range [][]string{{"--json=true"}, {"--json=1"}, {"--json=false", "--json"}, {"--json", "--json=false"}, nil} {
		var out, stderr bytes.Buffer
		code := Run(context.Background(), args, IO{In: strings.NewReader(""), Out: &out, Err: &stderr})
		if code != Success || stderr.Len() != 0 {
			t.Fatalf("code %d stderr=%s", code, stderr.String())
		}
		if requestedJSON(args) {
			object(t, out.String(), code, "daypack")
		} else if !strings.HasPrefix(out.String(), "Manage project") {
			t.Fatalf("unexpected help: %q", out.String())
		}
	}
}

// failAfterWriter exercises failures on both the cause and hint writes.
type failAfterWriter struct {
	remaining int
	err       error
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.remaining == 0 {
		return 0, w.err
	}
	w.remaining--
	return len(p), nil
}

func TestDiagnosticWriteErrors(t *testing.T) {
	want := errors.New("diagnostic write failed")
	for _, successfulWrites := range []int{0, 1} {
		writer := &failAfterWriter{remaining: successfulWrites, err: want}
		if err := diagnostic(writer, "cause", "hint"); !errors.Is(err, want) {
			t.Fatalf("after %d writes: got %v, want %v", successfulWrites, err, want)
		}
	}
}

func TestRunDiagnosticWriteFailure(t *testing.T) {
	environment(t)
	var out bytes.Buffer
	code := Run(context.Background(), []string{"--unknown", "--json"}, IO{
		Out: &out,
		Err: &failAfterWriter{err: errors.New("diagnostic write failed")},
	})
	if code != Error {
		t.Fatalf("code %d, want %d", code, Error)
	}
	// The structured usage result is still delivered when stderr fails.
	object(t, out.String(), Usage, "daypack")
}
