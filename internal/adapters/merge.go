package adapters

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"sort"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
)

type Result struct {
	Targets     []contract.Target
	Items       []contract.Item
	Unsupported []contract.Unsupported
	Notes       []contract.Note
}

type pair struct{ target, entry string }
type producer struct {
	name  string
	items []contract.Item
}

// Desired merges only the selected (entry, harness) pairs. Entry order is
// retained so callers can later aggregate instruction fragments in manifest order.
func (r *Registry) Desired(entries []contract.Entry, project contract.Project) (Result, error) {
	var result Result
	selected := map[string]bool{}
	for _, name := range project.Harnesses {
		if _, ok := r.adapters[name]; !ok {
			return Result{}, fmt.Errorf("unknown harness %q", name)
		}
		selected[name] = true
	}
	for _, name := range r.Names() {
		if !selected[name] {
			continue
		}
		p := project
		p.Harnesses = slices.Clone(project.Harnesses)
		result.Notes = append(result.Notes, r.adapters[name].Notes(p)...)
	}
	slices.SortFunc(result.Notes, func(a, b contract.Note) int {
		return slices.Compare([]string{a.Harness, a.Code, a.Message}, []string{b.Harness, b.Code, b.Message})
	})
	result.Notes = slices.Compact(result.Notes)
	groups := map[pair]producer{}
	used := map[string]bool{}
	ownership := lock.New()
	for _, entry := range entries {
		if entry.Ref == "" {
			return Result{}, fmt.Errorf("empty entry ref")
		}
		names := entry.Harnesses
		if names == nil {
			names = project.Harnesses
		} else if len(names) == 0 {
			return Result{}, fmt.Errorf("entry %q has empty harness selection", entry.Ref)
		}
		names = slices.Clone(names)
		sort.Strings(names)
		names = slices.Compact(names)
		for _, name := range names {
			if !selected[name] {
				return Result{}, fmt.Errorf("entry %q harness %q is not selected", entry.Ref, name)
			}
			a := r.adapters[name]
			if !a.Supports(entry.Artifact.Type) {
				result.Unsupported = append(result.Unsupported, contract.Unsupported{Harness: name, Entry: entry.Ref, Type: entry.Artifact.Type})
				continue
			}
			p := project
			p.Harnesses = slices.Clone(project.Harnesses)
			items, err := a.Desired(contract.CloneEntry(entry), p)
			if err != nil {
				var unsupported *contract.UnsupportedError
				if errors.As(err, &unsupported) && unsupported != nil {
					fields := slices.Clone(unsupported.Fields)
					slices.Sort(fields)
					fields = slices.Compact(fields)
					result.Unsupported = append(result.Unsupported, contract.Unsupported{Harness: name, Entry: entry.Ref, Type: entry.Artifact.Type, Fields: fields})
					continue
				}
				return Result{}, fmt.Errorf("adapter %q rendering entry %q failed", name, entry.Ref)
			}
			byTarget := map[string][]contract.Item{}
			for _, original := range items {
				item, err := r.validateItem(name, entry, original)
				if err != nil {
					return Result{}, err
				}
				byTarget[item.Target] = append(byTarget[item.Target], item)
			}
			targets := make([]string, 0, len(byTarget))
			for target := range byTarget {
				targets = append(targets, target)
			}
			sort.Strings(targets)
			for _, target := range targets {
				list := byTarget[target]
				sort.Slice(list, func(i, j int) bool { return itemOrder(list[i]) < itemOrder(list[j]) })
				key := pair{target, entry.Ref}
				if old, ok := groups[key]; ok {
					if !reflect.DeepEqual(old.items, list) {
						return Result{}, fmt.Errorf("target %q entry %q disagrees between adapters %q and %q", target, entry.Ref, old.name, name)
					}
					continue
				}
				for _, item := range list {
					// Reuse lock's physical identity/overlap rules. Repeated physical
					// identities within one group must also fail, rather than replace.
					owned := lock.Item{Target: item.Target, Entry: item.Entry, Kind: item.Kind, Path: item.Path, Link: item.Link, Key: item.Key, Value: string(item.Value), Marker: item.Marker}
					if _, exists := ownership.Find(owned); exists {
						return Result{}, fmt.Errorf("duplicate physical item for target %q entry %q", target, entry.Ref)
					}
					if err := ownership.Put(owned); err != nil {
						return Result{}, fmt.Errorf("physical item conflict for target %q entry %q", target, entry.Ref)
					}
					result.Items = append(result.Items, item)
				}
				groups[key] = producer{name, list}
				used[target] = true
			}
		}
	}
	for _, name := range r.Names() {
		pa, ok := r.adapters[name].(contract.ProjectAdapter)
		if !ok || !selected[name] {
			continue
		}
		var effective []contract.Entry
		for _, entry := range entries {
			if (entry.Harnesses == nil || slices.Contains(entry.Harnesses, name)) && r.adapters[name].Supports(entry.Artifact.Type) {
				effective = append(effective, contract.CloneEntry(entry))
			}
		}
		p := project
		p.Harnesses = slices.Clone(project.Harnesses)
		items, err := pa.DesiredProject(effective, p)
		if err != nil {
			return Result{}, fmt.Errorf("adapter %q rendering project items failed", name)
		}
		for _, original := range items {
			item, err := r.validateProjectItem(name, original)
			if err != nil {
				return Result{}, err
			}
			owned := lock.Item{Target: item.Target, Entry: item.Entry, Kind: item.Kind, Path: item.Path, Marker: item.Marker}
			if _, exists := ownership.Find(owned); exists {
				return Result{}, fmt.Errorf("duplicate physical item for target %q project item", item.Target)
			}
			if err := ownership.Put(owned); err != nil {
				return Result{}, fmt.Errorf("physical item conflict for target %q project item", item.Target)
			}
			result.Items = append(result.Items, item)
			used[item.Target] = true
		}
	}
	for _, t := range r.Targets() {
		if used[t.ID] {
			result.Targets = append(result.Targets, t)
		}
	}
	sort.Slice(result.Unsupported, func(i, j int) bool {
		a, b := result.Unsupported[i], result.Unsupported[j]
		if a.Entry != b.Entry {
			return a.Entry < b.Entry
		}
		return a.Harness < b.Harness
	})
	return result, nil
}

