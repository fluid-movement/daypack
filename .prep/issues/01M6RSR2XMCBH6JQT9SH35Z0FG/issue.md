---
title: Check reports unset secret variables
kind: code
parent: 01M6RSR1TPWPNN1MC4EJY0D4M2
depends_on:
  - 01M6RTJB4VVF8M46JAZ5RA32P0
tags:
  - secrets
---

FR-19, kept small. As part of check/status, report every environment variable referenced by the project's MCP servers (library-based and inline) that is not set, before the user hits a runtime failure. Never prints values. Other sources (keychain, Bitwarden) are post-v1.

## Open questions
