// Package instructions renders instruction fragments as managed sections of
// the shared AGENTS.md file. It is pure: no filesystem access.
package instructions

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
)

// File is the single instruction file every harness reads (Claude Code via
// the CLAUDE.md import bridge).
const File = "AGENTS.md"

// Target returns an independent declaration of the shared AGENTS.md target.
func Target() contract.Target {
	return contract.Target{ID: contract.AgentsMD, Root: File, Mode: contract.FileRoot, Types: []contract.ArtifactType{library.Fragment}}
}

// FragmentItem renders one fragment as a markdown section named after its
// reference. The body bytes are kept verbatim; only owned LF framing is added
// so the marker comments sit on their own lines.
func FragmentItem(e contract.Entry) (contract.Item, error) {
	if e.Artifact.Type != library.Fragment || library.ValidateName(library.Fragment, e.Artifact.Name) != nil || e.Ref != "fragment:"+e.Artifact.Name {
		return contract.Item{}, errors.New("invalid fragment entry")
	}
	body, err := library.FragmentBody(e.Content)
	if err != nil {
		return contract.Item{}, fmt.Errorf("fragment %s: %w", e.Artifact.Name, err)
	}
	if bytes.Contains(body, []byte("<!-- daypack:")) {
		return contract.Item{}, fmt.Errorf("fragment %s contains daypack markers", e.Artifact.Name)
	}
	payload := append([]byte("\n"), body...)
	if !bytes.HasSuffix(payload, []byte("\n")) {
		payload = append(payload, '\n')
	}
	return contract.Item{Target: contract.AgentsMD, Entry: e.Ref, Kind: contract.MarkdownSection, Path: File, Marker: e.Ref, Bytes: payload}, nil
}
