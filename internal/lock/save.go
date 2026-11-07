package lock

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

func (l *Lock) Save(root string) error {
	if l.Version != 1 {
		return fmt.Errorf("unsupported lock version")
	}
	if err := safeLock(root); err != nil {
		return err
	}
	checked := New()
	for _, i := range l.Items() {
		if err := checked.Put(i); err != nil {
			return err
		}
		if err := safeParents(root, i.Path); err != nil {
			return err
		}
	}
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(document{1, l.Items()}); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, ".daypack"), 0700); err != nil {
		return err
	}
	if err := safeLock(root); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(root, ".daypack/lock.toml"), b.Bytes(), 0600)
}
