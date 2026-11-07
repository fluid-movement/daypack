---
type: decision
title: Generated harness config is git-excluded, never committed
description: 'daypack excludes exactly the paths it manages via .git/info/exclude (from its lockfile); check enforces it. Reason: managed items are symlinks/generated and not useful to commit; keeps content out of git; hand-written siblings stay trackable. Cost: apply needed per clone/worktree.'
date: "2026-11-06"
issue: 01M6RSRR7P0GKP661AK1RHJK0A
supersedes:
  - 01M6RTJB2SRWN78AAGN1JTTSDP
outcome: true
---


