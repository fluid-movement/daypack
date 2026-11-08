package status

import (
	"context"
	"errors"
	"sort"

	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
)

// git checks that intended-untracked managed paths are excluded and untracked.
func (h *health) git(ctx context.Context) error {
	in, err := lock.InRepository(h.root)
	if err != nil {
		return errors.New("git is unavailable or failed")
	}
	if !in {
		return nil
	}
	// Combine retained and desired rows; desired rows carry the tracking policy.
	merged := lock.New()
	for _, i := range h.in.Ownership.Items() {
		if t, ok := h.targets[i.Target]; ok && t.Contains(i.Path) {
			if err := merged.Put(i); err != nil {
				return errors.New("inconsistent ownership rows")
			}
		}
	}
	for _, i := range h.in.Desired {
		if err := merged.Put(i); err != nil {
			return errors.New("inconsistent ownership rows")
		}
	}
	items := merged.Items()
	missing, err := lock.MissingExclusions(h.root, items)
	switch {
	case errors.Is(err, lock.ErrExcludeBlock):
		h.add(Check{Code: "git-exclude-malformed", Message: "the Git exclude file has a malformed daypack block", Severity: library.Error}, "", "")
	case err != nil:
		return errors.New("could not read the Git exclude file")
	default:
		for _, p := range missing {
			h.add(Check{Code: "git-exclude-missing", Path: p, Message: "managed path is not in this project's exclude block", Severity: library.Error}, "", "")
		}
	}
	tracked := map[string]bool{}
	for _, i := range items {
		if i.Tracked {
			tracked[i.Path] = true
		}
	}
	paths := []string{}
	seen := map[string]bool{}
	for _, i := range items {
		if !tracked[i.Path] && !seen[i.Path] {
			seen[i.Path] = true
			paths = append(paths, i.Path)
		}
	}
	sort.Strings(paths)
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		t, err := lock.IsTracked(h.root, p)
		if err != nil {
			return errors.New("could not inspect Git tracking")
		}
		if t {
			h.add(Check{Code: "git-tracked-managed-path", Path: p, Message: "generated path is tracked by Git", Severity: library.Error}, "", "")
		}
	}
	return nil
}
