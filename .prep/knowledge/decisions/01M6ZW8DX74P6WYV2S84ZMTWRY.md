---
type: decision
title: Init refreshes generated housekeeping; invalid MCP never fails init
description: 'Resolves blocker: re-running library init is a refresh, not an error.'
date: "2026-11-09"
issue: 01M6XDC080RGFSJ6S4Q5YZR1M5
---

Resolution of the implementer's blocker (orchestrator, consistent with the user's AGENTS.md decision that re-running library init refreshes generated files in an existing library): (1) library init on an existing real library directory is allowed and only creates/refreshes generated housekeeping (.env.example block, managed .gitignore block, AGENTS.md); it never touches artifacts or private files. Existing tests that require init to refuse an existing library (TestInitRefusesExisting) must be updated to the new contract; init still fails for a file, dangling link or unsafe destination. (2) Init succeeds even when MCP records are invalid: the .env.example refresh is skipped with a warning naming the problem, and doctor/doctor --fix report it. (3) The .gitignore is edited only through a managed marker block; unrelated user lines are preserved (existing tests for that stay).
