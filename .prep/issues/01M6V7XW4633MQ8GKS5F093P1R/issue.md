---
title: Project registry and apply --all (post-v1)
kind: code
parent: 01M6ZYR5JF2KKXZM3S731V5PPF
tags:
  - post-v1
priority: low
---

Machine-local (not synced) registry of projects keyed by git common dir, pruned when paths vanish, registered on init/apply. Enables apply --all, 'which projects use X', used-by warnings on library remove, and rename --update-projects (dry-run by default, skipping repos with uncommitted manifest changes). Deferred by decision 'No global project registry in v1'.

## Open questions
