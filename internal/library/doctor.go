package library

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func gitStatus(root string, args ...string) (int, error) {
	err := exec.Command("git", append([]string{"-C", root}, args...)...).Run()
	if err == nil {
		return 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	return -1, err
}
func Doctor(path string) ([]Problem, error) {
	l, err := Load(path)
	if err != nil {
		return nil, err
	}
	problems := l.Validate()
	file := filepath.Join(l.root, "secrets.env")
	add := func(message string) {
		problems = append(problems, problem(file, "git", message, "Ignore /secrets.env and keep it untracked."))
	}
	status, err := gitStatus(l.root, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return nil, fmt.Errorf("inspect Git hygiene in %s: %w", l.root, err)
	}
	if status == 0 {
		tracked, e := gitStatus(l.root, "ls-files", "--error-unmatch", "--", "secrets.env")
		if e != nil {
			return nil, e
		}
		switch tracked {
		case 0:
			add("secrets.env is tracked by git")
		case 1:
		default:
			add("could not inspect tracked secrets.env with git")
		}
		ignored, e := gitStatus(l.root, "check-ignore", "--no-index", "--", "secrets.env")
		if e != nil {
			return nil, e
		}
		switch ignored {
		case 0:
		case 1:
			add("secrets.env is not ignored by git")
		default:
			add("could not inspect secrets.env ignore status with git")
		}
	} else {
		// A .git marker in an ancestor means a failing repository, not a clean
		// non-Git library (including linked worktrees with .git files).
		for p := l.root; ; p = filepath.Dir(p) {
			if _, e := os.Lstat(filepath.Join(p, ".git")); e == nil {
				add("could not inspect library Git repository")
				sortProblems(problems)
				return problems, nil
			} else if !os.IsNotExist(e) {
				return nil, e
			}
			if filepath.Dir(p) == p {
				break
			}
		}
		data, e := os.ReadFile(filepath.Join(l.root, ".gitignore"))
		if e != nil && !os.IsNotExist(e) {
			return nil, e
		}
		ignored := false
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "/secrets.env" || line == "secrets.env" {
				ignored = true
			}
			if line == "!/secrets.env" || line == "!secrets.env" {
				ignored = false
			}
		}
		if !ignored {
			add("secrets.env has no ignore rule in library .gitignore")
		}
	}
	sortProblems(problems)
	return problems, nil
}
