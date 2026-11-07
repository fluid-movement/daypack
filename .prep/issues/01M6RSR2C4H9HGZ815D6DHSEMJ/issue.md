---
title: 'Manifest authoring commands: init, copy from project, add/remove'
kind: code
parent: 01M6RSR1PXHKSP1819E2VPNBFE
tags:
  - manifest
---

FR-10, reduced. Create and change a manifest without hand-editing; the toml stays hand-editable and the source of truth.

- init creates a manifest, optionally copying another project's manifest (daypack init --from <path>), since new projects usually start as a copy of a working one.
- add and remove commands edit the toml in place preserving comments and order, e.g. daypack add skill:svelte, daypack add mcp:context7 --only claude-code, daypack remove mcp:svelte-docs. --only restricts an entry to the listed harnesses (per-artifact targeting decision); a change of --only on an existing entry updates it.
- No interactive picker and no TUI in v1. NFR-1: once a manifest exists, apply needs no prompts.

## Open questions
