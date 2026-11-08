// Package contract contains the pure, harness-independent adapter interface.
package contract

import (
	"maps"
	"slices"

	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/lock"
	"github.com/fluid-movement/daypack/internal/manifest"
)

type ArtifactType = library.ArtifactType

type Entry struct {
	Ref      string
	Artifact library.Artifact
	// nil inherits Project.Harnesses; explicit empty selections are invalid.
	Harnesses []string
	Inline    bool
	Content   []byte
}

// FromManifest combines a parsed entry with the artifact and bytes resolved by
// core. Harness selection stays unnormalized so omission remains distinguishable.
func FromManifest(e manifest.Entry, artifact library.Artifact, content []byte) Entry {
	if e.Inline != nil {
		artifact.MCP = e.Inline
	}
	artifact.Type, artifact.Name = e.Type, e.Name
	return CloneEntry(Entry{Ref: string(e.Type) + ":" + e.Name, Artifact: artifact, Harnesses: e.Harnesses, Inline: e.Inline != nil, Content: content})
}

// CloneEntry isolates mutable source data before handing it to an adapter.
func CloneEntry(e Entry) Entry {
	e.Harnesses = slices.Clone(e.Harnesses)
	e.Content = slices.Clone(e.Content)
	if e.Artifact.MCP != nil {
		d := *e.Artifact.MCP
		d.Args = slices.Clone(d.Args)
		d.Env = maps.Clone(d.Env)
		d.Headers = maps.Clone(d.Headers)
		e.Artifact.MCP = &d
	}
	return e
}

type Project struct {
	Root      string
	Harnesses []string
}
type Adapter interface {
	Name() string
	Supports(ArtifactType) bool
	Targets() []Target
	Desired(Entry, Project) ([]Item, error)
	Notes(Project) []Note
}

// ItemKind uses the ownership package's vocabulary without adding lock state
// (tracking, hashes, or preservation decisions) to adapter output.
type ItemKind = lock.Kind

const (
	Symlink          = lock.Symlink
	File             = lock.File
	JSONEntry        = lock.JSONEntry
	JSONArrayElement = lock.JSONArrayElement
	MarkdownSection  = lock.MDSection
)

type Item struct {
	Target, Entry           string
	Kind                    ItemKind
	Path, Link, Key, Marker string
	Bytes, Value            []byte
}
type Note struct{ Harness, Code, Message string }
type Unsupported struct {
	Harness, Entry string
	Type           ArtifactType
	Fields         []string
}

// UnsupportedError reports field capabilities for an otherwise valid entry.
// It must contain only field names, never definition values.
type UnsupportedError struct{ Fields []string }

func (*UnsupportedError) Error() string { return "unsupported adapter fields" }
