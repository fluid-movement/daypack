package lock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// TOML table keys use the same escaped literal segments as JSON ownership keys.
// Payloads contain a standalone table body, not a document with its own header.
func HashTOML(data []byte) (string, error) {
	var v map[string]any
	if _, err := toml.Decode(string(data), &v); err != nil {
		return "", fmt.Errorf("invalid TOML table")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("invalid TOML table value")
	}
	return HashBytes(b), nil
}

type tableSpan struct {
	start, end int
	key        []string
	array      bool
}

// tomlStatements splits only at top-level newlines. Quoted strings (including
// multiline strings), arrays, inline tables and comments cannot introduce headers.
func tomlStatements(data []byte) [][2]int {
	var out [][2]int
	start, depth := 0, 0
	var quote byte
	multiline, escaped, comment := false, false, false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if comment {
			if c != '\n' {
				continue
			}
			comment = false
		} else if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if quote == '"' && c == '\\' {
				escaped = true
				continue
			}
			if c == quote {
				if !multiline {
					quote = 0
				} else if i+2 < len(data) && data[i+1] == quote && data[i+2] == quote {
					// Four/five quotes end a multiline string with one/two literal quotes.
					i += 2
					for i+1 < len(data) && data[i+1] == quote {
						i++
					}
					quote = 0
				}
			}
			continue
		} else {
			switch c {
			case '#':
				comment = true
			case '\'', '"':
				quote = c
				multiline = i+2 < len(data) && data[i+1] == c && data[i+2] == c
				if multiline {
					i += 2
				}
			case '[', '{':
				depth++
			case ']', '}':
				depth--
			}
		}
		if c == '\n' && depth == 0 {
			out = append(out, [2]int{start, i + 1})
			start = i + 1
		}
	}
	if start < len(data) {
		out = append(out, [2]int{start, len(data)})
	}
	return out
}

func tableSpans(data []byte) ([]tableSpan, error) {
	var spans []tableSpan
	for _, stmt := range tomlStatements(data) {
		raw := bytes.TrimSpace(data[stmt[0]:stmt[1]])
		if len(raw) == 0 || raw[0] == '#' {
			continue
		}
		if raw[0] == '[' {
			// Let the real TOML parser handle quoted/escaped segments and comments.
			var probe map[string]any
			md, err := toml.Decode(string(raw)+"\n__daypack_probe = 0\n", &probe)
			if err != nil {
				return nil, fmt.Errorf("unsafe TOML table header")
			}
			keys := md.Keys()
			if len(keys) == 0 {
				return nil, fmt.Errorf("unsafe TOML table header")
			}
			key := keys[len(keys)-1]
			if len(key) < 2 {
				return nil, fmt.Errorf("unsafe TOML table header")
			}
			spans = append(spans, tableSpan{stmt[0], stmt[1], slices.Clone([]string(key[:len(key)-1])), bytes.HasPrefix(raw, []byte("[["))})
		} else if len(spans) > 0 {
			spans[len(spans)-1].end = stmt[1]
		}
	}
	return spans, nil
}
func tablePrefix(a, b []string) bool { return len(a) <= len(b) && slices.Equal(a, b[:len(a)]) }
func tableAt(v map[string]any, parts []string) (map[string]any, bool, error) {
	for _, k := range parts {
		value, ok := v[k]
		if !ok {
			return nil, false, nil
		}
		v, ok = value.(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("TOML path crosses non-table")
		}
	}
	return v, true, nil
}
func tableHeader(parts []string) (string, error) {
	// Use the TOML encoder to quote key names correctly (including literal dots).
	var b bytes.Buffer
	nested := map[string]any{"__daypack_probe": int64(0)}
	for n := len(parts) - 1; n >= 0; n-- {
		nested = map[string]any{parts[n]: nested}
	}
	if err := toml.NewEncoder(&b).Encode(nested); err != nil {
		return "", fmt.Errorf("invalid TOML key")
	}
	lines := strings.Split(b.String(), "\n")
	for n := len(lines) - 1; n >= 0; n-- {
		if strings.HasPrefix(strings.TrimSpace(lines[n]), "[") {
			return strings.TrimSpace(lines[n]) + "\n", nil
		}
	}
	return "", fmt.Errorf("invalid TOML key")
}

