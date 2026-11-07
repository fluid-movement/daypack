---
title: 'Claude Code adapter: skills'
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
depends_on:
  - 01M6RSR2MYB5VPVPS8WHTWTDQG
  - 01M6RVX52JWVSGVS8C6QMHTH6A
  - 01M6RSR2DX646G82T1EYZBD071
tags:
  - claude-code
priority: high
---

Claude Code adapter, skills (finding 01M6TETMWQSAMYAGRMW5T045MG). Target ID: claude-skills-dir. For each skill entry that applies to claude-code, the adapter declares one Symlink item: .claude/skills/<name> -> <library>/skills/<name> (absolute path; the link points straight at the library, not chained through .agents/skills). Claude Code does not read the shared .agents/skills directory, hence this separate target. Owned and git-excluded through the lockfile.

Note: Claude Code gives personal (user-level) skills precedence over project skills, so a leftover global skill of the same name overrides the project one; the leftover-global check covers that.

## Open questions
