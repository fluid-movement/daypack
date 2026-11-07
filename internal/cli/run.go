package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// Run executes a fresh tree, renders once, and returns a process exit code.
func Run(ctx context.Context, args []string, streams IO) int {
	return run(ctx, args, streams, nil)
}

func run(ctx context.Context, args []string, streams IO, configure func(*cobra.Command)) int {
	if streams.In == nil {
		streams.In = strings.NewReader("")
	}
	if streams.Out == nil {
		streams.Out = io.Discard
	}
	if streams.Err == nil {
		streams.Err = io.Discard
	}
	root := NewRootCommand(streams)
	state := root.Context().Value(invocationKey{}).(*invocation)
	if configure != nil {
		configure(root)
	}
	root.SetContext(context.WithValue(ctx, invocationKey{}, state))
	root.SetArgs(args)
	cmd, err := root.ExecuteC()
	if cmd == nil {
		cmd = root
	}
	// Parsing can fail before Cobra reaches the JSON flag. Preserve the request
	// without invoking command logic or reading values of directory flags as flags.
	state.options.JSON = requestedJSON(args)
	if err != nil {
		code, hint := Error, ""
		var failure *Failure
		if errors.As(err, &failure) {
			code, hint = failure.Code, failure.Hint
		} else if !state.handled {
			code, hint = Usage, "Run 'daypack --help' for usage."
		}
		state.result.Code = code
		state.result.Problems = []string{oneLine(err.Error())}
		diagnostic(streams.Err, err.Error(), hint)
	}
	if renderErr := render(streams.Out, state.options, cmd.CommandPath(), state.result); renderErr != nil {
		diagnostic(streams.Err, fmt.Sprintf("could not write result: %v", renderErr), "")
		return Error
	}
	return state.result.Code
}

func requestedJSON(args []string) bool {
	enabled := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if arg == "--project" || arg == "--library" {
			i++
			continue
		}
		if arg == "--json" {
			enabled = true
		}
		if value, ok := strings.CutPrefix(arg, "--json="); ok {
			if parsed, err := strconv.ParseBool(value); err == nil {
				enabled = parsed
			}
		}
	}
	return enabled
}
