package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// WriteSymlink atomically publishes a link without following the destination.
func WriteSymlink(root, p string, roots []string, target string) (err error) {
	if err = safeLock(root); err != nil {
		return err
	}
	if err = ValidatePath(root, p, roots); err != nil {
		return err
	}
	if s, e := os.Lstat(filepath.Join(root, p)); e == nil {
		if !s.Mode().IsRegular() && s.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("refuse replacing non-file %s", p)
		}
		if s.Mode()&os.ModeSymlink != 0 {
			v, e := os.Readlink(filepath.Join(root, p))
			if e != nil {
				return e
			}
			if v == target {
				return nil
			}
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if err = os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0755); err != nil {
		return err
	}
	if err = ValidatePath(root, p, roots); err != nil {
		return err
	}
	f, e := os.CreateTemp(filepath.Dir(filepath.Join(root, p)), ".daypack-link-*")
	if e != nil {
		return e
	}
	name := f.Name()
	if err = f.Close(); err != nil {
		return errors.Join(err, os.Remove(name))
	}
	if err = os.Remove(name); err != nil {
		return err
	}
	if err = os.Symlink(target, name); err != nil {
		return err
	}
	defer func() {
		if e := os.Remove(name); e != nil && !os.IsNotExist(e) {
			err = errors.Join(err, e)
		}
	}()
	if err = safeLock(root); err != nil {
		return err
	}
	if err = ValidatePath(root, p, roots); err != nil {
		return err
	}
	if s, e := os.Lstat(filepath.Join(root, p)); e == nil && !s.Mode().IsRegular() && s.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("refuse replacing non-file %s", p)
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	return os.Rename(name, filepath.Join(root, p))
}
