---
title: 'Claude Code adapter: MCP servers without plaintext secrets'
kind: code
parent: 01M6RSR1TPWPNN1MC4EJY0D4M2
depends_on:
  - 01M6RSR2MYB5VPVPS8WHTWTDQG
tags:
  - claude-code
priority: high
---

Write the manifest's MCP servers into the project for Claude Code (finding 01M6TETMWQSAMYAGRMW5T045MG).

Facts: project MCP lives in .mcp.json at the repo root, with ${VAR} and ${VAR:-default} expansion in command, args, env, url and headers (credential-named variables such as ANTHROPIC_API_KEY and NPM_TOKEN expand to empty in remote url/headers, so use other variable names). Project servers need approval; approval is stored as enabledMcpjsonServers in .claude/settings.local.json. There is no gitignored variant of .mcp.json.

Decision applied: config holds only secret references (issue 01M6RTJB4VVF8M46JAZ5RA32P0).

Decisions: entry-level ownership of .mcp.json via the lockfile (git-excluded if daypack created the file; if it pre-existed and is tracked the user sees the diff). daypack also writes enabledMcpjsonServers for its servers to the git-excluded .claude/settings.local.json so no manual approval is needed; this is the only settings key v1 writes. Unverified (see finding): whether the approval keys are honoured from an untracked settings.local.json in a never-trusted folder; verify during implementation and report in check if approval is still needed.

## Open questions
