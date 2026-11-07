package lock

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func relativePath(p string) error {
	if p == "" || p == "." || filepath.IsAbs(p) || filepath.VolumeName(p) != "" || strings.ContainsAny(p, "\x00\r\n\\:") {
		return fmt.Errorf("unsafe relative path %q", p)
	}
	for _, s := range strings.Split(p, "/") {
		if s == ".." || s == "." || s == "" {
			return fmt.Errorf("unsafe path component in %q", p)
		}
	}
	return nil
}

// ValidatePath requires trusted target roots supplied by the caller, not lock data.
func ValidatePath(root, p string, roots []string) error {
	if err := relativePath(p); err != nil {
		return err
	}
	allowed := p == ".daypack" || strings.HasPrefix(p, ".daypack/")
	for _, r := range roots {
		if err := relativePath(r); err != nil {
			return err
		}
		if p == r || strings.HasPrefix(p, r+"/") {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("path %q outside target roots", p)
	}
	return safeParents(root, p)
}
func safeParents(root, p string) error {
	if err := relativePath(p); err != nil {
		return err
	}
	// The core directory is never permitted to be a symlink, even for observations.
	if s, e := os.Lstat(filepath.Join(root, ".daypack")); e == nil {
		if s.Mode()&os.ModeSymlink != 0 || !s.IsDir() {
			return fmt.Errorf("unsafe .daypack directory")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	parts := strings.Split(p, "/")
	cur := root
	for _, part := range parts[:len(parts)-1] {
		cur = filepath.Join(cur, part)
		s, e := os.Lstat(cur)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if s.Mode()&os.ModeSymlink != 0 || !s.IsDir() {
			return fmt.Errorf("unsafe parent %s", cur)
		}
	}
	return nil
}
func inGit(root string) (bool, error) {
	cmd := exec.Command("git", "-C", root, "rev-parse", "--git-dir")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}
	if strings.Contains(string(out), "not a git repository") {
		return false, nil
	}
	return false, fmt.Errorf("git: %w: %s", err, out)
}
func IsTracked(root, p string) (bool, error) {
	if err := relativePath(p); err != nil {
		return false, err
	}
	ok, err := inGit(root)
	if err != nil || !ok {
		return false, err
	}
	out, err := exec.Command("git", "-C", root, "--literal-pathspecs", "ls-files", "-z", "--", p).Output()
	return len(out) > 0, err
}
func safeLock(root string) error {
	if err := safeParents(root, ".daypack/lock.toml"); err != nil {
		return err
	}
	if s, e := os.Lstat(filepath.Join(root, ".daypack/lock.toml")); e == nil {
		if !s.Mode().IsRegular() {
			return fmt.Errorf("lock must be a regular file")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	tracked, err := IsTracked(root, ".daypack/lock.toml")
	if err != nil {
		return err
	}
	if tracked {
		return fmt.Errorf("lock.toml is tracked")
	}
	return nil
}
