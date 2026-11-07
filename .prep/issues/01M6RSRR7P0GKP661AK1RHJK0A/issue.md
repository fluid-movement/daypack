---
title: 'Decide: generated harness config is never committed (git-excluded)'
kind: decision
parent: 01M6RSR1ZZM9VB5TAX90Y6QR0R
tags:
  - decision
priority: high
---

Generated harness configuration is never committed to the project repo. daypack git-excludes exactly the paths it manages, not whole harness directories, and `check`/status verifies the exclusion.

Details:
- Managed paths (e.g. `.claude/skills/svelte` symlinks, generated agents, local settings) come from daypack's own lockfile/state record. Hand-written siblings (a project's own skill) stay trackable with no negation rules.
- Exclusions go into `.git/info/exclude` rather than a committed `.gitignore`, so the repo stays clean and git worktrees share them.
- Files mixing hand-written and managed content (CLAUDE.md, .mcp.json, settings.json) are not ignored wholesale; daypack writes to the harness's gitignored local variant where one exists (settings.local.json, CLAUDE.local.md). How .mcp.json is handled is decided in the Claude Code MCP adapter research.
- The manifest stays the committed artifact (location: see the manifest-location decision).

Why: skills etc. are symlinks and not useful to commit; ignoring keeps resolved content out of git and keeps repos clean.
Consequence: every fresh clone or worktree needs an apply (see the auto-apply issue). Teammates are not served by the generated output alone; acceptable for a personal tool.

## Knowledge

- decision: Generated harness config is git-excluded, never committed (/decisions/01M6RTJB2SRWN78AAGN1JTTSDP.md)

## Open questions
