---
type: decision
title: 'CLI: Cobra, project verbs top level, library verbs under library'
description: 'Implemented with Cobra (Bubble Tea if a TUI is added later; TUI is not in v1). Project verbs are top level: init, add, remove, list, apply, status/check. Library verbs live under ''library'': init, import, add, edit, rename, remove, doctor. list shows available artifacts marked with what the current project uses. Reason: two distinct jobs (managing the global library vs wiring a project) kept separate so daily commands stay short.'
date: "2026-11-07"
---


