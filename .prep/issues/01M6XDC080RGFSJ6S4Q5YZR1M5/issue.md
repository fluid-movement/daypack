---
title: 'Library secrets file: use .env plus generated .env.example'
kind: code
parent: 01M6RSR1TPWPNN1MC4EJY0D4M2
tags:
  - secrets
priority: high
---

Rename the library's private secrets file from <library>/secrets.env to <library>/.env (dotenv format, same loader, same owner-only permission check, still git-ignored by library init). library init/doctor also generate <library>/.env.example listing every env var referenced by the library's MCP definitions with empty values (committed-safe), refreshed by doctor --fix without touching .env. Keep reading legacy secrets.env as a fallback with a doctor note suggesting the rename. Update docs/secrets.md, README and the library AGENTS.md issue text. Requested by the user: .env is the established pattern and .env.example shows which tokens must be added.

## Open questions
