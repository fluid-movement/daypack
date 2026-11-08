package secrets

import (
	"fmt"
	"strings"
)

// Reference is a variable reference and its byte span in the original text.
// A nil Default means required; a pointer to "" is an explicit empty default.
type Reference struct {
	Name       string
	Default    *string
	Start, End int
}

// ValidName accepts exactly [A-Za-z_][A-Za-z0-9_]*.
func ValidName(name string) bool {
	if name == "" {
		return false
	}
	for i := range len(name) {
		c := name[i]
		valid := c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || i > 0 && c >= '0' && c <= '9'
		if !valid {
			return false
		}
	}
	return true
}

// ParseReferences parses ${NAME} and ${NAME:-default}, without expansion.
// Errors contain locations and grammar only, never the input or default value.
func ParseReferences(text string) ([]Reference, error) {
	refs := []Reference{}
	for offset := 0; offset < len(text); {
		n := strings.Index(text[offset:], "${")
		if n < 0 {
			break
		}
		start := offset + n
		end := strings.IndexByte(text[start+2:], '}')
		if end < 0 {
			return nil, fmt.Errorf("unclosed variable reference at byte %d", start)
		}
		end += start + 2
		body := text[start+2 : end]
		name, fallback, hasDefault := strings.Cut(body, ":-")
		if !ValidName(name) || strings.ContainsAny(body, "{}") {
			return nil, fmt.Errorf("invalid variable reference at byte %d; use ${VAR} or ${VAR:-default}", start)
		}
		ref := Reference{Name: name, Start: start, End: end + 1}
		if hasDefault {
			ref.Default = &fallback
		}
		refs = append(refs, ref)
		offset = end + 1
	}
	return refs, nil
}

// EnvReference returns a reference for a validated environment variable name.
func EnvReference(name string) (string, error) {
	if !ValidName(name) {
		return "", fmt.Errorf("invalid environment variable name")
	}
	return "${" + name + "}", nil
}
