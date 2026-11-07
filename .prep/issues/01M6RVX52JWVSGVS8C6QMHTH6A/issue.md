---
title: 'Research: Claude Code project conventions'
kind: research
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
tags:
  - research
priority: high
---

Document, verified against current Claude Code docs and a real install, exactly how Claude Code reads project-level config. Output: a knowledge entry that the Claude Code adapter issues cite.

Questions to answer:
- Skills: .claude/skills layout, symlink support, and whether Claude Code reads the shared project .agents/skills directory.
- Subagents: .claude/agents format.
- Instructions: how Claude Code loads AGENTS.md (it reads CLAUDE.md); is an @AGENTS.md import the right mechanism; CLAUDE.md vs CLAUDE.local.md.
- MCP: .mcp.json format and scopes, ${VAR} expansion, where approval of project .mcp.json servers is stored, and how to wire them without hand approval if that is possible.
- Which of these have gitignored local variants, and precedence between user and project scopes.
- Where user-level (global) skills, agents and MCP live, including per-project local-scope MCP in ~/.claude.json, for the import scan.

## Open questions
