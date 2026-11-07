---
title: 'MCP definition model: local-process and remote'
kind: code
parent: 01M6RSR1TPWPNN1MC4EJY0D4M2
depends_on:
  - 01M6RTJB4VVF8M46JAZ5RA32P0
tags:
  - mcp
priority: high
---

FR-16. Schema for MCP server definitions: local-process and remote.

Decision: config holds only secret references and never depends on the daypack binary at runtime (issue 01M6RTJB4VVF8M46JAZ5RA32P0). Definitions declare required secrets by name plus a source reference; they never contain values.

The same schema is used for library files and for inline definitions in a project manifest, and one parser serves both. Validation rejects literal secret values in either place.

## Open questions
