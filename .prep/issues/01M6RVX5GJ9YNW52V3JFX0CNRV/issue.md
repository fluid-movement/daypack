---
title: 'Test strategy: fixture projects and golden files'
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
depends_on:
  - 01M6RVX5E50998CXQHV942P828
tags:
  - testing
priority: high
---

Test FOUNDATION only (helpers, fake adapter, sentinel helper, CI); the scenario tests below land with the feature issue that makes them possible, so this issue does not wait for them. Go standard testing; no network and no real home directory in any test.

- Fixture helpers: build a temp library (skills, mcp, fragments, agents) and a temp project (with git init, optionally a second worktree), run the real CLI entry point in-process with --library/--project overrides, and assert on the file tree.
- Golden files for adapter output (Item lists) and for plan text/JSON.
- A fake harness adapter in test code declaring shared and unique targets, used to prove the adapter contract.
- Sentinel-secret test: put a recognisable secret value in the environment and secrets.env, run apply/status/import, and assert it appears in no file under the project and in no command output.
- Coverage list (each scenario is implemented in the feature issue it belongs to, mapped to that issue's acceptance criteria): apply twice is a no-op; manifest removals converge; hand-written files and JSON keys survive; collisions refused; drift reported; unresolved entries skipped safely; shared target reconciled once; restricted entries; worktree exclusion; gated MCP plan behaviour.
- CI: a GitHub Actions workflow running go vet, go test and golangci-lint on push.

## Acceptance

1. [ ] go test ./... passes offline with no access to the real home directory (tests set DAYPACK_HOME and HOME to temp dirs)
2. [ ] The sentinel-secret test exists and fails if a secret value is ever written into a project
3. [ ] A fake-adapter test proves shared-target and restricted-entry behaviour
4. [ ] Each required scenario in the body has at least one named test
5. [ ] A CI workflow runs vet, tests and lint on push

## Open questions
