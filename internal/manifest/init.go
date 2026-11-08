package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ErrExists is wrapped when init would overwrite an existing daypack.toml.
var ErrExists = errors.New("daypack.toml already exists")

// InitOptions selects the harnesses and an optional manifest to copy.
type InitOptions struct {
	Harnesses []string
	From      string
}

// NormalizeHarnesses validates a harness selection and returns it in canonical
// order. An empty selection means both harnesses.
func NormalizeHarnesses(selected []string) ([]string, error) {
	for _, h := range selected {
		if !slices.Contains(knownHarnesses, h) {
			return nil, fmt.Errorf("unknown harness %q (expected claude-code or pi)", h)
		}
	}
	if len(selected) == 0 {
		return slices.Clone(knownHarnesses), nil
	}
	var out []string
	for _, h := range knownHarnesses {
		if slices.Contains(selected, h) {
			out = append(out, h)
		}
	}
	return out, nil
}

const skeletonTail = `
# Library references, by name. A table restricts an entry to some harnesses:
#   { name = "context7", harnesses = ["claude-code"] }
# Add entries with 'daypack add <type>:<name>' or edit this file by hand.
# skills = ["name"]
# mcp = ["name"]
# fragments = []
# agents = []

# Run 'daypack apply' to write these into the project.
`

func skeleton(harnesses []string) []byte {
	q := make([]string, len(harnesses))
	for i, h := range harnesses {
		q[i] = strconv.Quote(h)
	}
	return []byte("# daypack manifest: the AI configuration this project uses.\nversion = 1\nharnesses = [" + strings.Join(q, ", ") + "]\n" + skeletonTail)
}

// Init creates dir/daypack.toml, never overwriting. Only dir itself is checked
// for an existing manifest, so nested packages can have their own.
func Init(dir string, opts InitOptions) (string, error) {
	path := filepath.Join(dir, "daypack.toml")
	if _, err := os.Lstat(path); err == nil {
		return "", fmt.Errorf("%s: %w", path, ErrExists)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	harnesses, err := NormalizeHarnesses(opts.Harnesses)
	if err != nil {
		return "", err
	}
	var data []byte
	if opts.From == "" {
		data = skeleton(harnesses)
	} else {
		src := opts.From
		if info, err := os.Stat(src); err == nil && info.IsDir() {
			src = filepath.Join(src, "daypack.toml")
		}
		data, err = os.ReadFile(src)
		if err != nil {
			return "", fmt.Errorf("read source manifest: %w", err)
		}
		if _, err := parse(data, src); err != nil {
			return "", err
		}
		if len(opts.Harnesses) > 0 {
			if data, err = setHarnesses(data, harnesses); err != nil {
				return "", err
			}
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("%s: %w", path, ErrExists)
		}
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		return "", errors.Join(err, f.Close(), os.Remove(path))
	}
	if err := f.Close(); err != nil {
		return "", errors.Join(err, os.Remove(path))
	}
	return path, nil
}

var harnessesLine = regexp.MustCompile(`^[ \t]*harnesses[ \t]*=[ \t]*\[`)

// setHarnesses rewrites only the root harnesses array of a single-line layout.
func setHarnesses(data []byte, harnesses []string) ([]byte, error) {
	manual := fmt.Errorf("edit daypack.toml by hand: set harnesses = %v", harnesses)
	offset := 0
	for _, line := range strings.SplitAfter(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			break
		}
		if loc := harnessesLine.FindStringIndex(line); loc != nil {
			open := offset + loc[1] - 1
			_, closing, ok := arraySpans(data, open)
			if !ok {
				return nil, manual
			}
			q := make([]string, len(harnesses))
			for i, h := range harnesses {
				q[i] = strconv.Quote(h)
			}
			out := splice(data, open+1, closing, []byte(strings.Join(q, ", ")))
			m, err := parse(out, "daypack.toml")
			if err != nil {
				return nil, manual
			}
			var bad []string
			for _, e := range m.Entries() {
				for _, h := range e.Harnesses {
					if !slices.Contains(harnesses, h) {
						bad = append(bad, string(e.Type)+":"+e.Name)
						break
					}
				}
			}
			if len(bad) > 0 {
				return nil, fmt.Errorf("entries target harnesses outside the selection: %s", strings.Join(bad, ", "))
			}
			return out, nil
		}
		offset += len(line)
	}
	return nil, manual
}

// WriteFile replaces path atomically through a temp file in the same directory.
func WriteFile(path string, data []byte, mode os.FileMode) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".daypack-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.Remove(f.Name()))
		}
	}()
	if err = f.Chmod(mode); err != nil {
		return errors.Join(err, f.Close())
	}
	if _, err = f.Write(data); err != nil {
		return errors.Join(err, f.Close())
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
