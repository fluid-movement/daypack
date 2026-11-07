---
title: 'Test strategy: fixture projects and golden files'
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
tags:
  - testing
priority: high
---

End-to-end and unit test foundation. Go standard testing; no network and no real home directory in any test.

- Fixture helpers: build a temp library (skills, mcp, fragments, agents) and a temp project (with git init, optionally a second worktree), run the real CLI entry point in-process with --library/--project overrides, and assert on the file tree.
- Golden files for adapter output (Item lists) and for plan text/JSON.
- A fake harness adapter in test code declaring shared and unique targets, used to prove the adapter contract.
- Sentinel-secret test: put a recognisable secret value in the environment and secrets.env, run apply/status/import, and assert it appears in no file under the project and in no command output.
- Required scenarios (each maps to acceptance criteria elsewhere): apply twice is a no-op; manifest removals converge; hand-written files and JSON keys survive; collisions refused; drift reported; unresolved entries skipped safely; shared target reconciled once; restricted entries; worktree exclusion; gated MCP plan behaviour.
- CI: a GitHub Actions workflow running go vet, go test and golangci-lint on push.

## Open questions
