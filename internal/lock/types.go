package lock

import (
	"fmt"
	"sort"
	"strings"
)

type Kind string

const (
	Symlink          Kind = "symlink"
	File             Kind = "file"
	TOMLTable        Kind = "toml-table"
	JSONEntry        Kind = "json-entry"
	JSONArrayElement Kind = "json-array-element"
	MDSection        Kind = "md-section"
)

type Item struct {
	MCPIdentity string `toml:"mcp_identity,omitempty"`
	Target      string `toml:"target"`
	Entry       string `toml:"entry"`
	Kind        Kind   `toml:"kind"`
	Path        string `toml:"path"`
	Link        string `toml:"link,omitempty"`
	Hash        string `toml:"hash,omitempty"`
	File        string `toml:"file,omitempty"`
	Key         string `toml:"key,omitempty"`
	Value       string `toml:"value,omitempty"`
	Marker      string `toml:"marker,omitempty"`
	Tracked     bool   `toml:"tracked"`
	Preserve    bool   `toml:"preserve,omitempty"`
}
type Lock struct {
	Version int
	items   []Item
}

func New() *Lock { return &Lock{Version: 1} }
func identity(i Item) string {
	switch i.Kind {
	case File, Symlink:
		return i.Path
	case TOMLTable:
		return i.Path + "\x00toml\x00" + i.Key
	case JSONEntry:
		return i.Path + "\x00json\x00" + i.Key
	case JSONArrayElement:
		return i.Path + "\x00array\x00" + i.Key + "\x00" + i.Value
	default:
		return i.Path + "\x00md\x00" + i.Marker
	}
}
func (l *Lock) Items() []Item {
	out := append([]Item(nil), l.items...)
	sort.Slice(out, func(a, b int) bool { return identity(out[a]) < identity(out[b]) })
	return out
}
func (l *Lock) Find(i Item) (Item, bool) {
	for _, v := range l.items {
		if identity(v) == identity(i) {
			return v, true
		}
	}
	return Item{}, false
}
func (l *Lock) Forget(i Item) {
	for n, v := range l.items {
		if identity(v) == identity(i) {
			l.items = append(l.items[:n], l.items[n+1:]...)
			return
		}
	}
}
func validateItem(i Item) error {
	if err := relativePath(i.Path); err != nil {
		return err
	}
	if i.File != "" && i.File != i.Path {
		return fmt.Errorf("file differs from path")
	}
	switch i.Kind {
	case File, Symlink:
	case JSONEntry, JSONArrayElement, TOMLTable:
		if _, err := keySegments(i.Key); err != nil {
			return err
		}
	case MDSection:
		return validateMarker(i.Marker)
	default:
		return fmt.Errorf("unknown kind %q", i.Kind)
	}
	return nil
}
func overlap(a, b Item) bool {
	if a.Path != b.Path {
		return false
	}
	if a.Kind == File || a.Kind == Symlink || b.Kind == File || b.Kind == Symlink {
		return true
	}
	if a.Kind == MDSection || b.Kind == MDSection {
		return a.Kind != b.Kind || a.Marker == b.Marker
	}
	if a.Key == b.Key {
		return a.Kind != JSONArrayElement || b.Kind != JSONArrayElement || a.Value == b.Value
	}
	return strings.HasPrefix(a.Key, b.Key+".") || strings.HasPrefix(b.Key, a.Key+".")
}
func (l *Lock) Put(i Item) error {
	if err := validateItem(i); err != nil {
		return err
	}
	for n, v := range l.items {
		if identity(v) == identity(i) {
			if v.Kind != i.Kind || v.Target != i.Target || v.Entry != i.Entry {
				return fmt.Errorf("ownership producer collision")
			}
			l.items[n] = i
			return nil
		}
		if overlap(v, i) {
			return fmt.Errorf("overlapping ownership at %s", i.Path)
		}
	}
	l.items = append(l.items, i)
	return nil
}
