---
type: decision
title: 'Implementation delegation: Codex first, Claude as fallback'
description: Codex first; Claude subagents when Codex is exhausted; serialize issues touching the same package
date: "2026-11-08"
---

User instruction: implementation goes to Codex (free usage) first; when Codex usage is exhausted, fall back to Claude Code subagents (sonnet/haiku, worktrees, per orchestrate skill). Also: issues touching the same package (e.g. internal/adapters/claudecode) run one at a time, since parallel worktrees produced a duplicate adapter.
