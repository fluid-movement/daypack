---
type: decision
title: Lint gate for issue checks
description: golangci-lint is the gate; errcheck -blank applies only to code the issue touches
date: "2026-11-07"
---

Checkers gate on golangci-lint (the CI gate, pinned v2.15.0) plus errcheck -blank only for files an issue adds or changes. Reason: errcheck -blank also flags pre-existing code outside the issue, which failed issues for reasons unrelated to their diff. How to apply: when checking an issue, run golangci-lint over the whole repo and errcheck -blank scoped to the issue diff.
