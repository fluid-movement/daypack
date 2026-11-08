package status

import "github.com/fluid-movement/daypack/internal/library"

// environment reports unset variable names for this project's MCP servers.
// Values are looked up for presence only and are never retained.
func (h *health) environment() {
	for _, e := range h.in.Entries {
		if e.Artifact.Type != library.MCP || e.Artifact.MCP == nil {
			continue
		}
		for _, name := range e.Artifact.MCP.RequiredVariables() {
			if _, set := h.opts.LookupEnv(name); !set {
				h.add(Check{Code: "unset-variable", Entry: e.Ref, Variable: name, Message: "required environment variable is not set", Severity: library.Error}, "", "")
			}
		}
	}
}
