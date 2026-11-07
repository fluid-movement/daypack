---
type: decision
title: 'MCP secrets: references only; sources are command templates'
description: 'Config holds only references and never depends on the daypack binary. env source = ${VAR}; keychain/Bitwarden/1Password = wrapped stdio command or headersHelper for remote, using the source''s own CLI. Reason: matches documented Claude Code and secret-manager patterns, keeps NFR-4, same read command powers the FR-19 check.'
date: "2026-11-06"
issue: 01M6RTJB4VVF8M46JAZ5RA32P0
supersedes:
  - 01M6RTJWS453327AP3MHFQ67P0
outcome: true
---


