package lock

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func HashBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func HashJSON(b []byte) (string, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return "", err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return "", fmt.Errorf("multiple or malformed JSON documents")
	}
	c, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return HashBytes(c), nil
}

// Keys are dot-separated literal object names; backslash escapes a dot or backslash.
// Query operators are always escaped before passing names to gjson/sjson.
func keySegments(key string) ([]string, error) {
	var parts []string
	var b strings.Builder
	escaped := false
	for _, r := range key {
		if escaped {
			if r != '.' && r != '\\' {
				return nil, fmt.Errorf("invalid key escape")
			}
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '.' {
			if b.Len() == 0 {
				return nil, fmt.Errorf("empty key segment")
			}
			parts = append(parts, b.String())
			b.Reset()
		} else {
			b.WriteRune(r)
		}
	}
	if escaped || b.Len() == 0 {
		return nil, fmt.Errorf("invalid key")
	}
	return append(parts, b.String()), nil
}
func query(parts []string) string {
	out := make([]string, len(parts))
	for n, s := range parts {
		var b strings.Builder
		for _, r := range s {
			if strings.ContainsRune("\\.*?#|!@[]:{}()", r) {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
		out[n] = b.String()
	}
	return strings.Join(out, ".")
}

// Validate object structure and duplicate names before any path lookup/edit.
func jsonToken(d *json.Decoder) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			t, e := d.Token()
			if e != nil {
				return e
			}
			k, ok := t.(string)
			if !ok || seen[k] {
				return fmt.Errorf("duplicate or invalid object key")
			}
			seen[k] = true
			if e = jsonToken(d); e != nil {
				return e
			}
		}
	case '[':
		for d.More() {
			if e := jsonToken(d); e != nil {
				return e
			}
		}
	default:
		return fmt.Errorf("invalid JSON delimiter")
	}
	_, err = d.Token()
	return err
}
func jsonValue(data []byte, key string) (gjson.Result, []string, error) {
	if !gjson.ValidBytes(data) {
		return gjson.Result{}, nil, fmt.Errorf("invalid JSON")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if err := jsonToken(d); err != nil {
		return gjson.Result{}, nil, err
	}
	parts, err := keySegments(key)
	if err != nil {
		return gjson.Result{}, nil, err
	}
	v := gjson.ParseBytes(data)
	for n := range parts {
		if !v.IsObject() {
			return gjson.Result{}, nil, fmt.Errorf("JSON path crosses non-object")
		}
		v = gjson.GetBytes(data, query(parts[:n+1]))
		if !v.Exists() {
			return v, parts, nil
		}
	}
	return v, parts, nil
}
func arrayIndex(v gjson.Result, value string) (int, error) {
	if !v.Exists() {
		return -1, nil
	}
	if !v.IsArray() {
		return -1, fmt.Errorf("expected string array")
	}
	idx := -1
	for n, e := range v.Array() {
		if e.Type != gjson.String {
			return -1, fmt.Errorf("expected string array")
		}
		if e.Str == value {
			if idx >= 0 {
				return -1, fmt.Errorf("duplicate array membership")
			}
			idx = n
		}
	}
	return idx, nil
}

// insertObject preserves the existing object's indentation and closing whitespace.
func insertObject(data []byte, parts []string, value []byte) ([]byte, error) {
	parent := gjson.ParseBytes(data)
	if len(parts) > 1 {
		parent = gjson.GetBytes(data, query(parts[:len(parts)-1]))
	}
	if !parent.Exists() {
		key, err := json.Marshal(parts[len(parts)-1])
		if err != nil {
			return nil, err
		}
		nested := append(append(append([]byte{'{'}, key...), ':'), value...)
		nested = append(nested, '}')
		return insertObject(data, parts[:len(parts)-1], nested)
	}
	if !parent.IsObject() {
		return nil, fmt.Errorf("parent is not an object")
	}
	start := parent.Index
	raw := []byte(parent.Raw)
	end := bytes.LastIndexByte(raw, '}')
	pos := end
	for pos > 0 && strings.ContainsRune(" \t\r\n", rune(raw[pos-1])) {
		pos--
	}
	empty := len(parent.Map()) == 0
	key, err := json.Marshal(parts[len(parts)-1])
	if err != nil {
		return nil, err
	}
	sep := ":"
	prefix := ""
	multiline := bytes.Contains(raw, []byte{'\n'})
	if multiline {
		indent := "  "
		lines := strings.Split(string(raw), "\n")
		for _, line := range lines[1:] {
			trim := strings.TrimLeft(line, " \t")
			if strings.HasPrefix(trim, "\"") {
				indent = line[:len(line)-len(trim)]
				break
			}
		}
		if empty {
			last := lines[len(lines)-1]
			indent = last[:len(last)-len(strings.TrimLeft(last, " \t"))] + indent
		}
		newline := "\n"
		if bytes.Contains(raw, []byte("\r\n")) {
			newline = "\r\n"
		}
		prefix = newline + indent
		sep = ": "
	} else if !empty {
		prefix = " "
		if bytes.Contains(raw, []byte(": ")) {
			sep = ": "
		}
	}
	var addition []byte
	if !empty {
		addition = append(addition, ',')
	}
	addition = append(addition, prefix...)
	addition = append(addition, key...)
	addition = append(addition, sep...)
	addition = append(addition, value...)
	out := append([]byte(nil), data[:start+pos]...)
	out = append(out, addition...)
	out = append(out, data[start+pos:]...)
	return out, nil
}
func SetJSON(data []byte, i Item, value []byte) ([]byte, error) {
	v, parts, err := jsonValue(data, i.Key)
	if err != nil {
		return nil, err
	}
	if i.Kind == JSONArrayElement {
		idx, e := arrayIndex(v, i.Value)
		if e != nil {
			return nil, e
		}
		if idx >= 0 {
			return append([]byte(nil), data...), nil
		}
		b, err := json.Marshal(i.Value)
		if err != nil {
			return nil, err
		}
		if !v.Exists() {
			return insertObject(data, parts, append(append([]byte{'['}, b...), ']'))
		}
		return sjson.SetRawBytes(data, query(parts)+".-1", b)
	}
	if i.Kind != JSONEntry {
		return nil, fmt.Errorf("not a JSON item")
	}
	if _, err := HashJSON(value); err != nil {
		return nil, err
	}
	if v.Exists() {
		return sjson.SetRawBytes(data, query(parts), value)
	}
	return insertObject(data, parts, value)
}
func RemoveJSON(data []byte, i Item) ([]byte, error) {
	v, parts, err := jsonValue(data, i.Key)
	if err != nil {
		return nil, err
	}
	p := query(parts)
	if i.Kind == JSONArrayElement {
		idx, e := arrayIndex(v, i.Value)
		if e != nil {
			return nil, e
		}
		if idx < 0 || i.Preserve {
			return append([]byte(nil), data...), nil
		}
		p += "." + strconv.Itoa(idx)
	} else if i.Kind != JSONEntry {
		return nil, fmt.Errorf("not a JSON item")
	}
	return sjson.DeleteBytes(data, p)
}
