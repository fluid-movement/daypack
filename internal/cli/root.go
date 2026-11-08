package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// IO supplies process streams and allows terminal detection to be injected.
type IO struct {
	In         io.Reader
	Out, Err   io.Writer
	IsTerminal func(io.Reader) bool
}

// Options is captured for each invocation and passed explicitly to handlers.
type Options struct {
	JSON, Quiet, NoColor   bool
	ProjectDir, LibraryDir string
	Interactive            bool
}

type invocationKey struct{}
type invocation struct {
	options Options
	streams IO
	result  Result
	handled bool
}

// Handler adapts package operations to the CLI without exposing Cobra to core.
type Handler func(context.Context, Options, []string) (Result, error)

// SetHandler wires a leaf command to the shared invocation contract.
func SetHandler(cmd *cobra.Command, handler Handler) {
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		state := cmd.Context().Value(invocationKey{}).(*invocation)
		state.handled = true
		var err error
		state.result, err = handler(cmd.Context(), state.options, args)
		return err
	}
}

// NewRootCommand builds an independent command tree for one invocation.
func NewRootCommand(streams IO) *cobra.Command {
	state := &invocation{streams: streams}
	root := &cobra.Command{
		Use: "daypack", Short: "Manage project AI configuration",
		SilenceErrors: true, SilenceUsage: true,
		Args: cobra.NoArgs,
	}
	root.SetContext(context.WithValue(context.Background(), invocationKey{}, state))
	root.SetIn(streams.In)
	root.SetOut(streams.Out)
	root.SetErr(streams.Err)
	root.CompletionOptions.DisableDefaultCmd = true
	flags := root.PersistentFlags()
	flags.BoolVar(&state.options.JSON, "json", false, "Print one JSON result (non-interactive)")
	flags.BoolVar(&state.options.Quiet, "quiet", false, "Suppress optional output")
	flags.BoolVar(&state.options.NoColor, "no-color", false, "Disable color")
	flags.StringVar(&state.options.ProjectDir, "project", "", "Project directory")
	flags.StringVar(&state.options.LibraryDir, "library", "", "Library directory (overrides DAYPACK_HOME)")
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		err := state.prepare(cmd)
		var failure *Failure
		if err != nil && !errors.As(err, &failure) {
			return &Failure{Code: Error, Cause: err}
		}
		return err
	}
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		var buf bytes.Buffer
		previous := cmd.OutOrStdout()
		cmd.SetOut(&buf)
		defaultHelp(cmd, args)
		cmd.SetOut(previous)
		state.result.Help = buf.String()
	})
	root.RunE = func(cmd *cobra.Command, args []string) error { return cmd.Help() }
	root.AddCommand(libraryCommand(streams, runEditor), applyCommand(streams))
	return root
}

func (s *invocation) prepare(cmd *cobra.Command) error {
	for _, name := range []string{"project", "library"} {
		if cmd.Flags().Changed(name) {
			value, err := cmd.Flags().GetString(name)
			if err != nil {
				return fmt.Errorf("read --%s: %w", name, err)
			}
			if value == "" {
				return &Failure{Code: Usage, Cause: fmt.Errorf("--%s must not be empty", name), Hint: "Supply a directory."}
			}
		}
	}
	if s.options.ProjectDir == "" {
		s.options.ProjectDir = "."
	}
	if s.options.LibraryDir == "" {
		s.options.LibraryDir = os.Getenv("DAYPACK_HOME")
		if s.options.LibraryDir == "" {
			base := os.Getenv("XDG_CONFIG_HOME")
			if base == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				base = filepath.Join(home, ".config")
			}
			s.options.LibraryDir = filepath.Join(base, "daypack")
		}
	}
	var err error
	s.options.ProjectDir, err = filepath.Abs(s.options.ProjectDir)
	if err != nil {
		return err
	}
	s.options.LibraryDir, err = filepath.Abs(s.options.LibraryDir)
	if err != nil {
		return err
	}
	detect := s.streams.IsTerminal
	if detect == nil {
		detect = func(reader io.Reader) bool {
			file, ok := reader.(*os.File)
			return ok && term.IsTerminal(int(file.Fd()))
		}
	}
	s.options.Interactive = !s.options.JSON && detect(s.streams.In)
	return nil
}
