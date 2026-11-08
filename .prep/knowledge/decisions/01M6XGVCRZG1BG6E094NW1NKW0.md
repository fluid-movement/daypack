---
type: decision
title: 'Library agent docs: AGENTS.md only, no skills'
description: Library folder gets a generated AGENTS.md; no per-harness skills.
date: "2026-11-08"
issue: 01M6V88RPGR75FV9QWBZ3MR41M
---

Chosen (by the user): library init writes a generated AGENTS.md at the library root and nothing else for agent guidance. Reason: AGENTS.md is universal across harnesses, while skills would need a separate implementation per harness. Alternative rejected: skills under the library's .agents/skills and .claude/skills.
