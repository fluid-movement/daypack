---
title: Managed-vs-handwritten ownership tracking
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
depends_on:
  - 01M6RSRR7P0GKP661AK1RHJK0A
tags:
  - apply
priority: high
---

FR-11, FR-12. Mechanism to mark and track what the tool manages so hand-written config is never overwritten or deleted and removals converge. Foundation for the apply engine.

Decision: generated harness config is git-excluded, never committed (knowledge: 'Generated harness config is git-excluded'; issue 01M6RSRR7P0GKP661AK1RHJK0A). Implication: the lockfile/state record lists every managed path; daypack writes exactly those paths into .git/info/exclude and removes entries when items are removed. Hand-written siblings stay trackable. Mixed files (CLAUDE.md, .mcp.json, settings.json) use marked managed sections or the harness's gitignored local variant.

## Open questions
