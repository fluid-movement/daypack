package lock

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type document struct {
	Version int    `toml:"version"`
	Items   []Item `toml:"item"`
}

func Load(root string) (*Lock, error) {
	if err := safeLock(root); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(root, ".daypack/lock.toml"))
	if os.IsNotExist(err) {
		return New(), nil
	}
	if err != nil {
		return nil, err
	}
	var d document
	meta, err := toml.Decode(string(b), &d)
	if err != nil {
		return nil, err
	}
	if d.Version != 1 || len(meta.Undecoded()) != 0 {
		return nil, fmt.Errorf("unsupported lock format")
	}
	l := New()
	for _, i := range d.Items {
		if _, ok := l.Find(i); ok {
			return nil, fmt.Errorf("duplicate lock identity")
		}
		if err := l.Put(i); err != nil {
			return nil, err
		}
		if err := safeParents(root, i.Path); err != nil {
			return nil, err
		}
	}
	return l, nil
}
