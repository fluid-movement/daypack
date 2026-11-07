---
title: Project manifest format and loader
kind: code
parent: 01M6RSR1PXHKSP1819E2VPNBFE
tags:
  - manifest
priority: high
---

The project manifest is a small, hand-editable toml in the project (location per the manifest-location decision, open question 4). It directly lists the library artifacts the project uses, by stable name, grouped by type: skills, mcp servers, instruction fragments, subagents. Hooks are post-v1. It also declares the target harnesses (claude-code, pi, or both). There are no templates: the manifest is the source of truth, and a new project starts by copying a manifest from a working project.

Requirements:
- Plain, commented, order-preserving toml; instruction fragments are assembled in list order (deterministic, FR-20).
- Artifact names are unique per type in the library, so there are no merge conflicts to resolve.
- Loading validates every referenced name against the library and fails with a clear message and nearest-name suggestions.
- Schema version field so the format can evolve; forward-compatible so an optional cross-project key (e.g. templates) could be added later without breaking existing files.
- MCP servers may also be defined inline in the manifest, using exactly the same schema as library MCP definitions (parser shared with the library loader). Example: [mcp.postgres-shop] with command and args. A name present both inline and in the library is an error, not silent shadowing. Literal secret values are rejected in inline definitions; secrets are references only (see MCP secret handling decision).
- Inline definitions exist for MCP servers only in v1. Skills, agents and instruction fragments are library-only.
- Per-artifact harness targeting: every entry (skill, MCP server, instruction fragment, subagent, library-based or inline) applies to all selected harnesses by default and may carry an optional harnesses list restricting it, written as a table, e.g. { name = "context7", harnesses = ["claude-code"] }. A plain string means all selected harnesses. Naming a harness that is not selected in the manifest is a validation error. Entries for a type a harness cannot use (e.g. subagents on Pi) are reported clearly, not silently dropped. Rationale: Claude Code is the full-tooling orchestrator; Pi runs small subagent-style tasks and should keep its context free.

## Open questions
