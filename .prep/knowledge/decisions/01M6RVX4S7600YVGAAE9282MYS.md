---
type: decision
title: 'Ownership: per-project lockfile, refuse collisions, git-excluded'
description: 'Each project has an in-project, git-excluded lockfile recording every managed path (with link target or hash) and every managed entry inside shared files (file, key, hash; CLAUDE.md also gets visible markers). Apply only touches what the lockfile owns; hand-edited managed content is reported as drift and not deleted without --force; a name collision with hand-written content makes apply refuse unless --adopt is passed. Reason: makes ''hand-written config survives every apply'' hold by construction; lockfile in the project survives moves/worktrees and makes eject trivial.'
date: "2026-11-06"
---


