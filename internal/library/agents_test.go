package library_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func TestAgentFrontmatterValidation(t *testing.T) {
	cases := []struct{ name, body, field string }{
		{"absent", "Review carefully.\n", "frontmatter"},
		{"unterminated", "---\nname: reviewer\ndescription: Review\n", "frontmatter"},
		{"malformed", "---\nname: [\ndescription: Review\n---\n", "frontmatter"},
		{"missing-name", "---\ndescription: Review\n---\n", "frontmatter.name"},
		{"blank-name", "---\nname: ' '\ndescription: Review\n---\n", "frontmatter.name"},
		{"missing-description", "---\nname: reviewer\n---\n", "frontmatter.description"},
		{"blank-description", "---\nname: reviewer\ndescription: ' '\n---\n", "frontmatter.description"},
		{"mismatched-name", "---\nname: another\ndescription: Review\n---\n", "frontmatter.name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := testutil.NewFixture(t, false)
			path := filepath.Join(f.Library, "agents/reviewer.md")
			if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			before := testutil.SnapshotTree(t, f.Project)
			lib, err := library.Load(f.Library)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, p := range lib.Validate() {
				if p.Severity == library.Error && p.File == path && p.Field == tc.field {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing source/field diagnostic: %v", lib.Validate())
			}
			_, err = lib.Resolve("agent:reviewer")
			var invalid *library.ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("Resolve: %v", err)
			}
			m, err := manifest.LoadManifest(filepath.Join(f.Project, "daypack.toml"))
			if err != nil {
				t.Fatal(err)
			}
			rejected := false
			for _, p := range m.Validate(lib) {
				if p.Severity == library.Error && p.Field == "agent:reviewer" {
					rejected = true
				}
			}
			if !rejected {
				t.Fatal("manifest accepted invalid agent")
			}
			testutil.AssertTree(t, f.Project, before)
		})
	}
}
