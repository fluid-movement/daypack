package library

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func Init(path string) error { _, err := InitWithProblems(path); return err }

func InitWithProblems(path string) (problems []Problem, err error) {
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if info, e := os.Stat(path); e == nil {
		if !info.IsDir() {
			return nil, fmt.Errorf("library %s is not a directory", path)
		}
		return RefreshHousekeeping(path)
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	path = filepath.Join(parent, filepath.Base(path))
	stage, clean, err := stagePath(path)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, clean()) }()
	if err = os.Mkdir(stage, 0755); err != nil {
		return nil, err
	}
	for _, name := range []string{"skills", "mcp", "fragments", "agents"} {
		if err = os.Mkdir(filepath.Join(stage, name), 0755); err != nil {
			return nil, fmt.Errorf("initialize library %s: %w", path, err)
		}
	}
	if problems, err = RefreshHousekeeping(stage); err != nil {
		return nil, err
	}
	return problems, publish(stage, path)
}
