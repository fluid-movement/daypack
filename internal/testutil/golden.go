package testutil

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateGoldens = flag.Bool("update", false, "explicitly update golden expectations")

func AssertGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *updateGoldens {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Errorf("golden %s mismatch\n- want: %q\n+ got:  %q", path, want, got)
	}
}

func canonicalJSON(data []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("expected one complete JSON document (trailing data: %v)", err)
	}
	return json.MarshalIndent(value, "", "  ")
}

func AssertJSONGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	canonical, err := canonicalJSON(got)
	if err != nil {
		t.Fatal(err)
	}
	if *updateGoldens {
		AssertGolden(t, path, append(canonical, '\n'))
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := canonicalJSON(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, canonical) {
		t.Errorf("JSON golden %s mismatch\n- want: %s\n+ got: %s", path, expected, canonical)
	}
}

// NormalizeFixturePaths replaces only explicitly declared roots. Longest roots
// win; operation order and all other bytes, including secret values, survive.
func NormalizeFixturePaths(data []byte, roots ...string) []byte {
	result := string(data)
	type replacement struct{ root, token string }
	replacements := make([]replacement, 0, len(roots))
	for i, root := range roots {
		if root != "" {
			replacements = append(replacements, replacement{root, fmt.Sprintf("<ROOT%d>", i)})
		}
	}
	sort.SliceStable(replacements, func(i, j int) bool { return len(replacements[i].root) > len(replacements[j].root) })
	for _, r := range replacements {
		result = strings.ReplaceAll(result, r.root, r.token)
		result = strings.ReplaceAll(result, filepath.ToSlash(r.root), r.token)
	}
	return []byte(result)
}
