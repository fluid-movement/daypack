package apply

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/fluid-movement/daypack/internal/plan"
)

// bridgeStub is the whole content of a CLAUDE.md that daypack owns.
const bridgeStub = "@AGENTS.md\n"

// committedSection reports rows covered by the committed-instruction
// exception: managed sections of AGENTS.md and of a handwritten CLAUDE.md.
func committedSection(i lock.Item) bool {
	return i.Kind == lock.MDSection && (i.Target == contract.AgentsMD || i.Target == contract.ClaudeMD)
}

// shapeInstructions decides how the Claude import bridge is owned. An absent
// CLAUDE.md (and no earlier section row) becomes an owned stub file; any
// existing CLAUDE.md gets a managed section and is otherwise left alone. A
// previous ownership kind is never switched. It also keeps the bridge when an
// unresolved fragment may still need it.
func (r *Reconciliation) shapeInstructions(m *manifest.Manifest, items []contract.Item) ([]contract.Item, error) {
	for _, e := range m.Entries() {
		if e.Type == library.Fragment && r.blocked["fragment:"+e.Name] && slices.Contains(e.Harnesses, "claude-code") {
			r.blocked[contract.BridgeEntry] = true
		}
	}
	for n, item := range items {
		if item.Target != contract.ClaudeMD {
			continue
		}
		file := false
		prior := false
		for _, old := range r.initial {
			if old.Path == item.Path {
				prior, file = true, old.Kind == lock.File
			}
		}
		if !prior {
			_, err := os.Lstat(filepath.Join(r.root, item.Path))
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			file = os.IsNotExist(err)
		}
		if file {
			items[n].Kind, items[n].Marker, items[n].Bytes = lock.File, "", []byte(bridgeStub)
		}
	}
	return items, nil
}

// sectionGroups lists work indexes of markdown sections per file, in plan order.
func (r *Reconciliation) sectionGroups() map[string][]int {
	groups := map[string][]int{}
	for n, w := range r.work {
		if w.item.Kind == lock.MDSection {
			groups[w.item.Path] = append(groups[w.item.Path], n)
		}
	}
	return groups
}

// finishSections applies file-wide section rules before the report is built:
// a drifted or conflicting section skips every other pending change in its
// file, and a pure reordering becomes a reported update.
func (r *Reconciliation) finishSections() error {
	for path, group := range r.sectionGroups() {
		stop := false
		for _, n := range group {
			op := r.work[n].op
			if op.Outcome == plan.Skipped && (op.Action == plan.Drift || op.Action == plan.Conflict) {
				stop = true
			}
		}
		if stop {
			for _, n := range group {
				if op := &r.work[n].op; op.Outcome == plan.Planned {
					op.Outcome, op.Reason = plan.Skipped, "another section in this file needs attention; file batch skipped"
				}
			}
			continue
		}
		var desired, managed []string
		for _, n := range group {
			w := r.work[n]
			managed = append(managed, w.item.Marker)
			if w.op.Action != plan.Remove {
				desired = append(desired, w.item.Marker)
			}
		}
		data, err := os.ReadFile(filepath.Join(r.root, path))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		moved, err := lock.MovedSections(data, desired, managed)
		if err != nil {
			return err
		}
		for _, n := range group {
			w := &r.work[n]
			if slices.Contains(moved, w.item.Marker) && w.op.Outcome == plan.Unchanged {
				w.op.Action, w.op.Outcome, w.op.Reason = plan.Update, plan.Planned, "reorder"
			}
		}
	}
	return nil
}

// reconcileBatch computes the new bytes of a markdown file for one executed
// batch. Only sections that are pending or unchanged take part; skipped or
// retained sections stay exactly where they are. Removals and updates run as
// separate batches, so each only manages its own rows.
func (r *Reconciliation) reconcileBatch(path string, data []byte, removing bool) ([]byte, error) {
	var managed []string
	var desired []lock.Section
	for _, w := range r.work {
		if w.item.Kind != lock.MDSection || w.item.Path != path || w.op.Outcome == plan.Skipped {
			continue
		}
		switch {
		case w.op.Action == plan.Remove:
			if removing && w.observation.State != lock.Missing && !w.item.Preserve {
				managed = append(managed, w.item.Marker)
			}
		case !removing:
			managed = append(managed, w.item.Marker)
			desired = append(desired, lock.Section{Marker: w.item.Marker, Body: w.payload.Bytes})
		}
	}
	return lock.ReconcileSections(data, desired, managed)
}
