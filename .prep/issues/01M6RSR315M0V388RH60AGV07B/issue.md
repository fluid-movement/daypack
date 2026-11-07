---
title: Instruction fragments assembled into AGENTS.md (all harnesses)
kind: code
parent: 01M6RSR1WEJADFBE7H43FG9Y19
depends_on:
  - 01M6RSR2MYB5VPVPS8WHTWTDQG
  - 01M6RSR2FKVBKQHS28Z7J39NHB
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

## Acceptance

1. [ ] Fragments are assembled in manifest list order into the daypack:begin/end section of AGENTS.md; text outside the markers is byte-identical after apply
2. [ ] Removing all fragments removes the section markers; a missing AGENTS.md is created only when at least one fragment is listed
3. [ ] The Claude Code mechanism for loading AGENTS.md is implemented per the research finding and checked in a fixture

Model (after review, fits the per-entry item model): each fragment entry is one md-section item in AGENTS.md, target ID agents-md, delimited by <!-- daypack:begin fragment:NAME --> / <!-- daypack:end fragment:NAME -->, kept in manifest order. New sections go after the last existing daypack section, else at the end of the file (AGENTS.md is created if absent and at least one fragment is listed). A reorder rewrites the sections in the new order; text outside daypack markers is never touched.

Claude Code: target ID claude-md. If no CLAUDE.md exists, create one containing only an '@AGENTS.md' import line (owned file). If a hand-written CLAUDE.md exists, add a daypack-marked md-section containing '@AGENTS.md' without asking (daypack never prompts here). The tracked flag for both AGENTS.md sections and an added CLAUDE.md section follows the committed-exception decision.

## Open questions