// inspectTOML checks that the subtree can be edited using explicit table spans.
// Dotted assignments outside those spans, sealed inline parents and arrays of
// tables are refused; rewriting the whole config would violate byte ownership.
func inspectTOML(data []byte, key string) (map[string]any, bool, []tableSpan, error) {
	parts, err := keySegments(key)
	if err != nil {
		return nil, false, nil, err
	}
	var doc map[string]any
	if _, err = toml.Decode(string(data), &doc); err != nil {
		return nil, false, nil, fmt.Errorf("invalid TOML config")
	}
	value, exists, err := tableAt(doc, parts)
	if err != nil {
		return nil, false, nil, err
	}
	spans, err := tableSpans(data)
	if err != nil {
		return nil, false, nil, err
	}
	// Reject inline ancestor containers even if the parser accepts extensions
	// that the TOML specification forbids.
	var context []string
	for _, stmt := range tomlStatements(data) {
		raw := bytes.TrimSpace(data[stmt[0]:stmt[1]])
		if len(raw) == 0 || raw[0] == '#' {
			continue
		}
		if raw[0] == '[' {
			for _, span := range spans {
				if span.start == stmt[0] {
					context = span.key
					break
				}
			}
			continue
		}
		var standalone map[string]any
		md, e := toml.Decode(string(raw), &standalone)
		if e != nil {
			return nil, false, nil, fmt.Errorf("unsafe TOML assignment")
		}
		for _, k := range md.Keys() {
			full := append(slices.Clone(context), []string(k)...)
			if tablePrefix(full, parts) {
				return nil, false, nil, fmt.Errorf("unsafe TOML inline or dotted parent")
			}
		}
	}
	var owned []tableSpan
	for _, span := range spans {
		if span.array && (tablePrefix(parts, span.key) || tablePrefix(span.key, parts)) {
			return nil, false, nil, fmt.Errorf("unsafe TOML array-table layout")
		}
		if tablePrefix(parts, span.key) {
			owned = append(owned, span)
		}
	}
	remainder := append([]byte(nil), data...)
	for n := len(owned) - 1; n >= 0; n-- {
		s := owned[n]
		remainder = append(remainder[:s.start], remainder[s.end:]...)
	}
	var rest map[string]any
	if _, err = toml.Decode(string(remainder), &rest); err != nil {
		return nil, false, nil, fmt.Errorf("unsafe TOML table layout")
	}
	if _, remaining, e := tableAt(rest, parts); e != nil || remaining {
		return nil, false, nil, fmt.Errorf("unsafe TOML table layout; explicit tables required")
	}
	header, err := tableHeader(parts)
	if err != nil {
		return nil, false, nil, err
	}
	// This also detects inline/dotted parent tables that cannot be extended.
	if _, err = toml.Decode(string(remainder)+"\n"+header+"__daypack_probe = 0\n", &rest); err != nil {
		return nil, false, nil, fmt.Errorf("unsafe TOML table parent")
	}
	return value, exists, owned, nil
}

func observeTOML(data []byte, key string) (string, bool, error) {
	value, exists, _, err := inspectTOML(data, key)
	if err != nil || !exists {
		return "", exists, err
	}
	b, err := json.Marshal(value)
	if err != nil {
		return "", false, fmt.Errorf("invalid TOML table value")
	}
	return HashBytes(b), true, nil
}
func RemoveTOML(data []byte, i Item) ([]byte, error) {
	if i.Kind != TOMLTable {
		return nil, fmt.Errorf("not a TOML table item")
	}
	_, _, spans, err := inspectTOML(data, i.Key)
	if err != nil {
		return nil, err
	}
	out := append([]byte(nil), data...)
	for n := len(spans) - 1; n >= 0; n-- {
		s := spans[n]
		out = append(out[:s.start], out[s.end:]...)
	}
	return out, nil
}
func SetTOML(data []byte, i Item, body []byte) ([]byte, error) {
	if i.Kind != TOMLTable {
		return nil, fmt.Errorf("not a TOML table item")
	}
	if _, err := HashTOML(body); err != nil {
		return nil, err
	}
	var value map[string]any
	if _, err := toml.Decode(string(body), &value); err != nil {
		return nil, fmt.Errorf("invalid TOML table")
	}
	parts, err := keySegments(i.Key)
	if err != nil {
		return nil, err
	}
	nested := value
	for n := len(parts) - 1; n >= 0; n-- {
		nested = map[string]any{parts[n]: nested}
	}
	var encoded bytes.Buffer
	if err = toml.NewEncoder(&encoded).Encode(nested); err != nil {
		return nil, fmt.Errorf("cannot encode TOML table")
	}
	out, err := RemoveTOML(data, i)
	if err != nil {
		return nil, err
	}
	// Encoder emits empty ancestor headers. Only add the owned subtree.
	spans, err := tableSpans(encoded.Bytes())
	if err != nil {
		return nil, err
	}
	start := -1
	for _, s := range spans {
		if slices.Equal(s.key, parts) {
			start = s.start
			break
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("cannot encode TOML subtree")
	}
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	out = append(out, encoded.Bytes()[start:]...)
	var check map[string]any
	if _, err = toml.Decode(string(out), &check); err != nil {
		return nil, fmt.Errorf("unsafe TOML table edit")
	}
	return out, nil
}
