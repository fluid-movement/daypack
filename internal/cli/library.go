package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/fluid-movement/daypack/internal/library"
	"github.com/spf13/cobra"
)

type artifactItem struct {
	Type library.ArtifactType `json:"type"`
	Name string               `json:"name"`
	Path string               `json:"path"`
}

func artifactResult(a library.Artifact, action string) Result {
	return Result{Items: []artifactItem{{a.Type, a.Name, a.Path}}, Message: fmt.Sprintf("%s %s:%s (%s)", action, a.Type, a.Name, a.Path)}
}
func referenceWarning(kind library.ArtifactType, name, path string) []library.Problem {
	return []library.Problem{{Severity: library.Warning, File: path, Message: fmt.Sprintf("Manifests using %s:%s will fail until updated.", kind, name)}}
}

type editorRunner func(context.Context, string, string, IO) error

func runEditor(ctx context.Context, editor, path string, streams IO) error {
	cmd := exec.CommandContext(ctx, editor, path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = streams.In, streams.Out, streams.Err
	return cmd.Run()
}
func libraryCommand(streams IO, editor editorRunner) *cobra.Command {
	group := &cobra.Command{Use: "library", Short: "Manage the artifact library"}
	init := &cobra.Command{Use: "init", Short: "Create a library or refresh its generated instructions", Args: cobra.NoArgs}
	SetHandler(init, func(_ context.Context, opts Options, _ []string) (Result, error) {
		if err := library.Init(opts.LibraryDir); err != nil {
			return Result{}, err
		}
		return Result{Message: "Initialized library instructions at " + opts.LibraryDir}, nil
	})
	add := &cobra.Command{Use: "add <type> <name>", Short: "Add a native artifact or valid stub", Args: cobra.ExactArgs(2)}
	var source string
	add.Flags().StringVar(&source, "from", "", "Import a native file or skill folder")
	SetHandler(add, func(_ context.Context, opts Options, args []string) (Result, error) {
		kind := library.ArtifactType(args[0])
		if err := library.ValidateName(kind, args[1]); err != nil {
			return Result{}, &Failure{Code: Usage, Cause: err}
		}
		a, err := library.Add(opts.LibraryDir, library.AddOptions{Type: kind, Name: args[1], Source: source})
		if err != nil {
			return Result{}, err
		}
		return artifactResult(a, "Added"), nil
	})
	edit := &cobra.Command{Use: "edit <type> <name>", Short: "Open the native file in $EDITOR (executable path)", Args: cobra.ExactArgs(2)}
	SetHandler(edit, func(ctx context.Context, opts Options, args []string) (Result, error) {
		kind := library.ArtifactType(args[0])
		if err := library.ValidateName(kind, args[1]); err != nil {
			return Result{}, &Failure{Code: Usage, Cause: err}
		}
		path, err := library.EditPath(opts.LibraryDir, kind, args[1])
		if err != nil {
			return Result{}, err
		}
		if opts.JSON || !opts.Interactive {
			return Result{}, &Failure{Code: Error, Cause: fmt.Errorf("editor requires an interactive terminal; native file: %s", path), Hint: "Edit the reported path outside JSON mode in a terminal."}
		}
		executable := os.Getenv("EDITOR")
		if strings.TrimSpace(executable) == "" {
			return Result{}, &Failure{Code: Error, Cause: fmt.Errorf("EDITOR is unset; native file: %s", path), Hint: "Set EDITOR to an executable path."}
		}
		if err = editor(ctx, executable, path, streams); err != nil {
			return Result{}, fmt.Errorf("editor failed for %s", path)
		}
		return artifactResult(library.Artifact{Type: kind, Name: args[1], Path: path}, "Edited"), nil
	})
	rename := &cobra.Command{Use: "rename <type> <old-name> <new-name>", Short: "Rename a library artifact", Args: cobra.ExactArgs(3)}
	SetHandler(rename, func(_ context.Context, opts Options, args []string) (Result, error) {
		kind := library.ArtifactType(args[0])
		for _, name := range args[1:] {
			if err := library.ValidateName(kind, name); err != nil {
				return Result{}, &Failure{Code: Usage, Cause: err}
			}
		}
		a, err := library.Rename(opts.LibraryDir, kind, args[1], args[2])
		if err != nil {
			return Result{}, err
		}
		result := artifactResult(a, "Renamed")
		if args[1] != args[2] {
			result.Problems = referenceWarning(kind, args[1], a.Path)
		}
		return result, nil
	})
	remove := &cobra.Command{Use: "remove <type> <name>", Short: "Remove a library artifact", Args: cobra.ExactArgs(2)}
	SetHandler(remove, func(_ context.Context, opts Options, args []string) (Result, error) {
		kind := library.ArtifactType(args[0])
		if err := library.ValidateName(kind, args[1]); err != nil {
			return Result{}, &Failure{Code: Usage, Cause: err}
		}
		if err := library.Remove(opts.LibraryDir, kind, args[1]); err != nil {
			return Result{}, err
		}
		return Result{Message: fmt.Sprintf("Removed %s:%s", kind, args[1]), Problems: referenceWarning(kind, args[1], opts.LibraryDir)}, nil
	})
	doctor := &cobra.Command{Use: "doctor", Short: "Validate the library and secrets Git hygiene", Args: cobra.NoArgs}
	var fix bool
	doctor.Flags().BoolVar(&fix, "fix", false, "Refresh generated library instructions before validation")
	SetHandler(doctor, func(_ context.Context, opts Options, _ []string) (Result, error) {
		if fix {
			if err := library.RefreshInstructions(opts.LibraryDir); err != nil {
				return Result{}, err
			}
		}
		problems, err := library.Doctor(opts.LibraryDir)
		if err != nil {
			return Result{}, err
		}
		result := Result{Problems: problems, Message: "Library validation finished."}
		for _, p := range problems {
			if p.Severity == library.Error {
				result.Code = Error
			}
		}
		return result, nil
	})
	group.AddCommand(init, add, edit, rename, remove, doctor, libraryImportCommand())
	return group
}
