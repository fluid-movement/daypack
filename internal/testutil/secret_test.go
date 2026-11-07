package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSentinelCleanProject(t *testing.T) {
	f := NewFixture(t, false)
	sentinel := PlantSentinel(t, f)
	if os.Getenv("DAYPACK_TEST_SECRET") != sentinel {
		t.Fatal("sentinel env not planted")
	}
	info, err := os.Stat(filepath.Join(f.Library, "secrets.env"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("secrets.env must be private")
	}
	writeFixture(t, filepath.Join(f.Project, ".config.json"), `{"TOKEN":"${DAYPACK_TEST_SECRET}"}`)
	AssertNoSecret(t, f.Project, sentinel, "${DAYPACK_TEST_SECRET}", "")
}

func TestSentinelDetectsLeaks(t *testing.T) {
	for _, location := range []string{"normal.txt", ".hidden/token", "stdout", "stderr", "link", "chain"} {
		t.Run(location, func(t *testing.T) {
			f := NewFixture(t, false)
			sentinel := PlantSentinel(t, f)
			outputs := []string{"", ""}
			switch location {
			case "stdout":
				outputs[0] = sentinel
			case "stderr":
				outputs[1] = sentinel
			case "link", "chain":
				target := filepath.Join(f.Library, "secrets.env")
				if location == "chain" {
					intermediate := filepath.Join(f.Library, "alias")
					if err := os.Symlink(target, intermediate); err != nil {
						t.Fatal(err)
					}
					target = intermediate
				}
				if err := os.Symlink(target, filepath.Join(f.Project, "innocent-link")); err != nil {
					t.Fatal(err)
				}
			default:
				writeFixture(t, filepath.Join(f.Project, location), sentinel)
			}
			findings, err := scanSecrets(f.Project, sentinel, outputs...)
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != 1 {
				t.Fatalf("expected one finding, got %d", len(findings))
			}
			if strings.Contains(strings.Join(findings, "\n"), sentinel) {
				t.Fatal("diagnostics exposed sentinel")
			}
		})
	}
}

func TestSentinelDoesNotFollowExternalLinks(t *testing.T) {
	f := NewFixture(t, false)
	sentinel := PlantSentinel(t, f)
	outside := t.TempDir()
	writeFixture(t, filepath.Join(outside, "private.txt"), sentinel)
	if err := os.Symlink(outside, filepath.Join(f.Project, "external")); err != nil {
		t.Fatal(err)
	}
	AssertNoSecret(t, f.Project, sentinel)
	if _, err := scanSecrets(f.Project, ""); err == nil {
		t.Fatal("empty sentinel accepted")
	}
}
