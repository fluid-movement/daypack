---
title: 'Discoverability: list available artifacts, validate manifest, editor schema'
kind: code
parent: 01M6RSR1PXHKSP1819E2VPNBFE
depends_on:
  - 01M6RSR28PM3XB3VXM0HDPJ61N
  - 01M6RSR21N92V3W1HVT3TBWGCD
  - 01M6RVX5E50998CXQHV942P828
tags:
  - manifest
priority: high
---

It must be clear which options exist when editing a manifest.

- daypack list [type] shows library artifacts by type with name and one-line description (from the artifact's own metadata, e.g. SKILL.md frontmatter), plus which are used by the current project and for which harnesses. Inline MCP servers are shown as project-local.
- Manifest validation (also run by check) rejects unknown names and unknown or unselected harness names, suggests the closest match, and errors on inline/library name collisions.
- Not in v1: generated JSON Schema for editors, shell completions.

## Acceptance

1. [ ] list shows all library artifacts grouped by type with description; entries used by the current project are marked with their harnesses
2. [ ] list works outside a project (no marks)
3. [ ] A manifest referencing an unknown name fails validation with the closest-name suggestion
4. [ ] list --json returns the same data

## Open questions
