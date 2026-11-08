package claudecode

import (
	"fmt"

	"github.com/fluid-movement/daypack/internal/adapters/contract"
	"github.com/fluid-movement/daypack/internal/library"
)

func agentItems(e contract.Entry) ([]contract.Item, error) {
	if err := library.ValidateName(library.Agent, e.Artifact.Name); err != nil {
		return nil, err
	}
	if e.Ref != "agent:"+e.Artifact.Name {
		return nil, fmt.Errorf("agent reference does not match artifact name")
	}
	return []contract.Item{{Target: contract.ClaudeAgentsDir, Entry: e.Ref, Kind: contract.File, Path: ".claude/agents/" + e.Artifact.Name + ".md", Bytes: append([]byte(nil), e.Content...)}}, nil
}
