package library

import (
	"bytes"
	"errors"
	"strings"

	"gopkg.in/yaml.v3"
)

// FragmentBody returns the instruction text of a fragment file: every byte
// after an optional leading YAML frontmatter block. Without frontmatter the
// input is returned unchanged (CRLF and a missing final newline survive).
// Malformed or unterminated frontmatter is an error.
func FragmentBody(data []byte) ([]byte, error) {
	first, rest, _ := bytes.Cut(data, []byte("\n"))
	if strings.TrimSpace(string(first)) != "---" {
		return append([]byte(nil), data...), nil
	}
	offset := len(first) + 1
	meta := []byte{}
	for rest != nil {
		var line []byte
		var more bool
		line, rest, more = bytes.Cut(rest, []byte("\n"))
		offset += len(line)
		if more {
			offset++
		} else {
			rest = nil
		}
		if strings.TrimSpace(string(line)) == "---" {
			var parsed nativeMetadata
			if err := yaml.Unmarshal(meta, &parsed); err != nil {
				return nil, errors.New("invalid YAML frontmatter")
			}
			return append([]byte(nil), data[min(offset, len(data)):]...), nil
		}
		meta = append(append(meta, line...), '\n')
	}
	return nil, errors.New("unterminated YAML frontmatter")
}
