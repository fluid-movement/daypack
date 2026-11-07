---
title: 'Initial setup: scan harnesses and import skills/MCP into the library'
kind: code
parent: 01M6RSQDN4E37HX2EM8F97SADA
depends_on:
  - 01M6RSR21N92V3W1HVT3TBWGCD
  - 01M6RVX52JWVSGVS8C6QMHTH6A
  - 01M6RVX5BR6J3T9KRKD8WXS548
tags:
  - import
priority: high
---

FR-2, success criterion 1. After install, the user's skills and MCP servers are scattered across harness configs. daypack scans the supported harnesses, copies what it finds into the library, and prints a list of the originals for the user to remove by hand. daypack never modifies or deletes harness config in this flow.

Scan (user-level only, never project files):
- Claude Code: user skills, user subagents, user-scope MCP servers, and per-project local-scope MCP servers stored in ~/.claude.json. Exact locations come from the Claude Code conventions research issue.
- Pi: skills in ~/.agents/skills/ and MCP config in ~/.pi/agent/mcp.json (verify in the Pi conventions research issue).
- Excluded: anything installed by a plugin or marketplace, and settings, permissions, env, user-level CLAUDE.md (untouched, per decision).

Import:
- Copies into the library in native formats; a symlinked skill is copied from its target. MCP servers become library TOML definitions.
- The same skill or server found in both harnesses is deduplicated by content; same name with different content is reported and the user chooses.
- Name collisions with existing library entries are never overwritten silently.
- Dry-run first (shows what would be imported, from where, under which names). Re-runnable and idempotent.
- Plaintext tokens in MCP env or headers are not copied into library definitions. They are moved into a global secrets .env file in the daypack config dir (owner-only permissions) and the definition gets a reference. Exact mechanism follows the MCP secret-source issue.

Output: a printed list of the original items (paths/entries) that are now in the library and can be removed. No removal, no prompts, no backups needed.

## Open questions
