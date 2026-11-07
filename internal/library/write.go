package library

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type AddOptions struct {
	Type         ArtifactType
	Name, Source string
}

// ValidateName checks CLI and core mutation arguments without touching disk.
func ValidateName(kind ArtifactType, name string) error {
	if !validType(kind) {
		return fmt.Errorf("invalid artifact type; use skill, mcp, fragment or agent")
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("invalid artifact name; use 1-64 lowercase letters, digits or hyphens")
	}
	return nil
}

// safePath rejects symlinks in every existing component, including ancestors.
func safePath(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	for p := path; ; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if e != nil && !os.IsNotExist(e) {
			return "", e
		}
		if e == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink path is not allowed: %s", p)
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	return path, nil
}

func nativePath(root string, kind ArtifactType, name string) (string, error) {
	if err := ValidateName(kind, name); err != nil {
		return "", err
	}
	root, err := safePath(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("open library %s: %w", root, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("library %s is not a directory", root)
	}
	dir, ext := "", ".md"
	switch kind {
	case Skill:
		dir, ext = "skills", ""
	case MCP:
		dir, ext = "mcp", ".toml"
	case Fragment:
		dir = "fragments"
	case Agent:
		dir = "agents"
	}
	return safePath(filepath.Join(root, dir, name+ext))
}
func absent(path string) error {
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		if err != nil {
			return err
		}
		return fmt.Errorf("destination already exists: %s", path)
	}
	return nil
}
func contentPath(path string, kind ArtifactType) string {
	if kind == Skill {
		return filepath.Join(path, "SKILL.md")
	}
	return path
}
func validateArtifact(path string, kind ArtifactType, name string) (Artifact, error) {
	file := contentPath(path, kind)
	if _, err := safePath(file); err != nil {
		return Artifact{}, err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return Artifact{}, fmt.Errorf("read artifact %s: %w", file, err)
	}
	a := Artifact{Type: kind, Name: name, Path: path}
	var problems []Problem
	if kind == MCP {
		d, p := ParseMCP(data, file)
		a.MCP, a.Description, problems = &d, d.Description, p
	} else {
		a.Description, problems = metadata(data, file, kind, name)
	}
	for _, p := range problems {
		if p.Severity == Error {
			return Artifact{}, fmt.Errorf("validate %s (%s): %s; match the requested name and repair the native definition", file, p.Field, p.Message)
		}
	}
	return a, nil
}
func copyNative(source, dest string, directory bool) error {
	source, err := safePath(source)
	if err != nil {
		return err
	}
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("read source %s: %w", source, err)
	}
	if info.IsDir() != directory {
		return fmt.Errorf("source %s has wrong artifact file type", source)
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.Mkdir(target, 0755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("source %s is not a regular file (symlinks are not imported)", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

// publish atomically installs a staged artifact without overwriting collisions.
func publish(stage, dest string) error {
	if err := renameExclusive(stage, dest); err != nil {
		return fmt.Errorf("publish %s: %w", dest, err)
	}
	return nil
}
func stagePath(dest string) (string, func() error, error) {
	dir, err := os.MkdirTemp(filepath.Dir(dest), ".daypack-stage-")
	if err != nil {
		return "", nil, err
	}
	return filepath.Join(dir, "artifact"), func() error {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("clean staging directory %s: %w", dir, err)
		}
		return nil
	}, nil
}
func Add(root string, opts AddOptions) (_ Artifact, err error) {
	dest, err := nativePath(root, opts.Type, opts.Name)
	if err != nil {
		return Artifact{}, err
	}
	if err = absent(dest); err != nil {
		return Artifact{}, err
	}
	stage, clean, err := stagePath(dest)
	if err != nil {
		return Artifact{}, err
	}
	defer func() { err = errors.Join(err, clean()) }()
	if opts.Source != "" {
		err = copyNative(opts.Source, stage, opts.Type == Skill)
	} else {
		if opts.Type == Skill {
			err = os.Mkdir(stage, 0755)
		}
		if err == nil {
			err = os.WriteFile(contentPath(stage, opts.Type), stub(opts.Type, opts.Name), 0644)
		}
	}
	if err != nil {
		return Artifact{}, fmt.Errorf("create artifact %s from %s: %w", dest, opts.Source, err)
	}
	a, err := validateArtifact(stage, opts.Type, opts.Name)
	if err != nil {
		return Artifact{}, fmt.Errorf("validate source %s for destination %s: %w", opts.Source, dest, err)
	}
	if err = publish(stage, dest); err != nil {
		return Artifact{}, err
	}
	a.Path = dest
	return a, nil
}
func stub(kind ArtifactType, name string) []byte {
	switch kind {
	case MCP:
		return []byte("description = \"New MCP server\"\ntype = \"stdio\"\ncommand = \"echo\"\nargs = []\n")
	case Fragment:
		return []byte("# Instructions\n\nAdd instructions here.\n")
	default:
		return []byte(fmt.Sprintf("---\nname: %s\ndescription: New %s\n---\n\nAdd instructions here.\n", name, kind))
	}
}
