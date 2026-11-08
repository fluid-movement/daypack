package apply

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/fluid-movement/daypack/internal/adapters"
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/fluid-movement/daypack/internal/plan"
)

type Options struct{ Force, Adopt bool }

// Prepare discovers and loads a project without creating any files.
func Prepare(ctx context.Context, start, libraryDir string, registry *adapters.Registry, options Options) (*Reconciliation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := manifest.FindProject(start)
	if err != nil {
		return nil, err
	}
	m, err := manifest.LoadManifest(filepath.Join(root, "daypack.toml"))
	if err != nil {
		return nil, err
	}
	lib, err := library.Load(libraryDir)
	if err != nil {
		return nil, err
	}
	return Build(ctx, root, m, lib, registry, options)
}

// Build validates and plans reconciliation. Rendering never grants ownership.
func Build(ctx context.Context, root string, m *manifest.Manifest, lib *library.Library, registry *adapters.Registry, options Options) (*Reconciliation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m == nil || lib == nil || registry == nil {
		return nil, fmt.Errorf("manifest, library and registry are required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	r := &Reconciliation{root: root, options: options, targets: map[string]contract.Target{}, entries: map[string]contract.Entry{}, blocked: map[string]bool{}, history: map[string]string{}, fs: defaultFilesystem()}
	for _, p := range append(lib.Validate(), m.ValidateForApply(lib)...) {
		if p.Severity == library.Error {
			return nil, fmt.Errorf("validation failed: %s %s", p.File, p.Field)
		}
		r.report.Notes = append(r.report.Notes, contract.Note{Code: "validation-warning", Message: p.File + ": " + p.Field})
	}
	for _, t := range registry.Targets() {
		r.targets[t.ID] = t
		r.roots = append(r.roots, t.Root)
	}
	r.owned, err = lock.Load(root)
	if err != nil {
		return nil, err
	}
	r.initial = r.owned.Items()
	project := contract.Project{Root: root}
	for _, h := range m.Harnesses {
		if _, ok := registry.Lookup(h); ok {
			project.Harnesses = append(project.Harnesses, h)
		}
	}
	for _, h := range project.Harnesses {
		a, _ := registry.Lookup(h)
		r.report.Notes = append(r.report.Notes, a.Notes(project)...)
	}
	r.project = contract.Project{Root: root, Harnesses: slices.Clone(project.Harnesses)}
	var entries []contract.Entry
	for _, e := range m.Entries() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ref := string(e.Type) + ":" + e.Name
		var artifact library.Artifact
		var content []byte
		if e.Inline == nil {
			artifact, err = lib.Resolve(ref)
			if err != nil {
				var missing *library.NotFoundError
				if !errors.As(err, &missing) {
					return nil, err
				}
				r.diagnostic(ref, "", plan.Unresolved, missing.Error())
				r.blocked[ref] = true
				continue
			}
			if e.Type == library.Agent || e.Type == library.Fragment {
				content, err = os.ReadFile(artifact.Path)
				if err != nil {
					return nil, fmt.Errorf("read %s: %w", ref, err)
				}
			}
		}
		entry := contract.FromManifest(e, artifact, content)
		r.entries[ref] = entry
		selected := []string{}
		for _, h := range e.Harnesses {
			if _, ok := registry.Lookup(h); ok {
				selected = append(selected, h)
			} else {
				r.diagnostic(ref, h, plan.Unsupported, "no registered adapter")
				r.blocked[ref] = true
			}
		}
		if len(selected) > 0 {
			entry.Harnesses = selected
			entries = append(entries, entry)
		}
	}
	result, err := registry.Desired(entries, project)
	if err != nil {
		return nil, err
	}
	for _, u := range result.Unsupported {
		r.diagnostic(u.Entry, u.Harness, plan.Unsupported, "artifact type is unsupported")
		r.blocked[u.Entry] = true
	}
	desired := lock.New()
	r.desired = desired
	for _, item := range result.Items {
		candidate, err := r.candidate(item)
		if err != nil {
			return nil, err
		}
		if err = desired.Put(candidate); err != nil {
			return nil, err
		}
		if err = r.addDesired(item, candidate); err != nil {
			return nil, err
		}
	}
	for _, old := range r.initial {
		t, known := r.targets[old.Target]
		if !known {
			r.report.Operations = append(r.report.Operations, operation(old, plan.Unsupported, plan.Skipped, "unknown target; ownership retained"))
			continue
		}
		if err = r.validateOwned(t, old); err != nil {
			return nil, err
		}
		if _, ok := desired.Find(old); ok {
			continue
		}
		if r.blocked[old.Entry] {
			r.report.Operations = append(r.report.Operations, operation(old, plan.Unresolved, plan.Skipped, "entry unavailable; previous ownership retained"))
			continue
		}
		obs, err := r.owned.Status(root, old)
		if err != nil {
			return nil, err
		}
		action, outcome, reason := plan.Remove, plan.Planned, ""
		if obs.State == lock.Drifted && !options.Force {
			action, outcome, reason = plan.Drift, plan.Skipped, "managed content changed; use --force"
		}
		if err = r.appendWork(work{item: old, observation: obs, op: operation(old, action, outcome, reason)}); err != nil {
			return nil, err
		}
	}
	if err = r.gate(); err != nil {
		return nil, err
	}
	// Check desired ownership compatibility without claiming it on disk.
	check := lock.New()
	for _, i := range r.initial {
		if _, ok := desired.Find(i); ok {
			continue
		}
		if err = check.Put(i); err != nil {
			return nil, err
		}
	}
	for _, i := range desired.Items() {
		if old, ok := r.owned.Find(i); ok && (old.Target != i.Target || old.Entry != i.Entry) {
			return nil, fmt.Errorf("ownership producer collision at %s", i.Path)
		}
		if err = check.Put(i); err != nil {
			return nil, err
		}
	}
	_, err = plan.Normalize(r.report)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Reconciliation) validateOwned(t contract.Target, i lock.Item) error {
	typ, name, ok := cutRef(i.Entry)
	if !ok || name == "" || !slices.Contains(t.Types, library.ArtifactType(typ)) || !t.Contains(i.Path) {
		return fmt.Errorf("invalid ownership for target %s at %s", t.ID, i.Path)
	}
	return lock.ValidatePath(r.root, i.Path, r.roots)
}
