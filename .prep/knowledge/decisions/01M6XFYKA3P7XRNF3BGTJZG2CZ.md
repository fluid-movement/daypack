---
type: decision
title: Generate library-local instructions from the binary
description: Library init and doctor --fix maintain a marked AGENTS.md at the selected library root while preserving user records and notes.
date: "2026-11-08"
issue: 01M6V88RPGR75FV9QWBZ3MR41M
---

The user confirmed that AGENTS.md is universal across harnesses and local to the library, with its text compiled into Go and its managed region versioned by a marker pair. New library init writes it; init re-run and doctor --fix add or refresh it in existing libraries. Notes outside the markers survive, and list/import/doctor must not classify it as an artifact.

Reason: agents working in the config folder need layout, schemas and commands matching the installed binary, without globally installed instructions or separately distributed content drifting from that binary. Alternatives excluded by the requirement are per-harness skills, globally installed instructions, external templates fetched at runtime, and replacing the whole file (which would discard user notes).
