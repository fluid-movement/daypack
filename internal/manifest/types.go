package manifest

import (
	"maps"
	"slices"
	"sort"

	"github.com/fluid-movement/daypack/internal/library"
)

type Entry struct {
	Type library.ArtifactType
	Name string
	// nil inherits the manifest selection; an empty non-nil slice is invalid.
	Harnesses []string
	Inline    *library.MCPDefinition
}

type Manifest struct {
	Version                        int
	Harnesses                      []string
	Skills, MCP, Fragments, Agents []Entry
	InlineMCP                      []Entry
	path                           string
	source                         []byte
	problems                       []Problem
}

type Problem = library.Problem

// Entries returns independent copies with effective harness selections. Array
// order is retained; inline MCP tables follow the arrays in name order.
func (m *Manifest) Entries() []Entry {
	var out []Entry
	for _, group := range [][]Entry{m.Skills, m.MCP, m.Fragments, m.Agents} {
		for _, entry := range group {
			out = append(out, m.copyEntry(entry))
		}
	}
	inline := slices.Clone(m.InlineMCP)
	sort.SliceStable(inline, func(i, j int) bool { return inline[i].Name < inline[j].Name })
	for _, entry := range inline {
		out = append(out, m.copyEntry(entry))
	}
	return out
}

func (m *Manifest) copyEntry(e Entry) Entry {
	if e.Harnesses == nil {
		e.Harnesses = slices.Clone(m.Harnesses)
	} else {
		e.Harnesses = slices.Clone(e.Harnesses)
	}
	if e.Inline != nil {
		d := *e.Inline
		d.Args = slices.Clone(d.Args)
		d.Env = maps.Clone(d.Env)
		d.Headers = maps.Clone(d.Headers)
		e.Inline = &d
	}
	return e
}

var arrayKinds = []struct {
	key  string
	kind library.ArtifactType
}{
	{"skills", library.Skill}, {"mcp", library.MCP}, {"fragments", library.Fragment}, {"agents", library.Agent},
}
