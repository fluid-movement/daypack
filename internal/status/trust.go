package status

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/fluid-movement/daypack/internal/library"
)

// Saved Pi project trust, not proof of current runtime trust.
const (
	savedTrusted   = "saved-trusted"
	savedUntrusted = "saved-untrusted"
	savedUnknown   = "unknown"
)

const trustGuidance = "Pi may silently skip project .pi/mcp.json and .agents/skills, especially non-interactively; trust the project with /trust in Pi."

// trust reads <home>/.pi/agent/trust.json directly and read-only when pi is selected.
func (h *health) trust() error {
	if !slices.Contains(h.in.Project.Harnesses, "pi") {
		return nil
	}
	state, detail, err := savedTrust(h.opts.HomeDir, h.root)
	if err != nil {
		return err
	}
	c := Check{Code: "pi-trust", Harness: "pi", Severity: library.Error}
	switch state {
	case savedTrusted:
		c.Severity = library.Warning
		c.Message = "project is saved as trusted in Pi; runtime overrides are not inspected"
	case savedUntrusted:
		c.Message = "project is saved as untrusted in Pi. " + trustGuidance
	default:
		c.Message = "no saved Pi trust decision (" + detail + "). " + trustGuidance
	}
	h.add(c, "", "")
	return nil
}

func savedTrust(home, root string) (state, detail string, err error) {
	data, err := os.ReadFile(filepath.Join(home, ".pi", "agent", "trust.json"))
	if errors.Is(err, os.ErrNotExist) {
		return savedUnknown, "no trust store", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("read Pi trust store: %w", err)
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil || raw == nil {
		return savedUnknown, "trust store is malformed", nil
	}
	decisions := map[string]*bool{}
	for k, v := range raw {
		var b *bool
		if json.Unmarshal(v, &b) != nil {
			return savedUnknown, "trust store is malformed", nil
		}
		decisions[filepath.Clean(k)] = b
	}
	dir, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	if real, e := filepath.EvalSymlinks(dir); e == nil {
		dir = real
	}
	for {
		if b := decisions[dir]; b != nil {
			if *b {
				return savedTrusted, "", nil
			}
			return savedUntrusted, "", nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return savedUnknown, "no decision for this project", nil
		}
		dir = parent
	}
}
