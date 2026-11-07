---
type: decision
title: Per-artifact harness targeting in the manifest
description: 'Every skill, MCP server, instruction fragment and subagent entry in the manifest applies to all selected harnesses by default, and may carry an optional harnesses list to restrict it (e.g. an MCP server only for claude-code). Reason: the user runs Claude Code as the orchestrator with full tooling and Pi for small subagent-style tasks where context must stay free. Shared targets are honoured, not hidden: a skill restricted to Pi still lands in the shared .agents/skills directory that other harnesses read, and check reports which other harnesses will see it.'
date: "2026-11-07"
---


