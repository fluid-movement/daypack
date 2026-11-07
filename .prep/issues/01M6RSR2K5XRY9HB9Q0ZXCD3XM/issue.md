---
title: Status and drift report
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
depends_on:
  - 01M6RSRR7P0GKP661AK1RHJK0A
tags:
  - apply
---

FR-14. Report what the project manifest lists, what is wired in, and drift (manual edits, broken or missing items).

Decision: generated harness config is git-excluded, never committed (issue 01M6RSRR7P0GKP661AK1RHJK0A). Status/check must also verify that every managed path is git-excluded and that no managed file is tracked by git; violations are reported as errors.

## Open questions
