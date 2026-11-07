---
title: Library rename with aliases, tombstones, and apply --fix (post-v1)
kind: code
parent: 01M6RSQDN4E37HX2EM8F97SADA
tags:
  - post-v1
priority: low
---

Lazy self-healing for renamed/removed library artifacts: rename records the old name as an alias (travels with the library via git), removal leaves a tombstone for a clear error, apply --fix rewrites manifests to the new name and repairs dangling links. Revisit if breakage from renames turns out to be frequent.

## Open questions
