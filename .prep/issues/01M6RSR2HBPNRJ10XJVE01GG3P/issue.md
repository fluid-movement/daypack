---
title: Preview / dry-run and change report
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
depends_on:
  - 01M6RSR2FKVBKQHS28Z7J39NHB
tags:
  - apply
priority: high
---

FR-13, NFR-3, NFR-1. The plan renderer and the confirmation gate. The plan is the single object printed by apply, apply --dry-run and status, and is available as JSON.

Plan output (text): grouped by harness target, one line per operation with a marker:
  + create   ~ update   - remove   ! conflict (collision)   ? drift   x unresolved/unsupported
Each line shows the entry (type:name), the path or JSON key, and for symlinks the link target. MCP servers show the full command line (command + args) or URL with ${VAR} references left unexpanded, and the list of secret variable names they need (never values). A summary line counts operations per marker.

Confirmation gate (decision: nothing that runs commands is applied silently): apply asks for confirmation only when the plan creates or changes an MCP server that is not already in the lockfile with identical command/url (a new executable), including every inline server on a first apply. Otherwise there is no prompt (NFR-1). --yes skips the prompt. With no TTY and no --yes, a gated plan applies nothing and exits 3 with a message naming the gated entries.

After applying, a result report lists what changed, what was skipped and why. JSON output carries the same structure (see CLI contract).

## Open questions
