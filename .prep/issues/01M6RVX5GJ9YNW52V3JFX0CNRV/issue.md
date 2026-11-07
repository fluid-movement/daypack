---
title: 'Test strategy: fixture projects and golden files'
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
tags:
  - testing
priority: high
---

Set up an end-to-end test harness with fixture projects and a temp library. Cases required: apply twice changes nothing; removing an item from the manifest removes its files and git exclusions; hand-written files and entries survive every apply; collisions are refused; drift is reported; no secret value appears in any project file. Each acceptance criterion in later issues maps to one of these cases.

## Open questions
