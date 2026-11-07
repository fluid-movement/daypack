package testutil

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/cli"
)

type CLIResult struct {
	Code           int
	Stdout, Stderr string
}

func RunCLI(t *testing.T, f Fixture, args ...string) CLIResult {
	t.Helper()
	return RunCLIWithInput(t, f, strings.NewReader(""), false, args...)
}

// RunCLIWithInput uses product terminal detection injection with captured streams.
func RunCLIWithInput(t *testing.T, f Fixture, in io.Reader, terminal bool, args ...string) CLIResult {
	t.Helper()
	var out, stderr bytes.Buffer
	argv := append([]string{"--library", f.Library, "--project", f.Project}, args...)
	code := cli.Run(context.Background(), argv, cli.IO{In: in, Out: &out, Err: &stderr, IsTerminal: func(io.Reader) bool { return terminal }})
	return CLIResult{Code: code, Stdout: out.String(), Stderr: stderr.String()}
}
