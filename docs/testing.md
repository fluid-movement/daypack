# Testing

Use standard Go testing and import `internal/testutil` only from test files.
Tests run offline and never need a live harness, secret manager, or user home.
Local Git is required for fixtures. Do not call `t.Parallel` in tests using
fixtures or sentinel helpers: they use `t.Setenv`.

`NewFixture(t, withWorktree)` creates separate temporary home, library and
project directories, sets HOME, DAYPACK_HOME and XDG_CONFIG_HOME, and initializes
an isolated Git repository. An optional second worktree has a `.git` file.
The library includes a skill with a support file, an MCP definition containing
only an environment reference, a fragment, and an agent. Edit these files directly
for malformed-input tests; no library initialization command is required.

`RunCLI(t, f, args...)` calls the real CLI in-process with explicit library and
project overrides, a non-TTY reader, and separate stdout/stderr. Use
`RunCLIWithInput` for explicit stdin and terminal state. CLI directory-precedence
coverage also lives in `internal/cli` because no feature command exists yet to
expose resolved directories.

`SnapshotTree` records every descendant including hidden files without following
symlinks. `AssertTree` compares an exact tree, reporting sorted path differences.
For selected-path assertions, select entries from the snapshot explicitly; Git
internals vary, so do not golden the whole `.git` tree. No global ignore rules
hide JSON siblings, dotfiles, or secrets.

Keep goldens under the owning package's `testdata/`. `AssertGolden` compares exact
bytes; `AssertJSONGolden` canonicalizes object keys, preserves array order and
rejects trailing documents. Updates require an explicit test invocation, such as
`go test ./internal/testutil -update`. Review changes before saving them.
`NormalizeFixturePaths` replaces only explicitly passed fixture roots with stable
placeholders. It does not scrub secrets or reorder operations. Foundation examples
are hand-built Item lists and plan text/JSON, not production renderer output.

`FakeAdapter` declares shared `agents-skills-dir` at `.agents/skills` and unique
`test-skills-dir` at `.test/skills`, emitting deterministic symlink items without
filesystem I/O. Its descriptors remain local until the adapter feature supplies
the production interface. Reconciliation and restriction execution belong there.

Call `PlantSentinel(t, f)` before apply/status/import and `AssertNoSecret(t,
f.Project, sentinel, result.Stdout, result.Stderr)` afterward, on success and
error paths. The scanner checks every regular project file, including hidden and
untracked files, without following external symlinks, and checks link targets
for secrets.env exposure. Failure diagnostics contain locations, never the
sentinel. The private library secrets.env intentionally holds the synthetic value
outside the project. Foundation tests inject leaks directly; they never execute
MCP commands or resolve real credentials.

## Deferred coverage map

Implement these tests with their owning feature, mapping assertions to that
issue's acceptance criteria. Their absence does not block this foundation.

| Feature issue | Named tests / assertions |
| --- | --- |
| Apply `01M6RSR2FKVBKQHS28Z7J39NHB` | `TestApplyTwiceNoOp`, `TestManifestRemovalConverges`, `TestUnresolvedEntryPreservesManagedContent` |
| Ownership `01M6RSR2DX646G82T1EYZBD071` | `TestHandwrittenFilesAndJSONKeysSurvive`, `TestCollisionRefused`, `TestManagedDriftReported` |
| Status `01M6RSR2K5XRY9HB9Q0ZXCD3XM` | Consumes ownership drift reports; `TestStatusNoSecretValues` |
| Adapter contract `01M6RSR2MYB5VPVPS8WHTWTDQG` | `TestSharedTargetOnce`, `TestRestrictedEntries`, `TestRestrictedSharedTargetReaders`; adapter features add Item goldens |
| Plan/gate `01M6RSR2HBPNRJ10XJVE01GG3P` | `TestGatedInlineMCPPlan`, plan text/JSON goldens |
| Ownership/apply | `TestWorktreeExclusion`: worktree `.git` file and handwritten sibling exclusions |
| Apply/MCP adapters | `TestApplyNoSecretValues` |
| Import `01M6RSR256MFHE5THC4HAJJEK8` | `TestImportNoSecretValues` |

Library MCP is trusted. Inline MCP requires confirmation on first apply and when
command/args/url change. Apply tests must cover no-TTY behavior: skip gated
operations, apply other operations, return 3. Secret-safety scenarios use the
sentinel helpers on both success and error paths.

Claude MCP adapter tests cover the library/inline renderer and compose its items
with the existing lock APIs for adoption, drift, cleanup, tracking and worktree
exclusion. These are offline composition tests, not production apply/status CLI
coverage. Live Claude approval in a never-trusted folder and settings lookup in a
worktree remain separate manual checks requiring user approval.

### Claude Code MCP approval: UNVERIFIED

Whether Claude Code honours `enabledMcpjsonServers` from an untracked
`.claude/settings.local.json` in a never-trusted folder, and which
`settings.local.json` it reads inside a git worktree, has NOT been verified. The
adapter therefore always emits a static `mcp-approval` Note telling the user
that project MCP servers may need approval via `/mcp` (or
`enabledMcpjsonServers`) on first use. To verify later (requires user OK for a
live session):

1. Create a fresh folder outside any trusted path, `git init` it, and apply a
   manifest with one innocuous local MCP server so `.mcp.json` and
   `.claude/settings.local.json` are written.
2. Start `claude` in that folder. Record the Claude version, config paths and
   trust state. Observe whether the server is connected without an approval
   prompt or `/mcp` shows it pending.
3. From that repository run `git worktree add ../wt`, apply in the worktree (its
   own `.mcp.json` and `settings.local.json`), start `claude` there, and record
   whether approval is honoured and which settings file was read.
4. Log the observations in the issue; if approval is not honoured keep the Note
   (and let status print the `/mcp` hint), otherwise reconsider it.

## Verification

Run `go build ./...`, `go vet ./...`, `go test ./...`,
`golangci-lint run ./...`, and `prep check`. After caching dependencies, verify
`GOPROXY=off GOSUMDB=off go test -count=1 ./...`. Compilation may use the tool
cache; product-test HOME remains temporary. Never run live evals here.

CI pins golangci-lint v2.15.0 (release binary built with Go 1.27.0), compatible
with this module's Go 1.27 requirement. The linter FAQ requires a build using
at least the analyzed Go language version:
https://golangci-lint.run/docs/welcome/faq/#which-go-versions-are-supported
CI may fetch tools and modules; tests themselves do not access the network.
