package manifest_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/fluid-movement/daypack/internal/testutil"
)

func TestCatalog(t *testing.T) {
	f := testutil.NewFixture(t, false)
	lib, err := library.Load(f.Library)
	if err != nil {
		t.Fatal(err)
	}
	load := func(text string) *manifest.Manifest {
		path := filepath.Join(t.TempDir(), "daypack.toml")
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		m, err := manifest.LoadManifest(path)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	both := []string{"claude-code", "pi"}
	t.Run("all types used", func(t *testing.T) {
		m, err := manifest.LoadManifest(filepath.Join(f.Project, "daypack.toml"))
		if err != nil {
			t.Fatal(err)
		}
		got := manifest.Catalog(lib, m)
		if len(got) != 4 {
			t.Fatalf("got %v", got)
		}
		for i, kind := range manifest.ArtifactTypes {
			if got[i].Type != kind || !got[i].Used || !reflect.DeepEqual(got[i].Harnesses, both) {
				t.Fatalf("entry %d: %+v", i, got[i])
			}
		}
	})
	t.Run("restriction and inline", func(t *testing.T) {
		m := load("version=1\nharnesses=['claude-code','pi']\nskills=[{name='testing',harnesses=['pi']}]\n[inline.mcp.local]\ntype='stdio'\ncommand='echo'\n")
		got := manifest.Catalog(lib, m)
		if len(got) != 5 || !reflect.DeepEqual(got[0].Harnesses, []string{"pi"}) {
			t.Fatalf("got %+v", got)
		}
		if got[1].Name != "docs" || got[1].Used || len(got[1].Harnesses) != 0 {
			t.Fatalf("docs: %+v", got[1])
		}
		if got[2].Name != "local" || got[2].Source != manifest.SourceProject || !got[2].Used {
			t.Fatalf("inline: %+v", got[2])
		}
	})
	t.Run("no project", func(t *testing.T) {
		got := manifest.Catalog(lib, nil)
		if len(got) != 4 {
			t.Fatalf("got %v", got)
		}
		for _, e := range got {
			if e.Used || e.Source != manifest.SourceLibrary {
				t.Fatalf("%+v", e)
			}
		}
		if !reflect.DeepEqual(got, manifest.Catalog(lib, nil)) {
			t.Fatal("nondeterministic")
		}
	})
}
