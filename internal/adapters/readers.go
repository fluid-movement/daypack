package adapters

import "slices"

import "github.com/fluid-movement/daypack/internal/adapters/contract"

// Reader names are exposure information, never selectable adapter registrations.
// AGENTS.md lists native readers; configured imports/context overrides are not
// assumed. Source: the project's Pi paths and reader-matrix finding.
var readers = map[string][]string{
	contract.CodexConfigTOML:   {"codex"},
	contract.CodexAgentsDir:    {"codex"},
	contract.AgentsSkillsDir:   {"codex", "cursor", "gemini-cli", "github-copilot", "opencode", "pi"},
	contract.AgentsMD:          {"codex", "cursor", "github-copilot", "opencode", "pi"},
	contract.ClaudeMD:          {"claude-code"},
	contract.ClaudeSkillsDir:   {"claude-code"},
	contract.ClaudeMCPJSON:     {"claude-code"},
	contract.PiMCPJSON:         {"pi"},
	contract.ClaudeMCPApproval: {"claude-code"},
	contract.ClaudeAgentsDir:   {"claude-code"},
}

func Readers(targetID string) []string { return slices.Clone(readers[targetID]) }
