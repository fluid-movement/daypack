---
title: Instruction fragments assembled into AGENTS.md (all harnesses)
kind: code
parent: 01M6RSR1WEJADFBE7H43FG9Y19
depends_on:
  - 01M6RVX52JWVSGVS8C6QMHTH6A
  - 01M6RVX5BR6J3T9KRKD8WXS548
tags:
  - claude-code
priority: low
---

FR-20. Lowest priority of the three v1 features (skills, MCP, instruction fragments); build after the others work.

Instruction fragments from the manifest are assembled deterministically (manifest list order) into a clearly marked managed section of AGENTS.md; hand-written content outside the section is preserved. AGENTS.md is the single target for all harnesses (Pi and others read it directly).

Decision: the managed section is committed with AGENTS.md; this is the one exception to git exclusion (content is plain text, no secrets). The lockfile still records the section so apply, removal and drift work like every other managed entry.

Claude Code reads CLAUDE.md, not AGENTS.md: the adapter must make Claude Code load AGENTS.md (e.g. an @AGENTS.md import). Exact marker format and the Claude Code mechanism are settled in this issue, based on the Claude Code conventions research.

## Open questions
