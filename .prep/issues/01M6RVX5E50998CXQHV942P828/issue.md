---
title: 'CLI contract: JSON output, exit codes, non-interactive mode'
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
tags:
  - cli
priority: high
---

Agents drive daypack as well as the user. Applies to every command (decision: Cobra; core as a Go package with the CLI as a thin front-end so another front-end, e.g. a TUI, could be added later).

- Package layout: operations live in importable packages (library, manifest, lock, plan, apply, adapters); cmd/ contains only Cobra wiring, flag parsing and rendering. No business logic in cmd/.
- --json on every command prints one JSON object to stdout and nothing else (logs go to stderr); the object always has ok, command, and command-specific fields (plan operations, problems, items).
- Exit codes: 0 success / nothing needing attention; 1 error (invalid input, I/O, internal); 2 usage error; 3 attention needed (pending changes in status/dry-run, drift, collision, skipped or gated entries).
- Non-interactive: no command prompts when stdin is not a TTY; prompts exist only in the gated apply case (preview issue) and are skipped by --yes.
- Errors are one line of cause plus one line of hint where possible, on stderr.
- Global flags: --json, --quiet, --no-color, --project <dir>, --library <dir> (overrides DAYPACK_HOME).
- No shell completions in v1.

## Open questions
