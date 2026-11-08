package manifest

import (
	"errors"
	"fmt"
	"regexp"
	"sort"

	"github.com/fluid-movement/daypack/internal/library"
)

var namePattern = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

// Validate resolves references without executing commands or expanding secrets.
func (m *Manifest) Validate(lib *library.Library) []Problem {
	out := append([]Problem{}, m.problems...)
	add := func(field, message string) {
		out = append(out, Problem{Severity: library.Error, File: m.path, Field: field, Message: message})
	}
	selected := map[string]bool{}
	checkHarnesses := func(harnesses []string, field string, restriction bool) {
		if len(harnesses) == 0 {
			add(field, "harness selection must not be empty")
		}
		seen := map[string]bool{}
		for _, h := range harnesses {
			if h != "claude-code" && h != "pi" {
				add(field, fmt.Sprintf("unknown harness %q", h))
			}
			if seen[h] {
				add(field, fmt.Sprintf("duplicate harness %q", h))
			}
			seen[h] = true
			if restriction && !selected[h] {
				add(field, fmt.Sprintf("harness %q is not selected in the manifest", h))
			}
		}
	}
	if m.Version != 1 {
		add("version", "unsupported manifest version")
	}
	checkHarnesses(m.Harnesses, "harnesses", false)
	for _, h := range m.Harnesses {
		selected[h] = true
	}
	libraryMCP := map[string]bool{}
	if lib != nil {
		for _, a := range lib.List(library.MCP) {
			libraryMCP[a.Name] = true
		}
	}
	seen := map[string]bool{}
	for _, e := range m.Entries() {
		ref := string(e.Type) + ":" + e.Name
		field := ref
		if e.Inline != nil {
			field = "inline.mcp." + e.Name
		}
		if !namePattern.MatchString(e.Name) {
			add(field, "name must contain 1-64 lowercase letters, digits or hyphens")
			continue
		}
		if seen[ref] {
			add(field, "duplicate artifact reference "+ref)
		}
		seen[ref] = true
		checkHarnesses(e.Harnesses, field+".harnesses", true)
		if e.Inline != nil {
			for _, p := range library.ValidateMCP(*e.Inline, m.path) {
				p.Field = field + "." + p.Field
				found := false
				for _, existing := range out {
					if existing == p {
						found = true
						break
					}
				}
				if !found {
					out = append(out, p)
				}
			}
			if libraryMCP[e.Name] {
				add(field, "inline MCP name collides with library MCP "+e.Name)
			}
			continue
		}
		if lib == nil {
			add(field, "cannot resolve "+ref+": library is required")
			continue
		}
		if _, err := lib.Resolve(ref); err != nil {
			var invalid *library.ValidationError
			if errors.As(err, &invalid) {
				add(field, ref+" has library validation errors")
				for _, p := range invalid.Problems {
					p.Field = field + "." + p.Field
					out = append(out, p)
				}
			} else {
				add(field, err.Error())
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Field != b.Field {
			return a.Field < b.Field
		}
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		return a.Message < b.Message
	})
	return out
}
