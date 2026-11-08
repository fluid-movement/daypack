package importer

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fluid-movement/daypack/internal/library"
)

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
func canonical(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved
	}
	absolute, err := filepath.Abs(path)
	if err == nil {
		return absolute
	}
	return path
}
func excluded(opts Options, path string) bool {
	for _, root := range []string{
		filepath.Join(opts.ClaudeConfigDir, "plugins"), filepath.Join(opts.ClaudeConfigDir, "skills", "synced"),
		filepath.Join(opts.PiAgentDir, "install"), filepath.Join(opts.PiAgentDir, "git"), filepath.Join(opts.PiAgentDir, "packages"), filepath.Join(opts.PiAgentDir, "node_modules"), filepath.Join(opts.HomeDir, ".pi", "packages"),
	} {
		if within(canonical(root), path) {
			return true
		}
	}
	return false
}
func readNative(path string, kind library.ArtifactType, opts Options) (candidate, error) {
	c := candidate{nativeSource: path}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return c, fmt.Errorf("broken or cyclic native source")
	}
	if excluded(opts, real) {
		return c, fmt.Errorf("plugin, synced or package source excluded")
	}
	if within(canonical(opts.LibraryDir), real) {
		return c, fmt.Errorf("source already points into library; retain the source link")
	}
	info, err := os.Stat(real)
	if err != nil {
		return c, fmt.Errorf("cannot inspect native source")
	}
	if (kind == library.Skill) != info.IsDir() {
		return c, fmt.Errorf("wrong native source type")
	}
	if kind == library.Agent {
		if !info.Mode().IsRegular() {
			return c, fmt.Errorf("native source must be a regular file")
		}
		data, e := os.ReadFile(real)
		if e != nil {
			return c, fmt.Errorf("cannot read native source")
		}
		c.files = []nativeFile{{data: data, mode: uint32(info.Mode().Perm())}}
	} else {
		err = filepath.WalkDir(real, func(file string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return fmt.Errorf("cannot read skill tree")
			}
			if entry.IsDir() {
				if file != real {
					rel, e := filepath.Rel(real, file)
					if e != nil {
						return fmt.Errorf("invalid skill directory")
					}
					c.files = append(c.files, nativeFile{path: filepath.ToSlash(rel), directory: true})
				}
				return nil
			}
			rel, e := filepath.Rel(real, file)
			if e != nil {
				return fmt.Errorf("invalid skill path")
			}
			target := file
			if entry.Type()&os.ModeSymlink != 0 {
				// Only SKILL.md links are native metadata aliases. Support trees must
				// remain self-contained and regular, just like library.Add requires.
				if rel != "SKILL.md" {
					return fmt.Errorf("skill tree contains a symlink")
				}
				target, e = filepath.EvalSymlinks(file)
				if e != nil || !within(real, target) || excluded(opts, target) {
					return fmt.Errorf("SKILL.md link escapes skill tree or is invalid")
				}
			}
			st, e := os.Stat(target)
			if e != nil || !st.Mode().IsRegular() {
				return fmt.Errorf("skill tree contains an unreadable or special file")
			}
			data, e := os.ReadFile(target)
			if e != nil {
				return fmt.Errorf("cannot read skill file")
			}
			c.files = append(c.files, nativeFile{path: filepath.ToSlash(rel), data: data, mode: uint32(st.Mode().Perm())})
			return nil
		})
		if err != nil {
			return c, err
		}
	}
	var data []byte
	for _, f := range c.files {
		if kind == library.Agent || f.path == "SKILL.md" {
			data = f.data
			break
		}
	}
	a, err := library.InspectNative(data, kind, filepath.Base(path))
	if err != nil {
		return c, fmt.Errorf("invalid native name or frontmatter")
	}
	a.Path = real
	c.artifact = a
	c.identity = nativeIdentity(kind, c.files)
	return c, nil
}
func nativeIdentity(kind library.ArtifactType, files []nativeFile) string {
	var b bytes.Buffer
	for _, f := range files {
		if kind == library.Skill {
			fmt.Fprintf(&b, "%d:%s:%d:%d:", len(f.path), f.path, f.mode&0111, len(f.data))
			if f.directory {
				b.WriteString("directory:")
			}
		}
		b.Write(f.data)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b.Bytes()))
}

// semanticMCP ignores TOML presence bookkeeping and empty optional containers.
func semanticMCP(d library.MCPDefinition, values map[string]string) string {
	d.Description = "" // Harness descriptions do not affect executable behavior.
	if len(d.Args) == 0 {
		d.Args = nil
	}
	if len(d.Env) == 0 {
		d.Env = nil
	}
	if len(d.Headers) == 0 {
		d.Headers = nil
	}
	replace := func(m map[string]string) map[string]string {
		out := map[string]string{}
		for k, v := range m {
			for name, literal := range values {
				if v == "${"+name+"}" {
					v = literal
					break
				}
			}
			out[k] = v
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	d.Env = replace(d.Env)
	d.Headers = replace(d.Headers)
	data, err := json.Marshal(d)
	if err != nil {
		return ""
	} // MCPDefinition has only JSON-supported fields.
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
