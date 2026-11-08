package cli

import (
	"context"
	"errors"
	"io"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/apply"
	"github.com/fluid-movement/daypack/internal/plan"
	"github.com/spf13/cobra"
)

func applyCommand(streams IO) *cobra.Command {
	return applyCommandWithRegistry(streams, adapters.DefaultRegistry)
}

func applyCommandWithRegistry(streams IO, registry func() (*adapters.Registry, error)) *cobra.Command {
	cmd := &cobra.Command{Use: "apply", Short: "Reconcile project configuration with its manifest", Args: cobra.NoArgs}
	var dryRun, yes, force, adopt bool
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without writing")
	cmd.Flags().BoolVar(&yes, "yes", false, "Approve inline MCP commands without prompting")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite or remove drifted managed items")
	cmd.Flags().BoolVar(&adopt, "adopt", false, "Take ownership of colliding items")
	SetHandler(cmd, func(ctx context.Context, options Options, _ []string) (Result, error) {
		registered, err := registry()
		if err != nil {
			return Result{}, err
		}
		r, err := apply.Prepare(ctx, options.ProjectDir, options.LibraryDir, registered, apply.Options{Force: force, Adopt: adopt})
		if err != nil {
			return Result{}, err
		}
		p := r.Plan()
		if plan.ConfirmationMode(p, options.Interactive && !options.JSON, yes, dryRun) == plan.Prompt && !options.Quiet {
			out := streams.Err
			if out == nil {
				out = io.Discard
			}
			if err := plan.RenderText(out, p); err != nil {
				return applyResult(p, Error, errors.New("could not write confirmation preview"))
			}
		}
		approved, err := ConfirmPlan(p, options, yes, dryRun, streams)
		if err != nil {
			return applyResult(p, Error, err)
		}
		if dryRun {
			return PlanResult(approved, applyExitCode(approved, true))
		}
		final, err := r.Execute(ctx, approved)
		if err != nil {
			return applyResult(final, Error, err)
		}
		return PlanResult(final, applyExitCode(final, false))
	})
	return cmd
}

func applyResult(p plan.Plan, code int, cause error) (Result, error) {
	result, err := PlanResult(p, code)
	if err != nil {
		return Result{}, err
	}
	return result, cause
}

func applyExitCode(p plan.Plan, dryRun bool) int {
	for _, op := range p.Operations {
		if op.Outcome == plan.Failed {
			return Error
		}
	}
	for _, op := range p.Operations {
		if op.Outcome == plan.Skipped {
			return Attention
		}
		switch op.Action {
		case plan.Conflict, plan.Drift, plan.Unresolved, plan.Unsupported:
			return Attention
		}
		if dryRun && (op.Outcome == "" || op.Outcome == plan.Planned) {
			return Attention
		}
	}
	return Success
}
