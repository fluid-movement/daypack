package manifest

import (
	"fmt"
	"os"
	"path/filepath"
)

// FindProject searches from start through the nearest repository boundary.
func FindProject(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("inspect project directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project start must be a directory")
	}
	for {
		info, err := os.Stat(filepath.Join(dir, "daypack.toml"))
		if err == nil {
			if !info.Mode().IsRegular() {
				return "", fmt.Errorf("%s: daypack.toml must be a regular file", dir)
			}
			return dir, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect manifest: %w", err)
		}
		_, err = os.Lstat(filepath.Join(dir, ".git"))
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect git boundary: %w", err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no daypack.toml found; run daypack init")
}
