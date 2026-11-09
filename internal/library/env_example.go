package library

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fluid-movement/daypack/internal/secrets"
)

type managedFile struct {
	root, path string
	data, out  []byte
	mode       os.FileMode
}

func managedContent(data []byte, identity, body string) ([]byte, error) {
	begin, end := "# daypack:begin "+identity, "# daypack:end "+identity
	text := string(data)
	nb, ne := strings.Count(text, begin), strings.Count(text, end)
	b, e := strings.Index(text, begin), strings.Index(text, end)
	if nb != ne || nb > 1 || nb == 1 && e < b {
		return nil, fmt.Errorf("malformed %s markers; repair the managed marker pair", identity)
	}
	// Owned markers must occupy complete lines.
	for _, marker := range []string{begin, end} {
		i := strings.Index(text, marker)
		if i >= 0 && (i > 0 && text[i-1] != '\n' || i+len(marker) < len(text) && text[i+len(marker)] != '\n') {
			return nil, fmt.Errorf("malformed %s markers", identity)
		}
	}
	region := begin + "\n" + body + end
	if nb == 0 {
		if len(text) > 0 && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		return []byte(text + region + "\n"), nil
	}
	return []byte(text[:b] + region + text[e+len(end):]), nil
}

func planManaged(root, name, identity, body string) (managedFile, error) {
	f := managedFile{root: root, path: filepath.Join(root, name), mode: 0644}
	if _, err := safeInstructionsPath(f.root, f.path); err != nil {
		return f, err
	}
	info, err := os.Lstat(f.path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return f, fmt.Errorf("%s must be a regular file", f.path)
		}
		f.mode = info.Mode().Perm()
		f.data, err = os.ReadFile(f.path)
	}
	if err != nil && !os.IsNotExist(err) {
		return f, err
	}
	f.out, err = managedContent(f.data, identity, body)
	return f, err
}

func (f managedFile) write() (err error) {
	if bytes.Equal(f.data, f.out) {
		return nil
	}
	stage, clean, err := stagePath(f.path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, clean()) }()
	if _, err = safeInstructionsPath(f.root, stage); err != nil {
		return err
	}
	if err = os.WriteFile(stage, f.out, f.mode); err != nil {
		return err
	}
	if err = os.Chmod(stage, f.mode); err != nil {
		return err
	}
	if _, err = safeInstructionsPath(f.root, f.path); err != nil {
		return err
	}
	return os.Rename(stage, f.path)
}

func exampleBody(root string) (string, []Problem, error) {
	if _, err := safeInstructionsPath(root, filepath.Join(root, "mcp")); err != nil {
		return "", nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "mcp"))
	if os.IsNotExist(err) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	names := map[string]bool{}
	var problems []Problem
	for _, entry := range entries {
		if housekeeping(entry.Name()) {
			continue
		}
		path := filepath.Join(root, "mcp", entry.Name())
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".toml" {
			problems = append(problems, problem(path, "mcp", "cannot scan MCP entry for .env.example", "Repair the MCP entry."))
			continue
		}
		if _, err := safeInstructionsPath(root, path); err != nil {
			return "", nil, err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return "", nil, err
		}
		if !info.Mode().IsRegular() {
			return "", nil, fmt.Errorf("MCP %s must be a regular file", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", nil, err
		}
		d, p := ParseMCP(data, path)
		for _, item := range p {
			if item.Severity == Error {
				problems = append(problems, item)
			}
		}
		for _, value := range d.fields() {
			refs, err := secrets.ParseReferences(value)
			if err != nil {
				continue
			}
			for _, ref := range refs {
				names[ref.Name] = true
			}
		}
	}
	keys := make([]string, 0, len(names))
	for name := range names {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	var out strings.Builder
	for _, name := range keys {
		out.WriteString(name + "=\n")
	}
	sortProblems(problems)
	return out.String(), problems, nil
}

const ignoreBody = "/.env\n/secrets.env\n!/.env.example\n"

func housekeepingPlans(root string) ([]managedFile, []Problem, error) {
	ignore, err := planManaged(root, ".gitignore", "library-secrets", ignoreBody)
	if err != nil {
		return nil, nil, err
	}
	body, problems, err := exampleBody(root)
	if err != nil {
		return nil, nil, err
	}
	plans := []managedFile{ignore}
	if len(problems) == 0 {
		example, err := planManaged(root, ".env.example", "env-example", body)
		if err != nil {
			return nil, nil, err
		}
		plans = append(plans, example)
	}
	return plans, problems, nil
}

// RefreshHousekeeping repairs generated files only; invalid MCP leaves the example intact.
func RefreshHousekeeping(root string) ([]Problem, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("library must be a directory")
	}
	plans, problems, err := housekeepingPlans(root)
	if err != nil {
		return nil, err
	}
	// Validate the guide before publishing any generated file.
	guidePath := filepath.Join(root, "AGENTS.md")
	if _, err := safeInstructionsPath(root, guidePath); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(guidePath); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("instructions must be a regular file")
	}
	guide, err := os.ReadFile(guidePath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if _, err = renderInstructions(guide); err != nil {
		return nil, err
	}
	if err = RefreshInstructions(root); err != nil {
		return nil, err
	}
	for _, f := range plans {
		if err = f.write(); err != nil {
			return nil, err
		}
	}
	hygiene, err := secretsHygiene(root)
	if err != nil {
		return nil, err
	}
	for _, p := range hygiene {
		if p.Field == "git" && strings.Contains(p.Message, "ignored") || strings.Contains(p.Message, "ignore rule") {
			return nil, fmt.Errorf("%s: %s", p.File, p.Message)
		}
	}
	return problems, nil
}

func generatedProblems(root string) ([]Problem, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	plans, problems, err := housekeepingPlans(root)
	if err != nil {
		return []Problem{problem(root, "generated", err.Error(), "Repair generated housekeeping paths or markers.")}, nil
	}
	for _, f := range plans {
		if !bytes.Equal(f.data, f.out) {
			problems = append(problems, Problem{Warning, f.path, "generated", "generated housekeeping is missing or stale", "Run daypack library doctor --fix."})
		}
	}
	return problems, nil
}
