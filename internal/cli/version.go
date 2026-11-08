package cli

import (
	"context"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// version is stamped at build time with -ldflags "-X <pkg>.version=<tag>".
var version = "dev"

// buildVersion returns the stamped version, falling back to the module version
// recorded by "go install module@version" when no stamp is present.
func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

type versionItem struct {
	Version string `json:"version"`
}

func versionCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "version", Short: "Print the daypack version", Args: cobra.NoArgs}
	SetHandler(cmd, func(context.Context, Options, []string) (Result, error) {
		v := buildVersion()
		return Result{Items: versionItem{Version: v}, Message: "daypack " + v}, nil
	})
	return cmd
}
