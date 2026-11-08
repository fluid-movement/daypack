package apply

import (
	"slices"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/plan"
)

// Inspection is a read-only, isolated projection of a prepared reconciliation.
// It carries no execution payloads and mutating it cannot affect the
// reconciliation.
type Inspection struct {
	Project   contract.Project
	Entries   []contract.Entry
	Desired   []lock.Item
	Ownership *lock.Lock
	Plan      plan.Plan
}

// Inspection returns copies of the resolved entries, desired ownership rows,
// observed ownership and presentation plan. It performs no I/O and never plans.
func (r *Reconciliation) Inspection() Inspection {
	in := Inspection{Project: contract.Project{Root: r.project.Root, Harnesses: slices.Clone(r.project.Harnesses)}, Ownership: lock.New(), Plan: r.Plan()}
	for _, e := range r.entries {
		in.Entries = append(in.Entries, contract.CloneEntry(e))
	}
	slices.SortFunc(in.Entries, func(a, b contract.Entry) int {
		switch {
		case a.Ref < b.Ref:
			return -1
		case a.Ref > b.Ref:
			return 1
		}
		return 0
	})
	if r.desired != nil {
		in.Desired = r.desired.Items()
	}
	if r.owned != nil {
		for _, i := range r.owned.Items() {
			// Items were validated when the lock was loaded.
			if err := in.Ownership.Put(i); err != nil {
				panic(err)
			}
		}
	}
	return in
}
