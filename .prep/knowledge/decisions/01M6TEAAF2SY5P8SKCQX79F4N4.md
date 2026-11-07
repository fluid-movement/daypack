---
type: decision
title: Pi is in v1 to prove the adapter interface
description: 'Pi ships in v1 alongside Claude Code. Reason: a second adapter is the test that the adapter interface is really harness-neutral. Pi supports the shared agent skills directory (~/.agents/skills/, project-level equivalent to be verified), so the skills adapter is written once for that standard and covers other harnesses that read it. Pi MCP config is a simple JSON file (~/.pi/agent/mcp.json). Pi instruction fragments stay out of v1.'
date: "2026-11-07"
---


