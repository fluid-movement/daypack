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
	generated, e := generatedProblems(l.root)
	if e != nil {
		return nil, e
	}
	problems = append(problems, generated...)
	metadata, err := secretsMetadata(l.root)
	if err != nil {
		return nil, err
	}
	problems = append(problems, metadata...)
	hygiene, err := secretsHygiene(l.root)
	if err != nil {
		return nil, err
	}
	problems = append(problems, hygiene...)
	sortProblems(problems)
	return problems, nil
}

// secretsHygiene inspects only Git policy, without reading secret contents.
func secretsHygiene(root string) ([]Problem, error) {
	var out []Problem
	for _, name := range []string{".env", "secrets.env", ".env.example"} {
		if name == "secrets.env" {
			if _, err := os.Lstat(filepath.Join(root, name)); os.IsNotExist(err) {
				continue
			}
		}
		p, err := fileHygiene(root, name)
		if err != nil {
			return nil, err
		}
		out = append(out, p...)
	}
	sortProblems(out)
	return out, nil
}

// secretsHygiene inspects only Git policy, without reading secret contents.
func fileHygiene(root, name string) ([]Problem, error) {
	problems := []Problem{}
	file := filepath.Join(root, name)
	add := func(message string) {
		problems = append(problems, problem(file, "git", message, "Ignore /.env and /secrets.env; keep /.env.example trackable."))
	}
	status, err := gitStatus(root, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return nil, fmt.Errorf("inspect Git hygiene in %s: %w", root, err)
	}
	if status == 0 {
		tracked, e := gitStatus(root, "ls-files", "--error-unmatch", "--", name)
		if e != nil {
			return nil, e
		}
		switch tracked {
		case 0:
			if name != ".env.example" {
				add(name + " is tracked by git")
			}
		case 1:
		default:
			add("could not inspect tracked " + name + " with git")
		}
		ignored, e := gitStatus(root, "check-ignore", "--no-index", "--", name)
		if e != nil {
			return nil, e
		}
		switch ignored {
		case 0:
			if name == ".env.example" {
				add(".env.example is ignored by git")
			}
		case 1:
			if name != ".env.example" {
				add(name + " is not ignored by git")
			}
		default:
			add("could not inspect " + name + " ignore status with git")
		}
	} else {
		// A .git marker in an ancestor means a failing repository, not a clean
		// non-Git library (including linked worktrees with .git files).
		for p := root; ; p = filepath.Dir(p) {
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
		data, e := os.ReadFile(filepath.Join(root, ".gitignore"))
		if e != nil && !os.IsNotExist(e) {
			return nil, e
		}
		ignored := false
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			negate := strings.HasPrefix(line, "!")
			pattern := strings.TrimPrefix(strings.TrimPrefix(line, "!"), "/")
			match, err := filepath.Match(pattern, name)
			if err != nil {
				// Invalid ignore patterns do not match any file.
				continue
			}
			if match {
				ignored = !negate
			}
		}
		if name == ".env.example" && ignored {
			add(".env.example is ignored by library .gitignore")
		}
		if name != ".env.example" && !ignored {
			add(name + " has no ignore rule in library .gitignore")
		}

	}
	sortProblems(problems)
	return problems, nil
}
