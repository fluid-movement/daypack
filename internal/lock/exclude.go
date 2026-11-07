package lock

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func absoluteRoot(root string) (string, error) {
	r, e := filepath.Abs(root)
	if e != nil {
		return "", e
	}
	if strings.ContainsAny(r, "\r\n") {
		return "", fmt.Errorf("newline in root")
	}
	return r, nil
}
func escapeIgnore(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune("\\*?[]!# ", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
func ExcludeBlock(root string, items []Item) (string, error) {
	r, e := absoluteRoot(root)
	if e != nil {
		return "", e
	}
	paths := map[string]bool{".daypack/": true}
	tracked := map[string]bool{}
	for _, i := range items {
		if e := validateItem(i); e != nil {
			return "", e
		}
		if i.Tracked {
			tracked[i.Path] = true
		} else {
			paths[i.Path] = true
		}
	}
	var sorted []string
	for p := range paths {
		if !tracked[p] {
			sorted = append(sorted, p)
		}
	}
	sort.Strings(sorted)
	var b strings.Builder
	b.WriteString("# daypack begin " + r + "\n")
	for _, p := range sorted {
		b.WriteString("/" + escapeIgnore(p) + "\n")
	}
	b.WriteString("# daypack end " + r + "\n")
	return b.String(), nil
}

type Exclusion struct {
	Root  string
	Stale bool
}
type excludeSpan struct {
	start, end int
	root       string
}

func excludeSpans(data []byte) ([]excludeSpan, error) {
	var spans []excludeSpan
	open := ""
	start, offset := 0, 0
	seen := map[string]bool{}
	for _, line := range bytes.SplitAfter(data, []byte{'\n'}) {
		s := strings.TrimSuffix(string(line), "\n")
		if strings.HasPrefix(s, "# daypack begin ") {
			if open != "" {
				return nil, fmt.Errorf("nested exclude block")
			}
			open = strings.TrimPrefix(s, "# daypack begin ")
			if seen[open] {
				return nil, fmt.Errorf("duplicate exclude block")
			}
			seen[open] = true
			start = offset
		} else if strings.HasPrefix(s, "# daypack end ") {
			if open == "" || strings.TrimPrefix(s, "# daypack end ") != open {
				return nil, fmt.Errorf("unmatched exclude block")
			}
			spans = append(spans, excludeSpan{start, offset + len(line), open})
			open = ""
		}
		offset += len(line)
	}
	if open != "" {
		return nil, fmt.Errorf("unclosed exclude block")
	}
	return spans, nil
}
func InspectExclude(data []byte) ([]Exclusion, error) {
	spans, e := excludeSpans(data)
	if e != nil {
		return nil, e
	}
	var out []Exclusion
	for _, s := range spans {
		_, e := os.Stat(s.root)
		if e != nil && !os.IsNotExist(e) {
			return nil, e
		}
		out = append(out, Exclusion{s.root, os.IsNotExist(e)})
	}
	return out, nil
}
func excludePath(root string) (string, error) {
	ok, e := inGit(root)
	if e != nil || !ok {
		return "", e
	}
	out, e := exec.Command("git", "-C", root, "rev-parse", "--git-path", "info/exclude").Output()
	if e != nil {
		return "", e
	}
	p := strings.TrimSuffix(string(out), "\n")
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	return p, nil
}
func UpdateExclude(root string, items []Item) error {
	block, e := ExcludeBlock(root, items)
	if e != nil {
		return e
	}
	return updateExclude(root, block)
}
func RemoveExclude(root string) error { return updateExclude(root, "") }
func updateExclude(root, block string) error {
	r, e := absoluteRoot(root)
	if e != nil {
		return e
	}
	p, e := excludePath(root)
	if e != nil || p == "" {
		return e
	}
	data, e := os.ReadFile(p)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	spans, e := excludeSpans(data)
	if e != nil {
		return e
	}
	out := append([]byte(nil), data...)
	replaced := false
	for _, s := range spans {
		if s.root == r {
			out = append(append(append([]byte(nil), data[:s.start]...), []byte(block)...), data[s.end:]...)
			replaced = true
			break
		}
	}
	if !replaced && block != "" {
		if len(out) > 0 && out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
		out = append(out, block...)
	}
	if bytes.Equal(out, data) {
		return nil
	}
	if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
		return e
	}
	return writeAtomic(p, out, 0644)
}
