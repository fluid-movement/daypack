package manifest

import (
	"slices"
	"sort"

	"github.com/fluid-movement/daypack/internal/library"
)

// Catalog sources.
const (
	SourceLibrary = "library"
	SourceProject = "project"
)

// ArtifactTypes lists artifact types in catalog order.
var ArtifactTypes = []library.ArtifactType{library.Skill, library.MCP, library.Fragment, library.Agent}

// UsageEntry describes one available artifact and whether the project uses it.
type UsageEntry struct {
	Type        library.ArtifactType `json:"type"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Source      string               `json:"source"`
	Used        bool                 `json:"used"`
	Harnesses   []string             `json:"harnesses"`
}

// Catalog joins library artifacts with a project's manifest. A nil manifest
// means no project: nothing is marked and no inline servers are listed.
// Manifest references missing from the library are not listed.
func Catalog(lib *library.Library, m *Manifest) []UsageEntry {
	used := map[string][]string{}
	var inline []Entry
	if m != nil {
		for _, e := range m.Entries() {
			if e.Inline != nil {
				inline = append(inline, e)
				continue
			}
			key := string(e.Type) + ":" + e.Name
			if _, ok := used[key]; !ok {
				used[key] = e.Harnesses
			}
		}
	}
	out := []UsageEntry{}
	for _, kind := range ArtifactTypes {
		items := lib.List(kind)
		sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
		for _, a := range items {
			entry := UsageEntry{Type: kind, Name: a.Name, Description: a.Description, Source: SourceLibrary, Harnesses: []string{}}
			if h, ok := used[string(kind)+":"+a.Name]; ok {
				entry.Used, entry.Harnesses = true, slices.Clone(h)
			}
			out = append(out, entry)
		}
		if kind != library.MCP {
			continue
		}
		for _, e := range inline {
			out = append(out, UsageEntry{Type: library.MCP, Name: e.Name, Source: SourceProject, Used: true, Harnesses: slices.Clone(e.Harnesses)})
		}
	}
	return out
}
