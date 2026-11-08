---
type: decision
title: Merge verified code while test-coverage follow-ups stay open
description: Verified behavior plus green gates merges; test-thinness findings go to a follow-up branch with the issue left open
date: "2026-11-08"
---

When a checker verifies behavior end to end and all gates pass but fails an issue only on test coverage (Claude MCP criterion 6, status), the code is merged to main and the issue stays open; the missing tests or evidence are added in a follow-up branch. Reason: the user wants the project done ASAP and downstream issues were blocked behind these. Live-eval criteria (Claude MCP approval) additionally need the user OK before running. How to apply: do not merge when gates fail or behavior is wrong; implementers must write tests against the exact wording of every criterion up front.
