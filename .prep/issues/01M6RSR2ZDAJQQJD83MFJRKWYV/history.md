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
