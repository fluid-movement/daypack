package apply

import (
	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/manifest"
	"github.com/fluid-movement/daypack/internal/plan"
)

// skipInlineForAuto reports whether unattended apply must leave an entry alone.
// Inline MCP servers come from the committed manifest and are never trusted
// unattended, regardless of consent history, a terminal or --yes.
func skipInlineForAuto(options Options, e manifest.Entry) bool {
	return options.Auto && e.Inline != nil
}

func (r *Reconciliation) skipInline(ref string, skipped *bool) {
	r.diagnostic(ref, "", plan.Unsupported, "inline MCP server skipped during auto-apply")
	r.blocked[ref] = true
	if !*skipped {
		*skipped = true
		r.report.Notes = append(r.report.Notes, contract.Note{Code: "auto-inline-skipped", Message: "Inline MCP servers skipped during auto-apply; run 'daypack apply'."})
	}
}
