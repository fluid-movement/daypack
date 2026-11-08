// Package skilldir renders pure directory links for skill targets.
package skilldir

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
)

// AgentsTarget returns an independent declaration of the shared skills root.
func AgentsTarget() contract.Target {
	return contract.Target{ID: contract.AgentsSkillsDir, Root: ".agents/skills", Mode: contract.DirectoryRoot, Types: []contract.ArtifactType{library.Skill}}
}

// Desired links the entire skill directory without accessing the filesystem.
func Desired(e contract.Entry, target contract.Target) ([]contract.Item, error) {
	if e.Artifact.Type != library.Skill {
		return nil, nil
	}
	if err := library.ValidateName(library.Skill, e.Artifact.Name); err != nil {
		return nil, fmt.Errorf("invalid skill name")
	}
	if e.Ref != "skill:"+e.Artifact.Name {
		return nil, fmt.Errorf("invalid skill reference")
	}
	if !filepath.IsAbs(e.Artifact.Path) || strings.ContainsAny(e.Artifact.Path, "\x00\r\n") {
		return nil, fmt.Errorf("invalid skill source path")
	}
	if target.ID == "" || contract.ValidatePath(target.Root) != nil || target.Mode != contract.DirectoryRoot || !slices.Contains(target.Types, library.Skill) {
		return nil, fmt.Errorf("invalid skill directory target")
	}
	return []contract.Item{{Target: target.ID, Entry: e.Ref, Kind: contract.Symlink, Path: path.Join(target.Root, e.Artifact.Name), Link: e.Artifact.Path}}, nil
}
