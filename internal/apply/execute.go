package apply

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"

	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/plan"
)

// Filesystem seams are private and per reconciliation, for deterministic failures.
type filesystem struct {
	write   func(string, string, []string, []byte, os.FileMode) error
	link    func(string, string, []string, string) error
	save    func(*lock.Lock, string) error
	exclude func(*lock.Lock, string) error
}

func defaultFilesystem() filesystem {
	return filesystem{lock.WriteFile, lock.WriteSymlink, func(l *lock.Lock, r string) error { return l.Save(r) }, func(l *lock.Lock, r string) error { return lock.UpdateExclude(r, l.Items()) }}
}

// Execute accepts only the planned operations, optionally marking gated rows
// skipped. It never executes paths or payloads supplied by a report consumer.
func (r *Reconciliation) Execute(ctx context.Context, approved plan.Plan) (plan.Plan, error) {
	result, err := r.approve(approved)
	if err != nil {
		return r.Plan(), err
	}
	if r.executed {
		return result, fmt.Errorf("reconciliation already executed")
	}
	r.executed = true
	if err = ctx.Err(); err != nil {
		return finish(result, err)
	}
	current, err := lock.Load(r.root)
	if err != nil {
		return finish(result, err)
	}
	if !reflect.DeepEqual(current.Items(), r.initial) {
		return finish(result, fmt.Errorf("ownership changed since preview"))
	}
	r.owned = current
	// A single declined or blocked member retains the entire inline entry.
	blocked := map[string]bool{}
	for _, o := range result.Operations {
		if _, inline := r.history[o.Entry]; inline && o.Outcome == plan.Skipped {
			blocked[o.Entry] = true
		}
	}
	indices := []int{}
	for n := range result.Operations {
		o := &result.Operations[n]
		if blocked[o.Entry] && o.Outcome == plan.Planned {
			o.Outcome = plan.Skipped
			o.Reason = "inline server was not approved as a whole"
		}
		if o.Outcome == plan.Planned {
			indices = append(indices, n)
		}
	}
	sort.Slice(indices, func(i, j int) bool {
		a, b := result.Operations[indices[i]], result.Operations[indices[j]]
		if (a.Action == plan.Remove) != (b.Action == plan.Remove) {
			return a.Action == plan.Remove
		}
		return slices.Compare([]string{a.Target, a.Entry, a.Path, a.Key, a.Marker, a.Member}, []string{b.Target, b.Entry, b.Path, b.Key, b.Marker, b.Member}) < 0
	})
	// Unchanged required members are private work too. They must participate in
	// inline preflight even though a no-op has no presentation row.
	activeInline := map[string]bool{}
	for _, n := range indices {
		ref := result.Operations[n].Entry
		if _, ok := r.history[ref]; ok {
			activeInline[ref] = true
		}
	}
	for _, w := range r.work {
		if !activeInline[w.item.Entry] {
			continue
		}
		stamp, e := r.stamp(w.item)
		if e != nil {
			return finish(result, e)
		}
		if stamp != w.stamp {
			blocked[w.item.Entry] = true
		}
	}
	// Preflight all items, so edits since preview cannot cause partial inline consent.
	for _, n := range indices {
		o := &result.Operations[n]
		w := r.findWork(*o)
		stamp, e := r.stamp(w.item)
		if e != nil {
			return finish(result, e)
		}
		if stamp != w.stamp {
			o.Outcome = plan.Skipped
			o.Reason = "content changed since preview"
			blocked[o.Entry] = true
		}
	}
	for n := range result.Operations {
		o := &result.Operations[n]
		if blocked[o.Entry] && o.Outcome == plan.Planned {
			o.Outcome = plan.Skipped
			o.Reason = "entry changed or was not approved"
		}
	}
	// Batches share an atomic container publish. Whole files and symlinks are singletons.
	batches := [][]int{}
	for _, n := range indices {
		o := result.Operations[n]
		if o.Outcome != plan.Planned {
			continue
		}
		shared := o.Kind != lock.File && o.Kind != lock.Symlink
		found := -1
		if shared {
			for k, b := range batches {
				first := result.Operations[b[0]]
				if first.Path == o.Path && (first.Action == plan.Remove) == (o.Action == plan.Remove) {
					found = k
					break
				}
			}
		}
		if found >= 0 {
			batches[found] = append(batches[found], n)
		} else {
			batches = append(batches, []int{n})
		}
	}
	// Track expected content after our own batches, while still detecting external edits.
	stamps := map[string]string{}
	for _, b := range batches {
		if err = ctx.Err(); err != nil {
			return finish(result, err)
		}
		for _, n := range b {
			w := r.findWork(result.Operations[n])
			expected := w.stamp
			if s, ok := stamps[w.item.Path]; ok {
				expected = s
			}
			actual, e := r.stamp(w.item)
			if e != nil {
				return failBatch(result, b, e)
			}
			if actual != expected {
				return failBatch(result, b, fmt.Errorf("content changed before publish at %s", w.item.Path))
			}
		}
		inlineRefs := map[string]bool{}
		for _, n := range b {
			ref := result.Operations[n].Entry
			if activeInline[ref] {
				inlineRefs[ref] = true
			}
		}
		for _, w := range r.work {
			if !inlineRefs[w.item.Entry] {
				continue
			}
			expected := w.stamp
			if s, ok := stamps[w.item.Path]; ok {
				expected = s
			}
			actual, e := r.stamp(w.item)
			if e != nil {
				return failBatch(result, b, e)
			}
			if actual != expected {
				return failBatch(result, b, fmt.Errorf("required inline item changed before publish at %s", w.item.Path))
			}
		}
		err = r.executeBatch(result, b)
		if err != nil {
			return failBatch(result, b, err)
		}
		for _, n := range b {
			result.Operations[n].Outcome = plan.Applied
		}
		if err = r.fs.save(r.owned, r.root); err != nil {
			return finish(result, fmt.Errorf("target changed but ownership save failed; retry may require --adopt: %w", err))
		}
		w := r.findWork(result.Operations[b[0]])
		stamps[w.item.Path], err = r.stamp(w.item)
		if err != nil {
			return finish(result, err)
		}
	}
	// Consent history is recorded only for complete successful inline executions.
	for ref, identity := range r.history {
		complete := true
		for _, o := range result.Operations {
			if o.Entry == ref && o.Outcome != plan.Applied && o.Outcome != plan.Unchanged {
				complete = false
			}
		}
		if !complete {
			continue
		}
		for _, w := range r.work {
			if w.item.Entry != ref || w.op.Action == plan.Remove {
				continue
			}
			if i, ok := r.owned.Find(w.item); ok {
				i.MCPIdentity = identity
				if err = r.owned.Put(i); err != nil {
					return finish(result, err)
				}
			}
		}
	}
	if len(r.owned.Items()) > 0 || len(r.initial) > 0 || len(batches) > 0 {
		if err = r.fs.save(r.owned, r.root); err != nil {
			return finish(result, err)
		}
		if err = r.fs.exclude(r.owned, r.root); err != nil {
			return finish(result, err)
		}
	}
	return result, nil
}
func (r *Reconciliation) approve(p plan.Plan) (plan.Plan, error) {
	original := r.Plan()
	normalized, err := plan.Normalize(p)
	if err != nil {
		return original, err
	}
	if len(normalized.Operations) != len(original.Operations) || !reflect.DeepEqual(normalized.Notes, original.Notes) {
		return original, fmt.Errorf("approval does not match prepared plan")
	}
	for n, o := range normalized.Operations {
		expected := original.Operations[n]
		if o.Gated && expected.Outcome == plan.Planned && o.Outcome == plan.Skipped {
			o.Outcome = expected.Outcome
			o.Reason = expected.Reason
		}
		if !reflect.DeepEqual(o, expected) {
			return original, fmt.Errorf("approval altered prepared operation")
		}
	}
	return normalized, nil
}
func (r *Reconciliation) findWork(o plan.Operation) work {
	for _, w := range r.work {
		x := w.op
		x.Outcome = o.Outcome
		x.Reason = o.Reason
		if reflect.DeepEqual(x, o) {
			return w
		}
	}
	panic("validated operation missing private payload")
}
func finish(p plan.Plan, err error) (plan.Plan, error) {
	for n := range p.Operations {
		if p.Operations[n].Outcome == plan.Planned {
			p.Operations[n].Outcome = plan.Skipped
			p.Operations[n].Reason = "execution stopped"
		}
	}
	return p, err
}
func failBatch(p plan.Plan, b []int, err error) (plan.Plan, error) {
	for _, n := range b {
		p.Operations[n].Outcome = plan.Failed
		p.Operations[n].Reason = "target operation failed"
	}
	return finish(p, err)
}

