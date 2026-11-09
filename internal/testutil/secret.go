package testutil

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func PlantSentinel(t *testing.T, f Fixture) string {
	t.Helper()
	sentinel := "DAYPACK_SYNTHETIC_SECRET_6fe28c1b_DO_NOT_COPY"
	t.Setenv("DAYPACK_TEST_SECRET", sentinel)
	path := filepath.Join(f.Library, ".env")
	if err := os.WriteFile(path, []byte("DAYPACK_TEST_SECRET="+sentinel+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	return sentinel
}

func AssertNoSecret(t *testing.T, project, sentinel string, outputs ...string) {
	t.Helper()
	findings, err := scanSecrets(project, sentinel, outputs...)
	if err != nil {
		t.Fatal("secret scan failed: ", err)
	}
	for _, finding := range findings {
		t.Error("secret exposure: " + finding)
	}
}

// scanSecrets reports locations only, never the matching bytes.
func scanSecrets(project, sentinel string, outputs ...string) ([]string, error) {
	if sentinel == "" {
		return nil, fmt.Errorf("empty sentinel")
	}
	var findings []string
	for i, output := range outputs {
		if bytes.Contains([]byte(output), []byte(sentinel)) {
			findings = append(findings, fmt.Sprintf("output[%d]", i))
		}
	}
	err := filepath.WalkDir(project, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			resolved := target
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(path), resolved)
			}
			// Resolving the link path detects chains without reading target contents.
			if final, err := filepath.EvalSymlinks(path); err == nil {
				resolved = final
			}
			if bytes.Contains([]byte(target), []byte(sentinel)) || (filepath.Base(filepath.Clean(target)) == "secrets.env" || filepath.Base(filepath.Clean(target)) == ".env") || (filepath.Base(resolved) == "secrets.env" || filepath.Base(resolved) == ".env") {
				findings = append(findings, "link: "+path)
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(sentinel)) {
			findings = append(findings, "file: "+path)
		}
		return nil
	})
	return findings, err
}
