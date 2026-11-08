---
type: decision
title: Fragment sections reconcile per file; bridge uses reserved ref
description: Ordered ReconcileSections in lock; CLAUDE.md bridge via ProjectAdapter with ref fragment:@agents-md
date: "2026-11-08"
issue: 01M6RSR315M0V388RH60AGV07B
---

Surviving sections keep slots unless order changes, new ones attach after their manifest predecessor, drift or conflict in one section skips the whole file batch. Bridge ref contains @, which library names cannot, and keeps the fragment type prefix so validateOwned accepts it.
