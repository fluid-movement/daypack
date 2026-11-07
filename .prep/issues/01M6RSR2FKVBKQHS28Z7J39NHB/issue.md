---
title: 'Apply engine: convergent, repeatable'
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
depends_on:
  - 01M6RSRR7P0GKP661AK1RHJK0A
tags:
  - apply
priority: high
---

FR-9, FR-11. Reconcile project harness config with the resolved manifest; idempotent; removes things no longer wanted; target Claude Code, Pi or both.

Decision: generated harness config is git-excluded, never committed (knowledge: 'Generated harness config is git-excluded'; issue 01M6RSRR7P0GKP661AK1RHJK0A). Apply updates the lockfile and the git exclusions (.git/info/exclude) in the same operation.

Safe failure (decision: no global project registry in v1): if a manifest entry cannot be resolved in the library, apply reports it, skips that entry, and never deletes already-applied managed items because of it. Dangling symlinks into the library are reported by status/check. Apply exits non-zero when anything was skipped.

## Open questions
