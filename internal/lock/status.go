package lock

import (
	"fmt"
	"os"
	"path/filepath"
)

type State string

const (
	OK        State = "ok"
	Missing   State = "missing"
	Drifted   State = "drifted"
	Collision State = "collision"
)

type Observation struct {
	State                      State
	Converged, Adopt, Preserve bool
}

func (l *Lock) Status(root string, i Item) (Observation, error) {
	if err := validateItem(i); err != nil {
		return Observation{}, err
	}
	if err := safeLock(root); err != nil {
		return Observation{}, err
	}
	if err := safeParents(root, i.Path); err != nil {
		return Observation{}, err
	}
	old, owned := l.Find(i)
	current, exists, err := observe(root, i)
	if err != nil {
		return Observation{}, err
	}
	if !exists {
		return Observation{State: Missing}, nil
	}
	desired := i.Hash
	previous := old.Hash
	if i.Kind == Symlink {
		desired = i.Link
		previous = old.Link
	}
	if i.Kind == JSONArrayElement {
		desired = i.Value
		previous = old.Value
	}
	if current == "\x00wrong kind" {
		state := Collision
		if owned {
			state = Drifted
		}
		return Observation{State: state}, nil
	}
	if current == desired {
		return Observation{State: OK, Converged: true, Adopt: !owned, Preserve: old.Preserve || (!owned && i.Kind == JSONArrayElement)}, nil
	}
	if !owned {
		return Observation{State: Collision}, nil
	}
	if current == previous {
		return Observation{State: OK, Preserve: old.Preserve}, nil
	}
	return Observation{State: Drifted, Preserve: old.Preserve}, nil
}
func observe(root string, i Item) (string, bool, error) {
	path := filepath.Join(root, i.Path)
	s, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if i.Kind == Symlink {
		if s.Mode()&os.ModeSymlink == 0 {
			return "\x00wrong kind", true, nil
		}
		v, e := os.Readlink(path)
		return v, true, e
	}
	if !s.Mode().IsRegular() {
		return "\x00wrong kind", true, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	switch i.Kind {
	case File:
		return HashBytes(b), true, nil
	case JSONEntry, JSONArrayElement:
		v, _, e := jsonValue(b, i.Key)
		if e != nil {
			return "", false, e
		}
		if i.Kind == JSONArrayElement {
			idx, e := arrayIndex(v, i.Value)
			return i.Value, idx >= 0, e
		}
		if !v.Exists() {
			return "", false, nil
		}
		h, e := HashJSON([]byte(v.Raw))
		return h, true, e
	case MDSection:
		a, z, found, e := section(b, i.Marker)
		if e != nil || !found {
			return "", found, e
		}
		return HashBytes(b[a:z]), true, nil
	}
	return "", false, fmt.Errorf("unsupported kind")
}

// RemoveOwned refuses drift/collisions and leaves shared containers in place.
// The caller saves the lock after the successful batch.
func (l *Lock) RemoveOwned(root string, i Item, roots []string) error {
	old, ok := l.Find(i)
	if !ok {
		return fmt.Errorf("unowned item")
	}
	obs, err := l.Status(root, old)
	if err != nil {
		return err
	}
	if obs.State == Drifted || obs.State == Collision {
		return fmt.Errorf("refuse removal: %s", obs.State)
	}
	if err := ValidatePath(root, old.Path, roots); err != nil {
		return err
	}
	if obs.State == Missing || old.Preserve {
		l.Forget(old)
		return nil
	}
	p := filepath.Join(root, old.Path)
	if old.Kind == File || old.Kind == Symlink {
		if err := safeLock(root); err != nil {
			return err
		}
		if err := ValidatePath(root, old.Path, roots); err != nil {
			return err
		}
		err = os.Remove(p)
	} else {
		var b []byte
		b, err = os.ReadFile(p)
		if err != nil {
			return err
		}
		if old.Kind == MDSection {
			b, err = RemoveSection(b, old.Marker)
		} else {
			b, err = RemoveJSON(b, old)
		}
		if err != nil {
			return err
		}
		s, e := os.Stat(p)
		if e != nil {
			return e
		}
		err = WriteFile(root, old.Path, roots, b, s.Mode().Perm())
	}
	if err != nil {
		return err
	}
	l.Forget(old)
	return nil
}
