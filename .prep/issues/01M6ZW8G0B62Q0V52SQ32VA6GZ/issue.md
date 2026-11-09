---
title: Codex harness adapter (skills, MCP, fragments, subagents if supported)
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
tags:
  - codex
priority: high
---

Add OpenAI Codex CLI as a third harness, 'codex', so manifests can target it ('harnesses = ["claude-code","pi","codex"]' and --only codex). Follow the adapter contract in internal/adapters/contract and the existing claudecode and pi adapters (shared-target declarations, ownership lockfile, git excludes, dry-run plan). Scope, to be confirmed against Codex's current documentation (the enrichment pass must research the real project-level config layout and record findings): skills via the shared .agents/skills target that Codex reads (reuse skilldir; no duplicate links when pi is also selected); MCP servers merged into the project Codex config (.codex/config.toml [mcp_servers.<name>] tables, TOML merge with ownership of only daypack-managed tables, user tables preserved) using env-var references only, never plaintext secrets, and with the correct Codex fields for env passthrough and bearer-token env vars; instruction fragments via AGENTS.md, which Codex reads natively (reuse the existing fragments assembly, no extra files); custom subagents only if Codex documents project-level agent definitions, otherwise report unsupported with a clear note. Also: notes for Codex trust requirements for project config (untrusted projects ignore .codex/config.toml), register 'codex' as a known harness in init/add/status/README, daypack library import support for existing Codex skills if cheap. Tests with the testadapter-style fixtures and goldens; no live Codex evals without the user's OK; document anything unverifiable in docs/testing.md.

## Open questions
