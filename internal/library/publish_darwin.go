package library

import "golang.org/x/sys/unix"

// The exclusive rename publishes files and folders atomically without replacing
// even an empty directory or a dangling symlink created by another process.
func renameExclusive(source, destination string) error {
	return unix.RenameatxNp(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_EXCL)
}
