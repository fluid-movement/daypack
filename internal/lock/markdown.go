package lock

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

func validateMarker(m string) error {
	if m == "" || strings.ContainsAny(m, "\r\n<> \t") || strings.Contains(m, "--") {
		return fmt.Errorf("invalid marker name")
	}
	return nil
}

var markers = regexp.MustCompile(`<!-- daypack:(begin|end) ([^\r\n]*?) -->`)

// span locates one complete marker pair: begin marker, body and end marker.
type span struct {
	name                           string
	begin, bodyStart, bodyEnd, end int
}

// parseSpans validates every daypack marker in the document and returns the
// sections in file order. Duplicate, nested, unmatched or unclosed markers fail.
func parseSpans(data []byte) ([]span, error) {
	open := ""
	seen := map[string]bool{}
	var spans []span
	var cur span
	for _, m := range markers.FindAllSubmatchIndex(data, -1) {
		kind := string(data[m[2]:m[3]])
		n := string(data[m[4]:m[5]])
		if err := validateMarker(n); err != nil {
			return nil, err
		}
		if kind == "begin" {
			if open != "" || seen[n] {
				return nil, fmt.Errorf("nested or duplicate markers")
			}
			open = n
			seen[n] = true
			cur = span{name: n, begin: m[0], bodyStart: m[1]}
		} else {
			if open != n {
				return nil, fmt.Errorf("unmatched markers")
			}
			open = ""
			cur.bodyEnd, cur.end = m[0], m[1]
			spans = append(spans, cur)
		}
	}
	if open != "" {
		return nil, fmt.Errorf("unclosed marker")
	}
	return spans, nil
}

func section(data []byte, name string) (int, int, bool, error) {
	if err := validateMarker(name); err != nil {
		return 0, 0, false, err
	}
	spans, err := parseSpans(data)
	if err != nil {
		return 0, 0, false, err
	}
	for _, s := range spans {
		if s.name == name {
			return s.bodyStart, s.bodyEnd, true, nil
		}
	}
	return 0, 0, false, nil
}

func SetSection(data []byte, name string, body []byte) ([]byte, error) {
	a, b, found, err := section(data, name)
	if err != nil {
		return nil, err
	}
	if bytes.Contains(body, []byte("<!-- daypack:")) {
		return nil, fmt.Errorf("body contains markers")
	}
	if found {
		out := append([]byte(nil), data[:a]...)
		out = append(out, body...)
		return append(out, data[b:]...), nil
	}
	out := append([]byte(nil), data...)
	out = append(out, []byte("<!-- daypack:begin "+name+" -->")...)
	out = append(out, body...)
	out = append(out, []byte("<!-- daypack:end "+name+" -->")...)
	return out, nil
}
func RemoveSection(data []byte, name string) ([]byte, error) {
	a, b, found, err := section(data, name)
	if err != nil {
		return nil, err
	}
	if !found {
		return append([]byte(nil), data...), nil
	}
	a -= len("<!-- daypack:begin " + name + " -->")
	b += len("<!-- daypack:end " + name + " -->")
	return append(append([]byte(nil), data[:a]...), data[b:]...), nil
}
