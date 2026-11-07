---
title: 'Discoverability: list available artifacts, validate manifest, editor schema'
kind: code
parent: 01M6RSR1PXHKSP1819E2VPNBFE
depends_on:
  - 01M6RSR28PM3XB3VXM0HDPJ61N
  - 01M6RSR21N92V3W1HVT3TBWGCD
tags:
  - manifest
priority: high
---

It must be clear which options exist when editing a manifest.

- daypack list [type] shows library artifacts by type with name and one-line description (from the artifact's own metadata, e.g. SKILL.md frontmatter), plus which are used by the current project and for which harnesses. Inline MCP servers are shown as project-local.
- Manifest validation (also run by check) rejects unknown names and unknown or unselected harness names, suggests the closest match, and errors on inline/library name collisions.
- daypack init writes a commented manifest showing the available sections and the optional per-entry harnesses setting.
- Not in v1: generated JSON Schema for editors, shell completions.

## Open questions
