- 2026-11-06T16:20:51Z edited by claude-code: depends_on, requirement

- 2026-11-06T16:21:18Z edited by claude-code: requirement

- 2026-11-06T16:41:42Z edited by claude-code: requirement

- 2026-11-06T16:43:40Z edited by claude-code: depends_on

- 2026-11-07T07:34:12Z edited by claude-code: requirement

- 2026-11-07T14:36:24Z edited by claude-code: requirement

- 2026-11-07T14:57:34Z edited by claude-code: depends_on

- 2026-11-07T15:05:05Z edited by claude-code: requirement

- 2026-11-07T15:05:05Z edited by claude-code: depends_on

- 2026-11-08T08:14:04Z codex: Extended the existing claudecode.Adapter and retained its single DefaultRegistry registration, skill/agent behavior and unsupported Fragment behavior. MCP uses Artifact.MCP and library validation to emit one reference-preserving JSONEntry and one JSON-encoded JSONArrayElement per server. Added deterministic payload golden, library/inline selection, safe validation/sentinel tests and lock composition covering raw array-name conversion, hashing, collisions/drift, no-op writes, approval adoption/Preserve persistence and cleanup, per-file tracking and repository/worktree exclusions. These are renderer/lock composition tests, not production apply/status CLI coverage. Verification passed: go build ./...; go vet ./...; go test -count=1 ./...; GOPROXY=off GOSUMDB=off go test -count=1 ./...; whole-repo go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run (0 issues); go run github.com/kisielk/errcheck@latest -blank ./... (clean across repo); git diff --check; prep check (0 errors, 2 existing K004 warnings). Used writable temporary Go/lint caches because default caches are sandbox-restricted. Manual criterion 6 remains UNVERIFIED: neither never-trusted-folder approval nor Claude worktree settings lookup was run; live harness checks require separate user OK, so no Claude version or approval observation is claimed and Notes remains unchanged pending evidence. No completion, commit or push performed.

- 2026-11-08T08:14:04Z codex: Implemented, ready for check: Extended the existing Claude adapter with reference-only MCP server and approval items, added offline rendering and ownership/worktree fixtures, and passed all requested automated checks; live approval behavior remains explicitly unverified.

- 2026-11-08T08:15:18Z claude-code: Check failed: criterion 6 unmet. No manual approval check (fresh folder, git worktree) is recorded in the log and no unverified limitation is documented; live Claude check needs user OK. Notes/status approval hint is also not decided. Criteria 1-5 and 7-13 verified (build, vet, test -count=1, offline GOPROXY=off test, golangci-lint v2.15.0 0 issues, errcheck -blank clean on claudecode, prep check run). Not completed.

- 2026-11-08T08:20:02Z claude-code: Criterion 6 UNVERIFIED: no live Claude session run (needs user OK). Claude adapter now emits static Note mcp-approval (approve project MCP servers via /mcp or enabledMcpjsonServers on first use), tested. Manual steps to verify later (fresh never-trusted folder with untracked settings.local.json, then git worktree with its own .mcp.json/settings.local.json; record Claude version, paths, trust state, approval, worktree lookup) are documented in docs/testing.md.

- 2026-11-08T08:21:18Z claude-code: Rebased onto main (skills delegate to skilldir, MCP dispatch kept; updated Pi tests for Claude MCP items and note). Implemented, ready for check: Claude Code adapter renders reference-only MCP servers plus approval entries and a static mcp-approval Note; live approval remains UNVERIFIED with manual steps documented in docs/testing.md.
