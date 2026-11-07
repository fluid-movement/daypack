---
title: 'Claude Code: project-scoped subagents'
kind: code
parent: 01M6RSR1WEJADFBE7H43FG9Y19
depends_on:
  - 01M6RVX52JWVSGVS8C6QMHTH6A
tags:
  - claude-code
---

FR-21. Subagents are Claude Code only. Facts (finding 01M6TETMWQSAMYAGRMW5T045MG): files live in .claude/agents/*.md, scanned recursively; frontmatter requires name and description, body is the system prompt; files with missing required fields or bad YAML are skipped silently, so validate before writing. Project beats user. Symlinked agent files are UNVERIFIED; copy if symlinks fail. Owned and git-excluded via the lockfile.

## Open questions
