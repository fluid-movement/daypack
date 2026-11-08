package apply

import (
	"encoding/json"

	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/plan"
)

func executableIdentity(d *library.MCPDefinition) string {
	if d == nil {
		return ""
	}
	projection := struct {
		Type, Command string
		Args          []string
		URL           string
	}{d.Type, d.Command, append([]string{}, d.Args...), d.URL}
	b, err := json.Marshal(projection)
	if err != nil {
		panic(err)
	}
	return lock.HashBytes(b)
}
func (r *Reconciliation) gate() error {
	for ref, e := range r.entries {
		if !e.Inline || e.Artifact.Type != library.MCP {
			continue
		}
		identity := executableIdentity(e.Artifact.MCP)
		verified, count := true, 0
		for _, w := range r.work {
			if w.item.Entry != ref || w.op.Action == plan.Remove {
				continue
			}
			count++
			old, ok := r.owned.Find(w.item)
			if !ok || old.MCPIdentity != identity {
				verified = false
				continue
			}
			obs, err := r.owned.Status(r.root, old)
			if err != nil {
				return err
			}
			if obs.State != lock.OK || !obs.Converged {
				verified = false
			}
		}
		var previous *library.MCPDefinition
		if verified && count > 0 {
			previous = e.Artifact.MCP
		}
		gated := plan.NeedsConfirmation(e, previous)
		blocked := r.blocked[ref]
		for _, w := range r.work {
			if w.item.Entry == ref && w.op.Outcome == plan.Skipped {
				blocked = true
			}
		}
		r.history[ref] = identity
		for n := range r.work {
			w := &r.work[n]
			if w.item.Entry != ref {
				continue
			}
			w.op.Gated = gated
			preview := plan.PreviewMCP(*e.Artifact.MCP)
			w.op.MCP = &preview
			if gated && w.op.Outcome == plan.Unchanged {
				w.op.Action = plan.Adopt
				w.op.Outcome = plan.Planned
				w.op.Reason = "inline executable consent required"
			}
			if blocked {
				w.op.Outcome = plan.Skipped
				w.op.Reason = "inline server has blocked items; complete server retained"
			}
		}
	}
	for _, w := range r.work {
		if w.op.Outcome != plan.Unchanged {
			r.report.Operations = append(r.report.Operations, w.op)
		}
	}
	return nil
}
