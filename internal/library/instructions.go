package library

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const instructionsBegin = "<!-- daypack:begin library-instructions -->"
const instructionsEnd = "<!-- daypack:end library-instructions -->"

// Increment the content revision whenever this compiled guide changes.
const instructionsBody = `
# Daypack library guide

Generated content revision: 2. Refresh with daypack library init or daypack library doctor --fix.
Keep personal notes outside the library-instructions markers; edits inside are replaced.
These instructions apply only to this library folder, across harnesses. No harness skills
or global instructions are installed.

## Selected library and layout

Selection precedence: --library, DAYPACK_HOME, XDG_CONFIG_HOME/daypack,
then HOME/.config/daypack. Use the same selection for all library commands.
Native artifacts live at skills/<name>/SKILL.md (with optional support files),
mcp/<name>.toml, fragments/<name>.md and agents/<name>.md.
Names contain 1-64 lowercase letters, digits or hyphens and are unique per type.
Root AGENTS.md is generated guidance, not an artifact or an import source.

## Native record schemas

Skills and agents require YAML frontmatter with name matching the skill folder
or agent filename (without .md), and a nonempty description. For example,
skills/review/SKILL.md or agents/review.md:

~~~yaml
---
name: review
description: Review changes carefully
---
Review the changes and explain findings.
~~~

Fragments are Markdown instructions, with optional YAML frontmatter. For example,
fragments/style.md:

~~~yaml
---
description: Shared style guidance
---
Use clear names.
~~~

MCP records are TOML. Description is optional. stdio requires command and allows
args and env; http requires url and allows headers. Do not mix transport fields.
For example, mcp/docs.toml:

~~~toml
description = "Documentation server"
type = "stdio"
command = "docs-server"
args = ["--format", "text"]
[env]
DOCS_TOKEN = "${DOCS_TOKEN}"
~~~

An alternative HTTP definition:

~~~toml
type = "http"
url = "https://${DOCS_HOST:-docs.example.test}/mcp"
[headers]
Authorization = "Bearer ${DOCS_TOKEN}"
~~~

## Commands

Library management:
- daypack library init: initialize an absent library with artifact directories,
  .gitignore, .env.example and this guide. Existing directories refresh generated
  housekeeping only. Invalid MCP skips the example with a warning.
- daypack library doctor: read-only validation of records and secrets metadata/Git hygiene.
- daypack library doctor --fix: refresh generated housekeeping, then validate normally;
  requires an existing library and does not repair records or secrets.
- daypack library import [--dry-run]: import user skills, agents and MCP definitions
  from supported harness configurations; --dry-run previews without writing.
- daypack library add <type> <name> [--from <native-path>]: add a stub or native source.
- daypack library edit <type> <name>: open its native file using EDITOR in a terminal.
- daypack library rename <type> <old-name> <new-name> and
  daypack library remove <type> <name>: change library artifacts and warn about
  manifest references; update affected manifests yourself.
Types are skill, mcp, fragment and agent.

Project wiring uses separate top-level commands: daypack init creates daypack.toml;
daypack add <type>:<name> and daypack remove <type>:<name> change project references;
daypack list [type] lists available artifacts with project usage;
daypack apply materializes configuration; daypack status and daypack check inspect it.

## Reference-only secrets

Credentials belong in the harness's inherited environment or the optional ignored,
owner-only (chmod 600) library .env, never in records, defaults or this guide.
Use ${NAME} or ${NAME:-default} with non-secret defaults in MCP command, args, env,
url and headers. Daypack does not automatically source dotenv or resolve values
when generating or validating configuration. Load trusted secrets into your shell
before launching the harness. Reads fall back to legacy secrets.env only when .env
is absent; manually rename legacy storage. Generation never reads either store; doctor checks
only its metadata and Git hygiene. Init refresh and doctor --fix leave all records,
support files, .env, secrets.env and unrelated files untouched.
Init and doctor --fix refresh managed .gitignore and .env.example blocks.
.env.example lists all referenced variables, including defaults, with empty values.
Keep .env and secrets.env ignored and .env.example trackable.
`

func renderInstructions(data []byte) ([]byte, error) {
	begin, end := strings.Index(string(data), instructionsBegin), strings.Index(string(data), instructionsEnd)
	// Count even incomplete owned markers, so malformed markers cannot accumulate.
	for _, marker := range []string{"begin", "end"} {
		prefix := "daypack:" + marker + " library-instructions"
		full := "<!-- " + prefix + " -->"
		if bytes.Count(data, []byte(prefix)) != bytes.Count(data, []byte(full)) {
			return nil, fmt.Errorf("malformed library-instructions markers; repair the managed marker pair")
		}
	}
	nb, ne := bytes.Count(data, []byte(instructionsBegin)), bytes.Count(data, []byte(instructionsEnd))
	if nb != ne || nb > 1 || (nb == 1 && end < begin) {
		return nil, fmt.Errorf("duplicate, nested or unmatched library-instructions markers; repair the managed marker pair")
	}
	region := []byte(instructionsBegin + instructionsBody + instructionsEnd)
	if nb == 0 {
		out := append([]byte(nil), data...)
		if len(out) > 0 && out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
		return append(append(out, region...), '\n'), nil
	}
	out := append([]byte(nil), data[:begin]...)
	out = append(out, region...)
	return append(out, data[end+len(instructionsEnd):]...), nil
}

// safeInstructionsPath guards descendants of a canonical library root only.
func safeInstructionsPath(root, path string) (string, error) {
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("instructions path escapes library: %s", path)
	}
	for p := path; p != root; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink path is not allowed: %s", p)
		}
	}
	return path, nil
}

// RefreshInstructions updates only the root guide of an existing library.
func RefreshInstructions(root string) (err error) {
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("open existing library: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("open existing library: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("library %s is not a directory", root)
	}
	path, err := safeInstructionsPath(root, filepath.Join(root, "AGENTS.md"))
	if err != nil {
		return err
	}
	var data []byte
	mode := os.FileMode(0644)
	info, err = os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("instructions %s must be a regular file", path)
		}
		mode = info.Mode().Perm()
		data, err = os.ReadFile(path)
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read instructions: %w", err)
	}
	out, err := renderInstructions(data)
	if err != nil {
		return fmt.Errorf("instructions %s: %w", path, err)
	}
	if bytes.Equal(data, out) {
		return nil
	}
	stage, clean, err := stagePath(path)
	if err != nil {
		return fmt.Errorf("stage instructions: %w", err)
	}
	defer func() { err = errors.Join(err, clean()) }()
	stage, err = safeInstructionsPath(root, stage)
	if err != nil {
		return fmt.Errorf("stage instructions: %w", err)
	}
	if err := os.WriteFile(stage, out, mode); err != nil {
		return fmt.Errorf("write instructions: %w", err)
	}
	if err := os.Chmod(stage, mode); err != nil {
		return fmt.Errorf("set instructions mode: %w", err)
	}
	if err := os.Rename(stage, path); err != nil {
		return fmt.Errorf("replace instructions: %w", err)
	}
	return nil
}
