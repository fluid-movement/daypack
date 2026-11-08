// Package importer copies user-owned harness artifacts into an existing library.
// Scanning and preflight are read-only; reports contain no executable definitions.
package importer

import (
	"github.com/fluid-movement/daypack/internal/library"
	"github.com/fluid-movement/daypack/internal/plan"
)

type Options struct {
	claudeState                                                  string
	HomeDir, ClaudeConfigDir, PiAgentDir, LibraryDir, ProjectDir string
	DryRun                                                       bool
}
type Source struct {
	Harness string `json:"harness"`
	Path    string `json:"path"`
	Key     string `json:"key,omitempty"`
}
type Item struct {
	Type         library.ArtifactType `json:"type"`
	Name         string               `json:"name"`
	Destination  string               `json:"destination,omitempty"`
	Sources      []Source             `json:"sources"`
	Action       plan.Action          `json:"action"`
	Outcome      plan.Outcome         `json:"outcome"`
	Reason       string               `json:"reason,omitempty"`
	SafeToRemove bool                 `json:"safe_to_remove"`
}
type Result struct {
	Plan  plan.Plan `json:"plan"`
	Items []Item    `json:"items"`
}
type nativeFile struct {
	directory bool
	path      string
	data      []byte
	mode      uint32
}
type candidate struct {
	artifact     library.Artifact
	sources      []Source
	files        []nativeFile
	values       map[string]string
	identity     string
	reason       string
	lossy        bool
	nativeSource string
}
