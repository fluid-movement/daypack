---
title: Report leftover global setup
kind: code
parent: 01M6RSQDN4E37HX2EM8F97SADA
depends_on:
  - 01M6RVX52JWVSGVS8C6QMHTH6A
  - 01M6RVX5BR6J3T9KRKD8WXS548
tags:
  - import
---

Success criterion 1 must be checkable. 'daypack status --global' (read-only) lists skills, subagents and MCP servers still present in the user-level config of the supported harnesses, excluding plugin-provided items, so the user sees what is left after import and detects new global installs. Exit 3 when leftovers exist. Uses the same scan code as import.

## Open questions
