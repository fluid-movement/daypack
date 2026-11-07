---
title: 'CLI contract: JSON output, exit codes, non-interactive mode'
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
tags:
  - cli
priority: high
---

Agents will drive daypack as well as the user. Every command has a stable --json output, consistent exit codes (ok, changes needed, drift found, error) and a non-interactive mode (no prompts once a manifest exists, NFR-1). No shell completions in v1.

## Open questions
