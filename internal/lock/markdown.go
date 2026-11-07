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

func section(data []byte, name string) (int, int, bool, error) {
	if err := validateMarker(name); err != nil {
		return 0, 0, false, err
	}
	open := ""
	seen := map[string]bool{}
	start, end := 0, 0
	found := false
	for _, m := range markers.FindAllSubmatchIndex(data, -1) {
		kind := string(data[m[2]:m[3]])
		n := string(data[m[4]:m[5]])
		if err := validateMarker(n); err != nil {
			return 0, 0, false, err
		}
		if kind == "begin" {
			if open != "" || seen[n] {
				return 0, 0, false, fmt.Errorf("nested or duplicate markers")
			}
			open = n
			seen[n] = true
			if n == name {
				start = m[1]
			}
		} else {
			if open != n {
				return 0, 0, false, fmt.Errorf("unmatched markers")
			}
			open = ""
			if n == name {
				end = m[0]
				found = true
			}
		}
	}
	if open != "" {
		return 0, 0, false, fmt.Errorf("unclosed marker")
	}
	return start, end, found, nil
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
