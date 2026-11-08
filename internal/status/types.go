// Package status builds the read-only health report behind `daypack status`.
// It consumes the apply preparation snapshot and never plans or writes.
package status

import (
	"context"
	"errors"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/apply"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/plan"
)

// Input is the shared preparation snapshot, not a second data model.
type Input = apply.Inspection

// State is the aggregated condition of an entry and harness pair.
type State string

const (
	OK          State = "ok"
	Pending     State = "pending"
	Drift       State = "drift"
	Collision   State = "collision"
	Unresolved  State = "unresolved"
	Unsupported State = "unsupported"
)

// Options supplies the trusted registry and the process environment seams.
type Options struct {
	Registry  *adapters.Registry
	HomeDir   string
	LookupEnv func(string) (string, bool)
}

// EntryStatus is one selected (entry, harness) pair with its physical detail.
type EntryStatus struct {
	Entry      string           `json:"entry"`
	Harness    string           `json:"harness"`
	State      State            `json:"state"`
	Operations []plan.Operation `json:"operations"`
}

// Check is a health finding. It carries names and paths only, never values.
type Check struct {
	Code     string           `json:"code"`
	Entry    string           `json:"entry,omitempty"`
	Harness  string           `json:"harness,omitempty"`
	Path     string           `json:"path,omitempty"`
	Variable string           `json:"variable,omitempty"`
	Message  string           `json:"message"`
	Severity library.Severity `json:"severity"`
}

// TargetReaders lists who reads a used target and who else would.
type TargetReaders struct {
	Target       string   `json:"target"`
	Selected     []string `json:"selected"`
	Readers      []string `json:"readers"`
	OtherReaders []string `json:"other_readers"`
}

// Report is the typed status result.
type Report struct {
	Changes plan.Report     `json:"changes"`
	Entries []EntryStatus   `json:"entries"`
	Checks  []Check         `json:"checks"`
	Readers []TargetReaders `json:"readers"`
}

// NeedsAttention reports whether the exit code should be 3.
func (r Report) NeedsAttention() bool {
	if len(r.Changes.Operations) > 0 {
		return true
	}
	for _, e := range r.Entries {
		if e.State != OK {
			return true
		}
	}
	for _, c := range r.Checks {
		if c.Severity == library.Error {
			return true
		}
	}
	return false
}

var errInput = errors.New("status requires ownership, a registry, a home directory and an environment lookup")

func (o Options) validate(in Input) error {
	if in.Ownership == nil || o.Registry == nil || o.HomeDir == "" || o.LookupEnv == nil {
		return errInput
	}
	return nil
}

// Inspect builds the report from a prepared snapshot.
func Inspect(ctx context.Context, in Input, opts Options) (Report, error) {
	if err := opts.validate(in); err != nil {
		return Report{}, err
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	changes, err := in.Plan.Report()
	if err != nil {
		return Report{}, err
	}
	h, err := newHealth(in, opts)
	if err != nil {
		return Report{}, err
	}
	if err := h.ownership(ctx); err != nil {
		return Report{}, err
	}
	if err := h.git(ctx); err != nil {
		return Report{}, err
	}
	h.environment()
	if err := h.trust(); err != nil {
		return Report{}, err
	}
	r := Report{Changes: changes, Checks: h.checks, Entries: h.entries(), Readers: h.readers()}
	sortChecks(r.Checks)
	return r, nil
}
