package library

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func Init(path string) (err error) {
	path, err = safePath(path)
	if err != nil {
		return err
	}
	if err = absent(path); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	stage, clean, err := stagePath(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, clean()) }()
	if err = os.Mkdir(stage, 0755); err != nil {
		return err
	}
	for _, name := range []string{"skills", "mcp", "fragments", "agents"} {
		if err = os.Mkdir(filepath.Join(stage, name), 0755); err != nil {
			return fmt.Errorf("initialize library %s: %w", path, err)
		}
	}
	if err = os.WriteFile(filepath.Join(stage, ".gitignore"), []byte("/secrets.env\n"), 0644); err != nil {
		return err
	}
	return publish(stage, path)
}
