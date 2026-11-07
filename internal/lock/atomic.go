package lock

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
)

// atomicWrite exposes failure seams to package tests without global mutable hooks.
func atomicWrite(path string, data []byte, mode os.FileMode, write func(*os.File, []byte) error, rename func(string, string) error) (err error) {
	old, err := os.ReadFile(path)
	if err == nil && bytes.Equal(old, data) {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".daypack-*")
	if err != nil {
		return err
	}
	defer func() {
		if cleanupErr := os.Remove(f.Name()); cleanupErr != nil && !os.IsNotExist(cleanupErr) {
			err = errors.Join(err, cleanupErr)
		}
	}()
	if err = f.Chmod(mode); err != nil {
		return errors.Join(err, f.Close())
	}
	if err = write(f, data); err != nil {
		return errors.Join(err, f.Close())
	}
	if err = f.Close(); err != nil {
		return err
	}
	return rename(f.Name(), path)
}
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	return atomicWrite(path, data, mode, func(f *os.File, b []byte) error { _, e := f.Write(b); return e }, os.Rename)
}

// WriteFile validates trusted roots again immediately before replacing a target.
func WriteFile(root, p string, roots []string, data []byte, mode os.FileMode) error {
	if err := safeLock(root); err != nil {
		return err
	}
	if err := ValidatePath(root, p, roots); err != nil {
		return err
	}
	if s, e := os.Lstat(filepath.Join(root, p)); e == nil && !s.Mode().IsRegular() {
		return &os.PathError{Op: "write", Path: p, Err: os.ErrInvalid}
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	return writeAtomic(filepath.Join(root, p), data, mode)
}
