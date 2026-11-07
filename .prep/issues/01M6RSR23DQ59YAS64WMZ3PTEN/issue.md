---
title: 'Library CLI: init, add, edit, rename, remove'
kind: code
parent: 01M6RSQDN4E37HX2EM8F97SADA
depends_on:
  - 01M6RSR21N92V3W1HVT3TBWGCD
  - 01M6RVX5E50998CXQHV942P828
tags:
  - library
priority: high
---

FR-1. Commands under the 'library' group (decision: project verbs top level, library verbs under library): library init, library add, library edit, library rename, library remove, plus library import (separate issue) and library doctor (validate the library). list is a top-level project-aware command (discoverability issue).

- Covers all v1 artifact types: skills, MCP servers, instruction fragments, subagents.
- edit opens the artifact's files in $EDITOR; there are no in-tool editors.
- rename and remove act on the library only and print a clear warning that manifests referencing the old name will fail until updated; no registry or healing in v1 (decision: no global project registry in v1).
- Names are unique per type; add refuses duplicates.

## Open questions
