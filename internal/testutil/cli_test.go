package testutil

import (
	"strings"
	"testing"

	"github.com/fluid-movement/daypack/internal/cli"
)

func TestRunCLI(t *testing.T) {
	f := NewFixture(t, false)
	t.Setenv("DAYPACK_HOME", t.TempDir())
	result := RunCLI(t, f, "--json")
	if result.Code != cli.Success || result.Stderr != "" || !strings.Contains(result.Stdout, `"ok":true`) {
		t.Fatalf("unexpected CLI result: %+v", result)
	}
	result = RunCLI(t, f, "--unknown")
	if result.Code != cli.Usage || result.Stderr == "" {
		t.Fatalf("usage streams: %+v", result)
	}
	result = RunCLIWithInput(t, f, strings.NewReader("yes\n"), true, "--help")
	if result.Code != cli.Success || !strings.Contains(result.Stdout, "Manage project") {
		t.Fatalf("explicit input: %+v", result)
	}
}

func TestRunCLIExplicitDirectoryOverrides(t *testing.T) {
	f := NewFixture(t, false)
	t.Setenv("DAYPACK_HOME", t.TempDir())
	// An empty fixture override must reach the real parser despite valid env defaults.
	// There are no feature commands yet that expose resolved directories; the CLI's
	// own TestNestedOptionsAndDirectoryPrecedence verifies resolved option values.
	f.Library = ""
	result := RunCLI(t, f)
	if result.Code != cli.Usage || !strings.Contains(result.Stderr, "--library must not be empty") {
		t.Fatalf("library override missing: %+v", result)
	}
	f.Library = t.TempDir()
	f.Project = ""
	result = RunCLI(t, f)
	if result.Code != cli.Usage || !strings.Contains(result.Stderr, "--project must not be empty") {
		t.Fatalf("project override missing: %+v", result)
	}
}
