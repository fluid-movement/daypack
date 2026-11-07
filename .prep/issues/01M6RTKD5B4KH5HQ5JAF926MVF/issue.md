---
title: Auto-apply on fresh clone or worktree
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
depends_on:
  - 01M6RSRR7P0GKP661AK1RHJK0A
tags:
  - apply
priority: high
---

Because generated config is git-excluded (decision 01M6RSRR7P0GKP661AK1RHJK0A), every fresh clone or git worktree needs an apply. Provide a way to do this without remembering: e.g. a post-checkout/worktree hook installer and/or a session-start check that detects an unapplied project and applies or warns. Must fit NFR-1 (no prompts once a manifest exists) and NFR-3 (never applies untrusted content). The orchestrate workflow creates a worktree per issue, so this matters in practice.

## Open questions
