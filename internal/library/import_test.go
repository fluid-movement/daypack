package library_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func TestImportPreflightAndExclusiveMCP(t *testing.T) {
	f := testutil.NewFixture(t, false)
	root, err := filepath.EvalSymlinks(f.Library)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/.env\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := testutil.SnapshotTree(t, root)
	if err = library.CheckSecrets(root, f.Project, map[string]string{"IMPORTED_TOKEN": "literal"}); err != nil {
		t.Fatal(err)
	}
	testutil.AssertTree(t, root, before)
	if err = library.WriteSecrets(root, f.Project, map[string]string{"IMPORTED_TOKEN": "literal"}); err != nil {
		t.Fatal(err)
	}
	before = testutil.SnapshotTree(t, root)
	if err = library.CheckSecrets(root, f.Project, map[string]string{"IMPORTED_TOKEN": "different", "NEW_TOKEN": "new"}); err == nil {
		t.Fatal("conflict accepted")
	}
	testutil.AssertTree(t, root, before)
	d := library.MCPDefinition{Type: "stdio", Command: "echo", Env: map[string]string{"TOKEN": "${IMPORTED_TOKEN}"}}
	a, err := library.AddMCP(root, "imported", d)
	if err != nil || a.MCP == nil {
		t.Fatal("MCP add failed", err)
	}
	before = testutil.SnapshotTree(t, root)
	if _, err = library.AddMCP(root, "imported", library.MCPDefinition{Type: "stdio", Command: "other"}); err == nil {
		t.Fatal("destination overwritten")
	}
	if _, err = library.AddMCP(root, "invalid", library.MCPDefinition{Type: "stdio", Command: "echo", URL: "https://example.invalid"}); err == nil {
		t.Fatal("invalid fields dropped")
	}
	testutil.AssertTree(t, root, before)
	if _, err = library.ValidateNative(filepath.Join(root, "skills", "testing"), library.Skill, "testing"); err != nil {
		t.Fatal(err)
	}
}
