---
type: decision
title: 'Claude Code MCP: entry-level ownership in .mcp.json, pre-approve via settings.local.json'
description: 'daypack writes MCP servers into the project .mcp.json with entry-level ownership recorded in the lockfile (only its own server names are touched). If daypack created the file it is git-excluded; if it pre-existed and is tracked, the user sees the diff (same exception as AGENTS.md). daypack also writes enabledMcpjsonServers for its servers into the git-excluded .claude/settings.local.json so servers work without the manual approval prompt; this is the only settings key v1 writes. Reason: makes ''one command and it works'' true while keeping hand-written config untouched.'
date: "2026-11-07"
---


