//go:build !darwin && !linux

package library

import "fmt"

// Fail closed where an exclusive directory rename is unavailable.
func renameExclusive(source, destination string) error {
	return fmt.Errorf("exclusive artifact publication is unsupported on this platform: %s to %s", source, destination)
}
