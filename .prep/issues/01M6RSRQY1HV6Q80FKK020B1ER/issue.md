---
title: 'Pi adapter: MCP servers'
kind: code
parent: 01M6RSR1Y6XR8HFX1YFTZSWGRR
depends_on:
  - 01M6RSR2MYB5VPVPS8WHTWTDQG
  - 01M6RVX5BR6J3T9KRKD8WXS548
  - 01M6RTJB4VVF8M46JAZ5RA32P0
  - 01M6RSR2DX646G82T1EYZBD071
tags:
  - pi
---

Pi MCP config written from the manifest's MCP servers (finding 01M6TESPYYQJZ5V7Y6BDE7SDJK).

Facts: project config is .pi/mcp.json (user: ~/.pi/agent/mcp.json), top-level mcpServers like Claude Code's. A project entry replaces a user entry of the same name. ${VAR} and $VAR expand in env, headers and oauth.clientSecret (url/command/args expansion UNVERIFIED); a per-value "!command" gives a headersHelper equivalent. Existing Pi files contain non-schema keys, so the adapter must preserve unknown keys and touch only its own entries.

Trust gate: project .pi/mcp.json and project .agents/skills load only after Pi project trust is granted (/trust), and are skipped silently in non-interactive modes by default. Status/check must report this; daypack should not grant trust itself.

Decision applied: config holds only secret references (issue 01M6RTJB4VVF8M46JAZ5RA32P0).

Decision: daypack does not set exposure; Pi's default stays.

## Acceptance

1. [ ] Target ID pi-mcp-json: one JSONEntry item per MCP server at .pi/mcp.json mcpServers.<name>, shape verified against the Pi finding, ${VAR} references preserved
2. [ ] Existing keys in .pi/mcp.json, including non-schema keys, survive apply and removal
3. [ ] No exposure key is written (Pi default stays)
4. [ ] status reports the Pi project trust requirement when pi is selected

Field support: Pi is only known to expand ${VAR} in env, headers and oauth.clientSecret. A server with ${VAR} references in url, command or args yields an 'unsupported' plan line for pi naming the field, unless the Pi research is extended (e.g. via 'pi mcp list') to verify expansion there, in which case record the result.

## Open questions
