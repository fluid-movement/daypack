package adapters

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/fluid-movement/daypack/internal/adapters/claudecode"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/adapters/pi"
	"github.com/fluid-movement/daypack/internal/library"
)

type Registry struct {
	adapters map[string]contract.Adapter
	targets  map[string]contract.Target
	declared map[string]map[string]contract.Target
}

// DefaultRegistry is the single production registration list.
func DefaultRegistry() (*Registry, error) { return NewRegistry(claudecode.New(), pi.New()) }

func NewRegistry(adapters ...contract.Adapter) (*Registry, error) {
	r := &Registry{adapters: map[string]contract.Adapter{}, targets: map[string]contract.Target{}, declared: map[string]map[string]contract.Target{}}
	for _, a := range adapters {
		if a == nil || reflect.ValueOf(a).Kind() == reflect.Pointer && reflect.ValueOf(a).IsNil() {
			return nil, fmt.Errorf("nil adapter")
		}
		name := a.Name()
		if name == "" || r.adapters[name] != nil {
			return nil, fmt.Errorf("empty or duplicate adapter name %q", name)
		}
		r.adapters[name] = a
		r.declared[name] = map[string]contract.Target{}
		for _, target := range a.Targets() {
			t := target
			t.Types = slices.Clone(t.Types)
			slices.Sort(t.Types)
			t.Types = slices.Compact(t.Types)
			if t.ID == "" || contract.ValidatePath(t.Root) != nil || (t.Mode != contract.FileRoot && t.Mode != contract.DirectoryRoot) || len(t.Types) == 0 {
				return nil, fmt.Errorf("adapter %q has invalid target %q", name, t.ID)
			}
			for _, typ := range t.Types {
				if typ != library.Skill && typ != library.MCP && typ != library.Agent && typ != library.Fragment || !a.Supports(typ) {
					return nil, fmt.Errorf("adapter %q target %q has unsupported type %q", name, t.ID, typ)
				}
			}
			if _, exists := r.declared[name][t.ID]; exists {
				return nil, fmt.Errorf("adapter %q repeats target %q", name, t.ID)
			}
			if old, exists := r.targets[t.ID]; exists && !reflect.DeepEqual(old, t) {
				return nil, fmt.Errorf("adapter %q disagrees on target %q root, mode or types", name, t.ID)
			}
			for id, old := range r.targets {
				if id != t.ID && (old.Root == t.Root || strings.HasPrefix(t.Root, old.Root+"/") || strings.HasPrefix(old.Root, t.Root+"/")) {
					return nil, fmt.Errorf("targets %q and %q overlap", id, t.ID)
				}
			}
			r.targets[t.ID] = t
			r.declared[name][t.ID] = t
		}
	}
	return r, nil
}
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func (r *Registry) Lookup(name string) (contract.Adapter, bool) {
	a, ok := r.adapters[name]
	return a, ok
}

// Targets returns independent trusted declarations, including targets of
// currently unselected adapters, for planning and ownership validation.
func (r *Registry) Targets() []contract.Target {
	out := make([]contract.Target, 0, len(r.targets))
	for _, t := range r.targets {
		t.Types = slices.Clone(t.Types)
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return strings.Compare(out[i].ID, out[j].ID) < 0 })
	return out
}
