package status

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/plan"
)

type pairKey struct{ entry, harness string }

type health struct {
	in       Input
	opts     Options
	root     string
	targets  map[string]contract.Target
	roots    []string
	entryMap map[string]contract.Entry
	checks   []Check
	// extra holds states contributed by checks, keyed by entry.
	extra map[string][]extraState
}

func newHealth(in Input, opts Options) (*health, error) {
	h := &health{in: in, opts: opts, root: in.Project.Root, targets: map[string]contract.Target{}, entryMap: map[string]contract.Entry{}, extra: map[string][]extraState{}, checks: []Check{}}
	for _, t := range opts.Registry.Targets() {
		h.targets[t.ID] = t
		h.roots = append(h.roots, t.Root)
	}
	for _, e := range in.Entries {
		h.entryMap[e.Ref] = e
	}
	return h, nil
}

type extraState struct {
	target string
	state  State
}

func (h *health) add(c Check, target string, state State) {
	h.checks = append(h.checks, c)
	if c.Entry != "" && state != "" {
		h.extra[c.Entry] = append(h.extra[c.Entry], extraState{target, state})
	}
}

// ownership observes every locked item whose target is in the trusted catalog.
func (h *health) ownership(ctx context.Context) error {
	for _, item := range h.in.Ownership.Items() {
		if err := ctx.Err(); err != nil {
			return err
		}
		t, known := h.targets[item.Target]
		if !known || !t.Contains(item.Path) {
			continue
		}
		if err := lock.ValidatePath(h.root, item.Path, h.roots); err != nil {
			return fmt.Errorf("unsafe managed path %s", item.Path)
		}
		obs, err := h.in.Ownership.Status(h.root, item)
		if err != nil {
			return fmt.Errorf("inspect managed path %s: %w", item.Path, err)
		}
		c := Check{Entry: item.Entry, Path: item.Path, Severity: library.Error}
		switch obs.State {
		case lock.Missing:
			c.Code, c.Message = "missing-managed-path", "managed content is missing"
			h.add(c, item.Target, Pending)
			continue
		case lock.Drifted:
			c.Code, c.Message = "managed-path-drift", "managed content was changed"
			h.add(c, item.Target, Drift)
		case lock.Collision:
			c.Code, c.Message = "managed-path-collision", "managed path holds unowned content"
			h.add(c, item.Target, Collision)
		}
		if item.Kind == lock.Symlink {
			if _, err := os.Stat(filepath.Join(h.root, filepath.FromSlash(item.Path))); err != nil {
				h.add(Check{Code: "dangling-library-link", Entry: item.Entry, Path: item.Path, Message: "managed link does not resolve to a library artifact", Severity: library.Error}, item.Target, Unresolved)
			}
		}
	}
	return nil
}

// declares reports whether a harness's adapter declares the target for the type.
func (h *health) declares(harness, target string) bool {
	for _, t := range h.opts.Registry.Declared(harness) {
		if t.ID == target {
			return true
		}
	}
	return false
}

func stateOf(a plan.Action) State {
	switch a {
	case plan.Conflict:
		return Collision
	case plan.Drift:
		return Drift
	case plan.Unresolved:
		return Unresolved
	case plan.Unsupported:
		return Unsupported
	}
	return Pending
}

// rank implements collision > drift > unresolved > unsupported > pending > ok.
func rank(s State) int {
	switch s {
	case Collision:
		return 5
	case Drift:
		return 4
	case Unresolved:
		return 3
	case Unsupported:
		return 2
	case Pending:
		return 1
	}
	return 0
}

func (h *health) pairs() []pairKey {
	seen := map[pairKey]bool{}
	for _, e := range h.in.Entries {
		for _, name := range e.Harnesses {
			seen[pairKey{e.Ref, name}] = true
		}
	}
	// Entries without a resolved definition, and obsolete owned entries, get one
	// row with no harness.
	for _, o := range h.in.Plan.Operations {
		if _, ok := h.entryMap[o.Entry]; !ok {
			seen[pairKey{o.Entry, ""}] = true
		}
	}
	for _, i := range h.in.Ownership.Items() {
		if _, ok := h.entryMap[i.Entry]; !ok {
			seen[pairKey{i.Entry, ""}] = true
		}
	}
	out := make([]pairKey, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].entry != out[j].entry {
			return out[i].entry < out[j].entry
		}
		return out[i].harness < out[j].harness
	})
	return out
}

// belongs attributes an operation to a pair using declarations and selections.
func (h *health) belongs(p pairKey, o plan.Operation) bool {
	if o.Entry != p.entry {
		return false
	}
	if _, ok := h.entryMap[o.Entry]; !ok {
		return p.harness == ""
	}
	if p.harness == "" {
		return false
	}
	if o.Target == "" {
		return o.Harness == "" || o.Harness == p.harness
	}
	return h.declares(p.harness, o.Target)
}

func (h *health) entries() []EntryStatus {
	out := []EntryStatus{}
	for _, p := range h.pairs() {
		row := EntryStatus{Entry: p.entry, Harness: p.harness, State: OK, Operations: []plan.Operation{}}
		for _, o := range h.in.Plan.Operations {
			if h.belongs(p, o) {
				row.Operations = append(row.Operations, o)
				if s := stateOf(o.Action); rank(s) > rank(row.State) {
					row.State = s
				}
			}
		}
		for _, x := range h.extra[p.entry] {
			if (p.harness == "" || h.declares(p.harness, x.target)) && rank(x.state) > rank(row.State) {
				row.State = x.state
			}
		}
		out = append(out, row)
	}
	return out
}

// readers renders the authoritative exposure table for targets in use.
func (h *health) readers() []TargetReaders {
	producers := map[string]map[string]bool{}
	use := func(target, entry string) {
		if _, ok := h.targets[target]; !ok {
			return
		}
		if producers[target] == nil {
			producers[target] = map[string]bool{}
		}
		e, ok := h.entryMap[entry]
		if !ok {
			return
		}
		for _, name := range e.Harnesses {
			if h.declares(name, target) {
				producers[target][name] = true
			}
		}
	}
	for _, i := range h.in.Desired {
		use(i.Target, i.Entry)
	}
	for _, i := range h.in.Ownership.Items() {
		use(i.Target, i.Entry)
	}
	ids := make([]string, 0, len(producers))
	for id := range producers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := []TargetReaders{}
	for _, id := range ids {
		selected := []string{}
		for name := range producers[id] {
			selected = append(selected, name)
		}
		sort.Strings(selected)
		all := adapters.Readers(id)
		slices.Sort(all)
		all = slices.Compact(all)
		other := []string{}
		for _, r := range all {
			if !producers[id][r] {
				other = append(other, r)
			}
		}
		out = append(out, TargetReaders{Target: id, Selected: selected, Readers: all, OtherReaders: other})
	}
	return out
}

func sortChecks(c []Check) {
	slices.SortStableFunc(c, func(a, b Check) int {
		return slices.Compare([]string{a.Code, a.Entry, a.Harness, a.Path, a.Variable, a.Message}, []string{b.Code, b.Entry, b.Harness, b.Path, b.Variable, b.Message})
	})
}
