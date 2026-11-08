package cli

import (
	"context"
	"os"

	"github.com/fluid-movement/daypack/internal/importer"
	"github.com/fluid-movement/daypack/internal/plan"
	"github.com/spf13/cobra"
)

func libraryImportCommand() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "import", Short: "Import user skills, agents and MCP definitions", Args: cobra.NoArgs}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview imports without writing anything")
	SetHandler(cmd, func(ctx context.Context, opts Options, _ []string) (Result, error) {
		home, err := os.UserHomeDir()
		if err != nil {
			return Result{}, err
		}
		claude := os.Getenv("CLAUDE_CONFIG_DIR")

		pi := os.Getenv("PI_CODING_AGENT_DIR")

		imported, err := importer.Import(ctx, importer.Options{HomeDir: home, ClaudeConfigDir: claude, PiAgentDir: pi, LibraryDir: opts.LibraryDir, ProjectDir: opts.ProjectDir, DryRun: dryRun})
		if err != nil {
			return Result{}, err
		}
		code := Success
		if imported.Attention() {
			code = Attention
		}
		for _, item := range imported.Items {
			if item.Outcome == plan.Failed {
				code = Error
			}
		}
		result, err := PlanResult(imported.Plan, code)
		result.Items = imported.Items
		return result, err
	})
	return cmd
}
