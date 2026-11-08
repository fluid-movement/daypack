package cli

import (
	"context"
	"os"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/apply"
	"github.com/fluid-movement/daypack/internal/status"
	"github.com/spf13/cobra"
)

func statusCommand() *cobra.Command {
	return statusCommandWithRegistry(adapters.DefaultRegistry)
}

func statusCommandWithRegistry(registry func() (*adapters.Registry, error)) *cobra.Command {
	cmd := &cobra.Command{Use: "status", Aliases: []string{"check"}, Short: "Report project state and drift without changing anything", Args: cobra.NoArgs}
	SetHandler(cmd, func(ctx context.Context, options Options, _ []string) (Result, error) {
		registered, err := registry()
		if err != nil {
			return Result{}, err
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return Result{}, err
		}
		r, err := apply.Prepare(ctx, options.ProjectDir, options.LibraryDir, registered, apply.Options{})
		if err != nil {
			return Result{}, err
		}
		report, err := status.Inspect(ctx, r.Inspection(), status.Options{Registry: registered, HomeDir: home, LookupEnv: os.LookupEnv})
		if err != nil {
			return Result{}, err
		}
		return StatusResult(report)
	})
	return cmd
}

// StatusResult wraps a report with the exit code implied by its contents.
func StatusResult(r status.Report) (Result, error) {
	code := Success
	if r.NeedsAttention() {
		code = Attention
	}
	return Result{Code: code, Status: &r}, nil
}
