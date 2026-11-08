package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fluid-movement/daypack/internal/hooks"
	"github.com/spf13/cobra"
)

func hookCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "hook", Short: "Manage the git post-checkout auto-apply hook"}
	install := &cobra.Command{Use: "install", Short: "Install a post-checkout hook running 'daypack apply --auto'", Args: cobra.NoArgs}
	SetHandler(install, func(ctx context.Context, options Options, _ []string) (Result, error) {
		executable, err := os.Executable()
		if err != nil {
			return Result{}, err
		}
		if executable, err = filepath.EvalSymlinks(executable); err != nil {
			return Result{}, err
		}
		if executable, err = filepath.Abs(executable); err != nil {
			return Result{}, err
		}
		res, err := hooks.Install(ctx, options.ProjectDir, executable)
		if err != nil {
			return Result{}, err
		}
		if res.Conflict {
			return hookResult(res, Attention, ""),
				&Failure{Code: Attention, Cause: fmt.Errorf("existing post-checkout hook not changed; add: %s", res.Invocation), Hint: "Edit " + res.Path + " by hand."}
		}
		msg := "Hook already installed: " + res.Path
		if res.Changed {
			msg = "Installed hook: " + res.Path
		}
		return hookResult(res, Success, msg), nil
	})
	uninstall := &cobra.Command{Use: "uninstall", Short: "Remove the post-checkout hook installed by daypack", Args: cobra.NoArgs}
	SetHandler(uninstall, func(ctx context.Context, options Options, _ []string) (Result, error) {
		res, err := hooks.Uninstall(ctx, options.ProjectDir)
		if err != nil {
			return Result{}, err
		}
		if res.Conflict {
			return hookResult(res, Attention, ""), &Failure{Code: Attention, Cause: errors.New("post-checkout hook was not installed by daypack and was left in place"), Hint: "Remove the daypack line from " + res.Path + " by hand."}
		}
		msg := "No daypack hook installed."
		if res.Changed {
			msg = "Removed hook: " + res.Path
		}
		return hookResult(res, Success, msg), nil
	})
	cmd.AddCommand(install, uninstall)
	return cmd
}

func hookResult(res hooks.Result, code int, message string) Result {
	return Result{Code: code, Items: res, Message: message}
}
