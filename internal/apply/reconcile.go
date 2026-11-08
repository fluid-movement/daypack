package apply

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/plan"
)

type work struct {
	item        lock.Item
	payload     contract.Item
	observation lock.Observation
	stamp       string
	op          plan.Operation
}

// Reconciliation holds trusted execution data independently from the report.
type Reconciliation struct {
	root     string
	options  Options
	targets  map[string]contract.Target
	roots    []string
	entries  map[string]contract.Entry
	blocked  map[string]bool
	history  map[string]string
	owned    *lock.Lock
	desired  *lock.Lock
	project  contract.Project
	initial  []lock.Item
	report   plan.Plan
	work     []work
	fs       filesystem
	executed bool
}

// Plan returns a deep presentation copy with no execution payload.
func (r *Reconciliation) Plan() plan.Plan {
	p, err := plan.Normalize(r.report)
	if err != nil {
		panic(err)
	}
	return p
}
func cutRef(ref string) (string, string, bool) { return strings.Cut(ref, ":") }
func (r *Reconciliation) diagnostic(ref, h string, a plan.Action, reason string) {
	r.report.Operations = append(r.report.Operations, plan.Operation{Entry: ref, Harness: h, Action: a, Outcome: plan.Skipped, Reason: reason})
}
func operation(i lock.Item, a plan.Action, o plan.Outcome, reason string) plan.Operation {
	return plan.Operation{Target: i.Target, Entry: i.Entry, Kind: i.Kind, Path: i.Path, Link: i.Link, Key: i.Key, Marker: i.Marker, Member: i.Value, Action: a, Outcome: o, Reason: reason}
}
func (r *Reconciliation) candidate(i contract.Item) (lock.Item, error) {
	v := lock.Item{Target: i.Target, Entry: i.Entry, Kind: i.Kind, Path: i.Path, Link: i.Link, Key: i.Key, Marker: i.Marker}
	var err error
	switch i.Kind {
	case lock.File, lock.MDSection:
		v.Hash = lock.HashBytes(i.Bytes)
	case lock.JSONEntry:
		v.Hash, err = lock.HashJSON(i.Value)
	case lock.JSONArrayElement:
		err = json.Unmarshal(i.Value, &v.Value)
	}
	if err != nil {
		return v, err
	}
	if err = lock.ValidatePath(r.root, v.Path, r.roots); err != nil {
		return v, err
	}
	v.Tracked, err = lock.IsTracked(r.root, v.Path)
	if err != nil {
		return v, err
	}
	if old, ok := r.owned.Find(v); ok {
		v.Preserve = old.Preserve
		v.Tracked = old.Tracked
	}
	// Committed exception: instruction sections are never git-excluded.
	v.Tracked = v.Tracked || committedSection(v)
	return v, nil
}
func (r *Reconciliation) addDesired(payload contract.Item, i lock.Item) error {
	if old, ok := r.owned.Find(i); ok {
		if err := r.validateOwned(r.targets[old.Target], old); err != nil {
			return err
		}
	}
	obs, err := r.owned.Status(r.root, i)
	if err != nil {
		return err
	}
	old, owned := r.owned.Find(i)
	if owned {
		if old.Kind != i.Kind {
			return fmt.Errorf("ownership kind changed at %s", i.Path)
		}
		prior, e := r.owned.Status(r.root, old)
		if e != nil {
			return e
		}
		if prior.State == lock.Drifted {
			obs.State = lock.Drifted
		}
	}
	i.Preserve = obs.Preserve
	a, o, reason := plan.Update, plan.Planned, ""
	switch {
	case obs.State == lock.Missing:
		a = plan.Create
	case !owned && obs.Preserve:
		a = plan.Adopt
	case !owned && !r.options.Adopt:
		a, o, reason = plan.Conflict, plan.Skipped, "unowned content; use --adopt"
	case !owned && obs.Converged:
		a = plan.Adopt
	case !owned:
		reason = "take over colliding content (--adopt)"
	case obs.State == lock.Drifted && !r.options.Force:
		a, o, reason = plan.Drift, plan.Skipped, "managed content changed; use --force"
	case obs.State == lock.Drifted:
		a, reason = plan.Update, "replace drifted managed content (--force)"
	case obs.Converged:
		o = plan.Unchanged
	}
	if i.Kind == lock.Symlink {
		if _, err := os.Stat(i.Link); err != nil {
			a, o, reason = plan.Unresolved, plan.Skipped, "symlink source is unavailable"
			r.blocked[i.Entry] = true
		}
	}
	return r.appendWork(work{item: i, payload: payload, observation: obs, op: operation(i, a, o, reason)})
}
func (r *Reconciliation) appendWork(w work) error {
	stamp, err := r.stamp(w.item)
	if err != nil {
		return err
	}
	w.stamp = stamp
	r.work = append(r.work, w)
	return nil
}

// stamp detects edits since preview without exposing content in reports.
func (r *Reconciliation) stamp(i lock.Item) (string, error) {
	if err := lock.ValidatePath(r.root, i.Path, r.roots); err != nil {
		return "", err
	}
	p := filepath.Join(r.root, i.Path)
	s, err := os.Lstat(p)
	if os.IsNotExist(err) {
		return "missing", nil
	}
	if err != nil {
		return "", err
	}
	if s.Mode()&os.ModeSymlink != 0 {
		if i.Kind != lock.Symlink && i.Kind != lock.File {
			return "", fmt.Errorf("shared container must be regular: %s", i.Path)
		}
		v, e := os.Readlink(p)
		return "link:" + v, e
	}
	if !s.Mode().IsRegular() {
		return "", fmt.Errorf("target must be a file: %s", i.Path)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return fmt.Sprint(s.Mode().Perm()) + ":" + lock.HashBytes(b), nil
}
