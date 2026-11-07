package library

import (
	"os"
	"path/filepath"
)

func DefaultPath() (string, error) {
	if p := os.Getenv("DAYPACK_HOME"); p != "" {
		return filepath.Abs(p)
	}
	if p := os.Getenv("XDG_CONFIG_HOME"); p != "" {
		return filepath.Abs(filepath.Join(p, "daypack"))
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(h, ".config", "daypack"))
}
