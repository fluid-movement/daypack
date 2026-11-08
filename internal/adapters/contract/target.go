package contract

import (
	"fmt"
	"path"
	"strings"
)

type RootMode string

const (
	DirectoryRoot RootMode = "directory"
	FileRoot      RootMode = "file"
)

type Target struct {
	ID, Root string
	Mode     RootMode
	Types    []ArtifactType
}

const (
	ClaudeSkillsDir   = "claude-skills-dir"
	AgentsSkillsDir   = "agents-skills-dir"
	AgentsMD          = "agents-md"
	ClaudeMD          = "claude-md"
	ClaudeMCPJSON     = "claude-mcp-json"
	PiMCPJSON         = "pi-mcp-json"
	ClaudeMCPApproval = "claude-mcp-approval"
	ClaudeAgentsDir   = "claude-agents-dir"
)

// ValidatePath is lexical only. Core must also check filesystem symlinks before
// accessing these paths; serialized lock rows are never a trusted root catalog.
func ValidatePath(p string) error {
	if p == "" || path.IsAbs(p) || strings.ContainsAny(p, "\\\x00:\r\n") {
		return fmt.Errorf("invalid project-relative path")
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return fmt.Errorf("path traversal")
		}
	}
	if path.Clean(p) != p || p == "." {
		return fmt.Errorf("noncanonical project-relative path")
	}
	return nil
}
func (t Target) Contains(p string) bool {
	if ValidatePath(p) != nil {
		return false
	}
	if t.Mode == FileRoot {
		return p == t.Root
	}
	return t.Mode == DirectoryRoot && strings.HasPrefix(p, t.Root+"/")
}
