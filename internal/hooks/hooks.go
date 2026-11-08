// Package hooks installs and removes the Git post-checkout hook that runs
// 'daypack apply --auto'. It never edits Git configuration and only ever
// removes or replaces a script that it can recognize byte-for-byte as its own.
package hooks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Result describes the hook file only; it carries no apply or lock data.
type Result struct {
	// Path is the post-checkout hook location resolved through Git.
	Path string `json:"path"`
	// Invocation is a safely quoted command that preserves the caller's exit
	// status, set when a foreign hook prevented installation.
	Invocation string `json:"invocation,omitempty"`
	Changed    bool   `json:"changed"`
	Conflict   bool   `json:"conflict"`
}

const (
	header = "#!/bin/sh\n# daypack post-checkout hook v1 (managed by 'daypack hook install'; remove with 'daypack hook uninstall')\n"
	tail   = " apply --auto </dev/null || :\nexit 0\n"
)

func quote(executable string) string {
	return "'" + strings.ReplaceAll(executable, "'", `'\''`) + "'"
}

func validate(executable string) error {
	if !filepath.IsAbs(executable) {
		return fmt.Errorf("executable path must be absolute")
	}
	if strings.ContainsAny(executable, "\x00\r\n") {
		return fmt.Errorf("executable path must not contain NUL, CR or LF")
	}
	return nil
}

func script(executable string) []byte { return []byte(header + quote(executable) + tail) }

// parseExecutable reads the executable back using only the strict encoding
// that script produces and confirms a regenerated script matches byte-for-byte.
func parseExecutable(data []byte) (string, bool) {
	rest, ok := strings.CutPrefix(string(data), header+"'")
	if !ok {
		return "", false
	}
	var exe strings.Builder
	for {
		i := strings.IndexByte(rest, '\'')
		if i < 0 {
			return "", false
		}
		exe.WriteString(rest[:i])
		rest = rest[i:]
		if after, ok := strings.CutPrefix(rest, `'\''`); ok {
			exe.WriteByte('\'')
			rest = after
			continue
		}
		break
	}
	path := exe.String()
	if validate(path) != nil || !bytes.Equal(script(path), data) {
		return "", false
	}
	return path, true
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// hookPath resolves the hooks directory with Git so linked worktrees and
// core.hooksPath are honored. Git runs from the worktree top so a relative
// core.hooksPath resolves exactly as Git itself resolves it.
func hookPath(ctx context.Context, projectDir string) (string, error) {
	dir, err := filepath.Abs(projectDir)
	if err != nil {
		return "", err
	}
	top, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("%s is not inside a Git working tree: %w", dir, err)
	}
	hooks, err := git(ctx, top, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	if err != nil {
		hooks, err = git(ctx, top, "rev-parse", "--git-path", "hooks")
		if err != nil {
			return "", err
		}
	}
	if !filepath.IsAbs(hooks) {
		hooks = filepath.Join(top, hooks)
	}
	return filepath.Join(hooks, "post-checkout"), nil
}

// Install writes the hook unless a foreign or edited hook already exists.
func Install(ctx context.Context, projectDir, executable string) (Result, error) {
	if err := validate(executable); err != nil {
		return Result{}, err
	}
	path, err := hookPath(ctx, projectDir)
	if err != nil {
		return Result{}, err
	}
	res := Result{Path: path}
	want := script(executable)
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return res, err
		}
		return res, create(path, want, &res)
	case err != nil:
		return res, err
	}
	if info.Mode().IsRegular() {
		data, err := os.ReadFile(path)
		if err != nil {
			return res, err
		}
		if bytes.Equal(data, want) {
			return res, nil
		}
		if _, owned := parseExecutable(data); owned {
			return res, replace(path, want, &res)
		}
	}
	res.Conflict = true
	res.Invocation = "( " + quote(executable) + " apply --auto </dev/null || : )"
	return res, nil
}

func create(path string, data []byte, res *Result) (err error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if rerr := os.Remove(path); rerr != nil {
				err = errors.Join(err, rerr)
			}
		}
	}()
	if _, err = f.Write(data); err != nil {
		return errors.Join(err, f.Close())
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Chmod(path, 0o755); err != nil {
		return err
	}
	res.Changed = true
	return nil
}

// replace swaps an owned script (for example after the binary moved) through a
// temporary file so a failure never leaves a partial hook.
func replace(path string, data []byte, res *Result) error {
	tmp := path + ".daypack-tmp"
	if err := os.Remove(tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var tmpRes Result
	if err := create(tmp, data, &tmpRes); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	res.Changed = true
	return nil
}

// Uninstall removes only a hook this package installed.
func Uninstall(ctx context.Context, projectDir string) (Result, error) {
	path, err := hookPath(ctx, projectDir)
	if err != nil {
		return Result{}, err
	}
	res := Result{Path: path}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return res, nil
	}
	if err != nil {
		return res, err
	}
	if info.Mode().IsRegular() {
		data, err := os.ReadFile(path)
		if err != nil {
			return res, err
		}
		if _, owned := parseExecutable(data); owned {
			if err := os.Remove(path); err != nil {
				return res, err
			}
			res.Changed = true
			return res, nil
		}
	}
	res.Conflict = true
	return res, nil
}
