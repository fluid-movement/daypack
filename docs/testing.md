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
for .env exposure. Failure diagnostics contain locations, never the
sentinel. The private library .env intentionally holds the synthetic value
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

### Claude Code MCP approval: verified 2026-11-09 (Claude Code 2.1.319)

Checked live with `claude mcp list` (no model call) using a throwaway stdio
server (`sleep 2`) that daypack applied into `.mcp.json` and an untracked
`.claude/settings.local.json` (`enabledMcpjsonServers: ["probe"]`):

- Trusted folder (a subdirectory of an already-trusted project): the server is
  approved from `settings.local.json` and Claude tries to connect (it fails only
  because `sleep` is not an MCP server). Control without the settings file:
  `Pending approval`.
- Git worktree of an already-trusted repository, with its own `.mcp.json` and
  `settings.local.json` written by apply: approved the same way; control without
  the settings file: `Pending approval`. So the worktree's own
  `settings.local.json` is the file read.
- Never-trusted folder, and a worktree of a never-trusted repository: `Pending
  approval` even with `enabledMcpjsonServers` present. Folder trust has to be
  granted first (the interactive trust dialog on first `claude` run, not
  exercised here); approval is honoured only afterwards.

Conclusion: approval entries work, but cannot bypass folder trust. The static
`mcp-approval` Note stays, because a fresh clone or worktree of an untrusted
repo still prompts. Not verified: the interactive first-run trust flow itself
and a real MCP server connecting.

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

## Codex documentation baseline and runtime limits

Researched 2026-11-09 against official OpenAI documentation (current project
layout, without a claim of compatibility with every installed CLI version):

- [Skills](https://learn.chatgpt.com/docs/build-skills): shared `.agents/skills`
  discovery and symlinked skill directories.
- [MCP](https://learn.chatgpt.com/docs/extend/mcp?surface=cli): project
  `.codex/config.toml`, `mcp_servers`, `env_vars`, `bearer_token_env_var`,
  `http_headers` and `env_http_headers`.
- [Config basics](https://learn.chatgpt.com/docs/config-file/config-basic) and
  [reference](https://learn.chatgpt.com/docs/config-file/config-reference):
  project config trust and precedence.
- [AGENTS.md](https://learn.chatgpt.com/docs/agent-configuration/agents-md):
  override precedence, directory discovery and default 32 KiB combined limit.
- [Subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents):
  standalone project `.codex/agents/*.toml`, required name, description and
  developer_instructions, optional model, inherited permissions/provider/MCP.

Offline tests cover renderer conformance and native TOML goldens; reference
capability rejection and secret sentinels; shared selection and last-reader
removal; table semantic hashes, quoted keys and descendants; user-byte
preservation, drift/adopt/force/eject, dry-run immutability, symlink containment,
symlinked library roots, and tracked/untracked excludes. The built daypack binary
can also be exercised on a realpath temporary project without launching Codex.

The following runtime behavior remains **UNVERIFIED**: installed Codex version
compatibility; project config skipping in trusted/untrusted interactive and
headless sessions; real custom-agent discovery/invocation; real MCP startup and
authentication. Generated config parsing does not verify those behaviors. No live
Codex eval is authorized or run by this implementation.

Future live verification requires the user's OK: record the Codex version, use
disposable trusted and untrusted projects with innocuous local MCP servers and
synthetic environment credentials, compare interactive/headless discovery,
explicitly invoke the translated custom agent and observe inherited permissions,
and verify MCP startup/authentication without exposing credentials. Do not modify
real trust decisions or use real credentials for these checks.