func (r *Reconciliation) executeBatch(p plan.Plan, batch []int) error {
	first := r.findWork(p.Operations[batch[0]])
	shared := first.item.Kind != lock.File && first.item.Kind != lock.Symlink
	if !shared {
		o := p.Operations[batch[0]]
		w := first
		if o.Action == plan.Remove {
			return r.owned.RemoveOwnedWithForce(r.root, w.item, r.roots, r.options.Force)
		}
		if o.Action != plan.Adopt {
			if err := lock.ValidatePath(r.root, w.item.Path, r.roots); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(filepath.Join(r.root, w.item.Path)), 0755); err != nil {
				return err
			}
			if w.item.Kind == lock.Symlink {
				if err := r.fs.link(r.root, w.item.Path, r.roots, w.item.Link); err != nil {
					return err
				}
			} else {
				// A forced/adopted symlink leaf may be unlinked, never followed.
				s, err := os.Lstat(filepath.Join(r.root, w.item.Path))
				if err == nil && s.Mode()&os.ModeSymlink != 0 {
					if err = os.Remove(filepath.Join(r.root, w.item.Path)); err != nil {
						return err
					}
				} else if err != nil && !os.IsNotExist(err) {
					return err
				}
				mode := os.FileMode(0644)
				if s != nil && s.Mode().IsRegular() {
					mode = s.Mode().Perm()
				}
				if err = r.fs.write(r.root, w.item.Path, r.roots, w.payload.Bytes, mode); err != nil {
					return err
				}
			}
		}
		return r.owned.Put(w.item)
	}
	path := first.item.Path
	if err := lock.ValidatePath(r.root, path, r.roots); err != nil {
		return err
	}
	mode := os.FileMode(0644)
	data := []byte{}
	s, err := os.Lstat(filepath.Join(r.root, path))
	exists := err == nil
	if err == nil {
		if !s.Mode().IsRegular() {
			return fmt.Errorf("shared container must be regular: %s", path)
		}
		mode = s.Mode().Perm()
		data, err = os.ReadFile(filepath.Join(r.root, path))
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	} else if first.item.Kind != lock.MDSection && first.item.Kind != lock.TOMLTable {
		data = []byte("{}")
	}
	writing := false
	for _, n := range batch {
		if first.item.Kind == lock.MDSection {
			// Markdown sections are reconciled once per file, in manifest order.
			out, e := r.reconcileBatch(path, data, p.Operations[batch[0]].Action == plan.Remove)
			if e != nil {
				return e
			}
			writing = exists && !bytes.Equal(out, data) || !exists && len(out) > 0
			data = out
			break
		}
		o := p.Operations[n]
		w := r.findWork(o)
		switch {
		case o.Action == plan.Adopt:
			continue
		case o.Action == plan.Remove:
			if w.observation.State == lock.Missing || w.item.Preserve {
				continue
			}
			switch w.item.Kind {
			case lock.MDSection:
				data, err = lock.RemoveSection(data, w.item.Marker)
			case lock.TOMLTable:
				data, err = lock.RemoveTOML(data, w.item)
			default:
				data, err = lock.RemoveJSON(data, w.item)
			}
		case w.item.Kind == lock.TOMLTable:
			data, err = lock.SetTOML(data, w.item, w.payload.Value)
		case w.item.Kind == lock.MDSection:
			data, err = lock.SetSection(data, w.item.Marker, w.payload.Bytes)
		default:
			data, err = lock.SetJSON(data, w.item, w.payload.Value)
		}
		if err != nil {
			return err
		}
		writing = true
	}
	if writing {
		if err = lock.ValidatePath(r.root, path, r.roots); err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(filepath.Join(r.root, path)), 0755); err != nil {
			return err
		}
		if err = r.fs.write(r.root, path, r.roots, data, mode); err != nil {
			return err
		}
	}
	for _, n := range batch {
		o := p.Operations[n]
		w := r.findWork(o)
		if o.Action == plan.Remove {
			r.owned.Forget(w.item)
		} else if err = r.owned.Put(w.item); err != nil {
			return err
		}
	}
	return nil
}
