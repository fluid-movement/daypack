---
title: 'Secret sources: env references and global .env file'
kind: code
parent: 01M6RSR1TPWPNN1MC4EJY0D4M2
depends_on:
  - 01M6RSR21N92V3W1HVT3TBWGCD
tags:
  - secrets
priority: high
---

FR-17, FR-18, success criterion 4. v1 supports one secret source: environment variables. MCP definitions reference secrets by variable name (${VAR}); no value is ever written to a project file or committable file.

Decision: config holds only secret references and never depends on the daypack binary at runtime (issue 01M6RTJB4VVF8M46JAZ5RA32P0).

- Values live in the user's environment, optionally via a global secrets .env file in the daypack config dir (owner-only permissions, never inside a project); how that file reaches the shell environment is part of this issue and must be documented (e.g. sourced from the shell profile).
- The source model stays pluggable (a source turns a reference into a read command) so keychain and Bitwarden can be added post-v1 without changing manifests or library files.
- Import (initial setup issue) writes discovered tokens into this .env file.

## Open questions
