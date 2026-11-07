---
type: finding
title: Name availability for daypack (checked 2026-11-07)
description: npm 'daypack' is taken (unrelated, v2.6.2); GitHub has unrelated daypack repos but the owner's own repo is fluid-movement/daypack; no Homebrew formula named daypack
date: "2026-11-07"
issue: 01M6RSRRH4NF44SDDT5466B2NM
---

Checked 2026-11-07 with npm view, gh search repos, brew search.

- npm: package 'daypack' exists (version 2.6.2, unrelated). Irrelevant while distribution is a Go binary via GitHub releases; matters only if an npm wrapper is ever wanted (then use a scoped name).
- GitHub: several unrelated repos named daypack (an OCaml scheduling library, a Gradio app generator, a CLI frontend). No clash with the planned repo fluid-movement/daypack; the name is not unique in search results but is usable under the owner's namespace.
- Homebrew: no formula or cask named daypack in the core taps (brew search finds only lapack/arpack), so a tap or core formula is possible later.
- Go module path: github.com/fluid-movement/daypack, no conflict.

Conclusion: usable as-is for a GitHub-hosted Go binary. No fallback name needed unless npm distribution is wanted.
