---
type: decision
title: Codex uses native shared skills, AGENTS.md and project custom-agent TOML
description: Support all four artifact types; translate library agent Markdown to standalone project TOML.
date: "2026-11-09"
issue: 01M6ZW8G0B62Q0V52SQ32VA6GZ
---

Current official documentation (researched 2026-11-09) establishes project skills at .agents/skills, native AGENTS.md, and standalone project custom agents at .codex/agents/*.toml. Sources: https://learn.chatgpt.com/docs/build-skills ; https://learn.chatgpt.com/docs/agent-configuration/agents-md ; https://learn.chatgpt.com/docs/agent-configuration/subagents .

Choose skilldir.AgentsTarget()/Desired and instructions.Target()/FragmentItem so Pi and Codex share one physical link/section and ownership row. Do not add a Codex-specific skill tree or instructions bridge.

Choose .codex/agents/<artifact-name>.toml as owned File items. Parse the existing library agent YAML frontmatter: name and description become TOML strings, the Markdown body becomes developer_instructions. Carry an explicitly supplied model string verbatim; omit model when absent so Codex inherits its setting. Reject other nonempty agent frontmatter fields with UnsupportedError field names rather than silently losing Claude-specific tools/permissions settings. Do not copy Markdown verbatim as a Codex agent, invent model aliases, or add an obsolete [agents.<role>] config_file registration. Native custom-agent support is documented, so declaring all agents unsupported would fail the requirement's conditional scope.

Custom agents inherit parent permissions and MCP servers. Do not emit agent-specific sandbox, approval, MCP settings or enablement flags. Document this boundary and leave runtime trust/availability checks unverified until an authorized live check.
