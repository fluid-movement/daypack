package library

import (
	"fmt"
	"os"
)

// EditPath returns a native file even if its content needs repair.
func EditPath(root string, kind ArtifactType, name string) (string, error) {
	path, err := nativePath(root, kind, name)
	if err != nil {
		return "", err
	}
	path = contentPath(path, kind)
	if _, err = safePath(path); err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("open artifact %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("artifact %s is not a regular file", path)
	}
	return path, nil
}
func Remove(root string, kind ArtifactType, name string) error {
	path, err := nativePath(root, kind, name)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("remove artifact %s: %w", path, err)
	}
	if kind == Skill && !info.IsDir() || kind != Skill && !info.Mode().IsRegular() {
		return fmt.Errorf("artifact %s has wrong file type", path)
	}
	return os.RemoveAll(path)
}