func itemOrder(i contract.Item) string {
	// JSON encoding is deterministic and includes all payload fields. It is used
	// only internally for ordering, never in diagnostics.
	b, err := json.Marshal(i) // Item contains only strings and byte slices.
	if err != nil {
		panic(err)
	}
	return string(b)
}

func (r *Registry) validateItem(name string, entry contract.Entry, item contract.Item) (contract.Item, error) {
	fail := func() (contract.Item, error) {
		return contract.Item{}, fmt.Errorf("adapter %q returned invalid item for target %q entry %q", name, item.Target, entry.Ref)
	}
	t, ok := r.declared[name][item.Target]
	if !ok || !slices.Contains(t.Types, entry.Artifact.Type) || item.Entry != entry.Ref || !t.Contains(item.Path) {
		return fail()
	}
	item.Bytes = append([]byte(nil), item.Bytes...)
	item.Value = append([]byte(nil), item.Value...)
	switch item.Kind {
	case contract.Symlink:
		if item.Link == "" || len(item.Bytes) != 0 || len(item.Value) != 0 || item.Key != "" || item.Marker != "" {
			return fail()
		}
	case contract.File:
		if item.Link != "" || len(item.Value) != 0 || item.Key != "" || item.Marker != "" {
			return fail()
		}
	case contract.MarkdownSection:
		if item.Marker == "" || item.Link != "" || len(item.Value) != 0 || item.Key != "" {
			return fail()
		}
	case contract.JSONEntry, contract.JSONArrayElement:
		if item.Key == "" || item.Link != "" || item.Marker != "" || len(item.Bytes) != 0 {
			return fail()
		}
		dec := json.NewDecoder(bytes.NewReader(item.Value))
		dec.UseNumber()
		var value any
		if err := dec.Decode(&value); err != nil {
			return fail()
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			return fail()
		}
		if item.Kind == contract.JSONArrayElement {
			if _, ok := value.(string); !ok {
				return fail()
			}
		}
		canonical, err := json.Marshal(value)
		if err != nil {
			return fail()
		}
		item.Value = canonical
	default:
		return fail()
	}
	return item, nil
}

// validateProjectItem accepts only the reserved bridge section on a declared
// target; project-level items never claim per-entry references.
func (r *Registry) validateProjectItem(name string, item contract.Item) (contract.Item, error) {
	t, ok := r.declared[name][item.Target]
	if !ok || item.Target != contract.ClaudeMD || item.Entry != contract.BridgeEntry || item.Kind != contract.MarkdownSection || item.Marker != contract.BridgeMarker ||
		!slices.Contains(t.Types, library.Fragment) || !t.Contains(item.Path) || item.Link != "" || item.Key != "" || len(item.Value) != 0 ||
		bytes.Contains(item.Bytes, []byte("<!-- daypack:")) {
		return contract.Item{}, fmt.Errorf("adapter %q returned invalid project item for target %q", name, item.Target)
	}
	item.Bytes = append([]byte(nil), item.Bytes...)
	return item, nil
}
