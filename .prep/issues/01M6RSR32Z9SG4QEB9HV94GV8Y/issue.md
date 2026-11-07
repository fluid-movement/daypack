---
title: 'Claude Code: project-scoped subagents'
kind: code
parent: 01M6RSR1WEJADFBE7H43FG9Y19
depends_on:
  - 01M6RSR2MYB5VPVPS8WHTWTDQG
  - 01M6RVX52JWVSGVS8C6QMHTH6A
  - 01M6RSR2DX646G82T1EYZBD071
tags:
  - claude-code
---

FR-21. Subagents are Claude Code only (finding on Claude Code conventions). Target ID claude-agents-dir. Each agent entry is one File item: .claude/agents/<name>.md, a copy of the library file (symlinked agent files are UNVERIFIED, so v1 copies; verifying symlinks is an optional follow-up). The lockfile hash is of the written bytes, so a library edit shows as a pending update on the next apply, not as drift.

Files live in .claude/agents/*.md, scanned recursively; frontmatter requires name and description; files missing a required field or with bad YAML are skipped silently by Claude Code, so library validation must reject them first. Project beats user.

## Open questions
