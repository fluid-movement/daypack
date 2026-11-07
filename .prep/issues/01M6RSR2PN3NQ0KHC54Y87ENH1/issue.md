---
title: 'Claude Code adapter: skills'
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
depends_on:
  - 01M6RVX52JWVSGVS8C6QMHTH6A
tags:
  - claude-code
priority: high
---

Wire library skills into the project for Claude Code. Facts (finding 01M6TETMWQSAMYAGRMW5T045MG): Claude Code reads .claude/skills/<name>/SKILL.md and supports per-skill directory symlinks; it does NOT read the shared .agents/skills directory. So this adapter creates per-skill symlinks in .claude/skills/ (pointing at the library, or at the shared .agents/skills entry), owned and git-excluded via the lockfile. Note skills precedence is personal over project (opposite to settings and MCP), so any leftover global skill of the same name wins: the leftover-global check matters here.

## Open questions
