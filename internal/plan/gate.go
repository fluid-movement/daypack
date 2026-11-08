package plan

import (
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
	"slices"
)

// NeedsConfirmation compares raw executable identity with verified managed history.
// Missing or unverifiable history must be passed as nil by the planner.
func NeedsConfirmation(entry contract.Entry, previous *library.MCPDefinition) bool {
	if !entry.Inline || entry.Artifact.Type != library.MCP {
		return false
	}
	d := entry.Artifact.MCP
	return previous == nil || d == nil || d.Type != previous.Type || d.Command != previous.Command || d.URL != previous.URL || !slices.Equal(d.Args, previous.Args)
}

type GateMode string

const (
	None    GateMode = "none"
	Prompt  GateMode = "prompt"
	Skip    GateMode = "skip"
	Preview GateMode = "preview"
)

func candidate(o Operation) bool {
	return o.Gated && (o.Outcome == "" || o.Outcome == Planned) && (o.Action == Create || o.Action == Update || o.Action == Adopt)
}
func ConfirmationMode(p Plan, interactive, yes, dryRun bool) GateMode {
	if !slices.ContainsFunc(p.Operations, candidate) {
		return None
	}
	if dryRun {
		return Preview
	}
	if yes {
		return None
	}
	if interactive {
		return Prompt
	}
	return Skip
}
func SkipGated(p Plan, reason string) Plan {
	p = clone(p)
	for i, o := range p.Operations {
		if candidate(o) {
			p.Operations[i].Outcome = Skipped
			p.Operations[i].Reason = reason
		}
	}
	return p
}

// GatedEntries returns unique candidate names in display order.
func GatedEntries(p Plan) []string {
	names := []string{}
	for _, o := range p.Operations {
		if candidate(o) {
			names = append(names, o.Entry)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}
