package manifest

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"github.com/BurntSushi/toml"
	"github.com/fluid-movement/daypack/internal/library"
)

// LoadManifest reads exactly path. Structural errors are returned here; semantic
// diagnostics (including inline MCP validation) are returned by Validate.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	return parse(data, path)
}

func parse(data []byte, path string) (*Manifest, error) {
	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		// TOML errors can include source lines containing credentials.
		return nil, fmt.Errorf("%s: invalid manifest TOML syntax", path)
	}
	allowed := map[string]bool{"version": true, "harnesses": true, "skills": true, "mcp": true, "fragments": true, "agents": true, "inline": true}
	for _, key := range sortedKeys(raw) {
		if !allowed[key] {
			return nil, fmt.Errorf("unknown manifest key %s", key)
		}
	}
	version, ok := raw["version"].(int64)
	if !ok {
		return nil, fmt.Errorf("version must be an integer (version = 1 is required)")
	}
	if version != 1 {
		return nil, fmt.Errorf("unsupported manifest version %d", version)
	}
	m := &Manifest{Version: 1, path: path, source: bytes.Clone(data)}
	var err error
	m.Harnesses, err = stringsField(raw["harnesses"], "harnesses")
	if err != nil {
		return nil, err
	}
	groups := []*[]Entry{&m.Skills, &m.MCP, &m.Fragments, &m.Agents}
	for i, group := range arrayKinds {
		value, exists := raw[group.key]
		if !exists {
			continue
		}
		list, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("%s must be an array", group.key)
		}
		for index, value := range list {
			field := fmt.Sprintf("%s[%d]", group.key, index)
			entry := Entry{Type: group.kind}
			switch v := value.(type) {
			case string:
				entry.Name = v
			case map[string]any:
				for _, key := range sortedKeys(v) {
					if key != "name" && key != "harnesses" {
						return nil, fmt.Errorf("unknown manifest key %s.%s", field, key)
					}
				}
				var ok bool
				entry.Name, ok = v["name"].(string)
				if !ok {
					return nil, fmt.Errorf("%s.name must be a string", field)
				}
				if h, exists := v["harnesses"]; exists {
					entry.Harnesses, err = stringsField(h, field+".harnesses")
					if err != nil {
						return nil, err
					}
				}
			default:
				return nil, fmt.Errorf("%s must be a string or entry table", field)
			}
			*groups[i] = append(*groups[i], entry)
		}
	}
	if value, exists := raw["inline"]; exists {
		inline, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("inline must be a table")
		}
		for _, key := range sortedKeys(inline) {
			if key != "mcp" {
				return nil, fmt.Errorf("unknown manifest key inline.%s", key)
			}
		}
		if value, exists := inline["mcp"]; exists {
			servers, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("inline.mcp must be a table")
			}
			for _, name := range sortedKeys(servers) {
				payload, ok := servers[name].(map[string]any)
				if !ok {
					return nil, fmt.Errorf("inline.mcp.%s must be a table", name)
				}
				e := Entry{Type: library.MCP, Name: name}
				if value, exists := payload["harnesses"]; exists {
					e.Harnesses, err = stringsField(value, "inline.mcp."+name+".harnesses")
					if err != nil {
						return nil, err
					}
					delete(payload, "harnesses")
				}
				var buf bytes.Buffer
				if err := toml.NewEncoder(&buf).Encode(payload); err != nil {
					return nil, fmt.Errorf("cannot decode inline.mcp.%s", name)
				}
				definition, problems := library.ParseMCP(buf.Bytes(), path)
				e.Inline = &definition
				for _, p := range problems {
					p.Field = "inline.mcp." + name + "." + p.Field
					m.problems = append(m.problems, p)
				}
				m.InlineMCP = append(m.InlineMCP, e)
			}
		}
	}
	return m, nil
}

func stringsField(value any, field string) ([]string, error) {
	list, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", field)
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%s must be an array of strings", field)
		}
		out = append(out, s)
	}
	return out, nil
}
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
