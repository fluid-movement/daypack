---
title: Bitwarden secret source (post-v1)
kind: code
parent: 01M6RSR1TPWPNN1MC4EJY0D4M2
depends_on:
  - 01M6RTJB4VVF8M46JAZ5RA32P0
tags:
  - post-v1
priority: low
---

FR-18. First external password-manager source, as a command template.

Decision: config holds only secret references and never depends on the daypack binary at runtime; sources are command templates (knowledge: 'MCP secrets: references only'; issue 01M6RTJB4VVF8M46JAZ5RA32P0). Uses the Bitwarden CLI read command. Known limit: bw needs an unlocked session (BW_SESSION); document and surface this in preview and verification. Model must allow 1Password (op read / op run) later.

## Open questions
