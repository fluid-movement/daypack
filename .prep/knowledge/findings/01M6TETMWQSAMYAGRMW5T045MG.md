---
type: finding
title: Claude Code project conventions (skills, agents, instructions, MCP, scopes)
description: Exact paths, formats, precedence and approval mechanics for Claude Code v2.1.319 project config, for the daypack adapters.
date: "2026-11-07"
issue: 01M6RVX52JWVSGVS8C6QMHTH6A
---

# Claude Code project conventions

Verified 2026-11-07 against docs and a local install, Claude Code 2.1.319. Local checks were read-only and key names only. No secret values were read.

Sources (all fetched 2026-11-07):
- https://code.claude.com/docs/en/memory
- https://code.claude.com/docs/en/skills
- https://code.claude.com/docs/en/sub-agents
- https://code.claude.com/docs/en/mcp
- https://code.claude.com/docs/en/settings
- https://code.claude.com/docs/en/claude-directory

The skills, sub-agents and MCP pages were truncated at about 100k chars by the fetcher. The unread tails are noted under UNVERIFIED.

## 1. Skills

Layout: `<root>/<name>/SKILL.md`, plus optional sibling files that the skill references.

| Scope | Path |
|---|---|
| Project | `<repo>/.claude/skills/<name>/SKILL.md` |
| Personal | `~/.claude/skills/<name>/SKILL.md` |
| Nested | `<subdir>/.claude/skills/<name>/SKILL.md`. Loads lazily, the first time Claude reads or edits a file under `<subdir>`. |
| Plugin | `<plugin>/skills/<name>/SKILL.md`, invoked as `/plugin:name` |
| Synced from claude.ai | `~/.claude/skills/synced/` |

- Project skills are discovered from the start dir up to the git root. In a linked worktree the search stops at the worktree root.
- Frontmatter: all fields are optional and `description` is recommended. Fields: `name`, `description`, `when_to_use`, `argument-hint`, `arguments`, `disable-model-invocation`, `user-invocable`, `allowed-tools`, `disallowed-tools`, `model`, `effort`, `context`, `agent`, `background`, `hooks`, `paths`, `shell`, `metadata`, `license`, `compatibility`.
- Unknown or misspelled fields are silently ignored.
- Minimal example:
```
---
description: Summarizes uncommitted changes. Use when asked what changed.
---
Body instructions...
```
- Symlinks: the docs say a `<name>` entry in the project or personal location "can be a symlink to a directory elsewhere on disk". It is loaded once even if several locations point at the same target. This is a per-skill-directory symlink, not a symlink of `.claude/skills` itself.
- LOCALLY VERIFIED: `~/.claude/skills/agent-browser -> ../../.agents/skills/agent-browser` is a relative dir symlink. That skill is listed as available in the running 2.1.319 session. Real dirs (`agent-pane`, `ai-usage`, `orchestrate`, `rex`) sit next to it.
- `.agents/skills`: NOT read by Claude Code. The skills docs never mention it, and a web search found no evidence of support. Adapter plan: keep canonical skills in `.agents/skills/<name>/` and create per-skill symlinks `.claude/skills/<name> -> ../../.agents/skills/<name>`. A relative target is portable.
- Name conflicts: enterprise > personal > project. A same-named project-root and nested skill both load, and the nested one gets a path-qualified name such as `/apps/web:deploy`. A skill in `.claude/skills` beats a same-named `.claude/commands/` file.
- Live reload: edits under existing skill dirs apply in-session. Creating a top-level skills dir that did not exist at session start needs `/reload-skills`.
- The docs name no project-local gitignored variant for skills. Project skills are meant to be committed. daypack will git-exclude them via `.git/info/exclude` per its decision.

## 2. Subagents

Path: `<repo>/.claude/agents/<anything>.md`, or `~/.claude/agents/`. Scanned recursively. Identity comes from the `name` field, not the filename or folder.

Format: YAML frontmatter, then a markdown body that is the system prompt.
```
---
name: code-reviewer
description: Reviews code for quality. Use proactively after changes.
tools: Read, Glob, Grep
model: sonnet
---
You are a code reviewer. ...
```
- Required: `name`, `description`.
  - `name`: max 256 chars, no `:`, must not start with `-`.
  - A file is silently skipped if: no `name`, the opening `---` is not on line 1, `name` has no `description`, or the YAML does not parse. Check with `claude plugin validate <dir>` (v2.1.233+).
- Optional: `tools` (comma string or YAML list), `disallowedTools`, `model` (`sonnet|opus|haiku|fable|<full id>|inherit`), `permissionMode`, `mcpServers` (names of configured servers, or inline definitions keyed by name), `skills` (preloaded, up to 32), `hooks`, `memory` (`user|project|local`), `maxTurns`, `effort`, `isolation: worktree`, `background`, `omitClaudeMd`, `initialPrompt`, `color`, `autoCompactWindow`, `experimental`.
- Multi-word fields are camelCase. Misspelled fields are silently ignored.
- Precedence (highest first): managed > `--agents` CLI JSON > `.claude/agents/` > `~/.claude/agents/` > plugin.
  - The project dir is searched from the cwd up to the repo root. For a duplicate name, the nearest dir wins.
  - Duplicate names inside one tree are resolved by filesystem order, which is undocumented. daypack must enforce unique names.
- Loading: at session start, with a file watcher on existing dirs. A restart is needed if the first agent file creates a previously nonexistent dir.
- Per-project gitignored variant: none for the agent files. The `memory: local` option stores agent memory in a local, non-shared location (see sub-agents#enable-persistent-memory).
- `.claude/agents/` is not read from `.agents/`. Generate the files into `.claude/agents/`.

## 3. Instructions: AGENTS.md, CLAUDE.md, CLAUDE.local.md

Files and scope:
- `./CLAUDE.md` or `./.claude/CLAUDE.md`: project, committed.
- `./CLAUDE.local.md`: private per-project. Loads alongside CLAUDE.md. The docs say "add to .gitignore"; Claude Code does not do it for you.
- `~/.claude/CLAUDE.md`: user.
- Managed CLAUDE.md: org policy. It cannot be excluded.

Loading rules:
- CLAUDE.md and CLAUDE.local.md are loaded from the cwd and every ancestor dir at launch. They are concatenated, ordered from the filesystem root down to the cwd, so the nearest file wins ties by position. Nothing is overridden.
- Subdir CLAUDE.md files load on demand, once Claude touches a file in that subdir.
- `.claude/rules/*.md` loads recursively. Rules without `paths:` frontmatter load at launch.
- `claudeMdExcludes` (a settings key with absolute-path globs) skips files. Arrays merge across layers.

Native AGENTS.md support (v2.1.277+; the installed 2.1.319 is newer):
- Default setting is `claude-md-or-agents-md`. Claude reads `AGENTS.md` (and `.claude/AGENTS.md`) from the cwd and ancestors only when there is NO `CLAUDE.md`, `.claude/CLAUDE.md` or `CLAUDE.local.md` in the cwd or above it. If any CLAUDE file exists, AGENTS.md is ignored by default.
- `~/.claude/CLAUDE.md` and `.claude/rules/` do not count for that check.
- Subdir AGENTS.md loads on demand, via the Read tool, if that subdir has no CLAUDE.md of its own.
- Setting (in `pluginConfigs` in `~/.claude/settings.json`, `--settings`, or similar):
  `{"pluginConfigs":{"cc-plugin-agents-md@builtin":{"options":{"instructionFiles":"claude-md-and-agents-md"}}}}`
  Valid values: `claude-md-or-agents-md` (default), `claude-md-and-agents-md`, `claude-md`, `managed-only`. The docs also mention the older ID `agents-md@builtin` before v2.1.285.
- AGENTS.md differences: `InstructionsLoaded` hooks do not fire for it, and external `@` imports in it load only if external imports were already approved.
- AGENTS.md read can be unavailable (built-in plugin disabled, some Bedrock or telemetry-off sessions before v2.1.281, first session after upgrade).

RECOMMENDATION for daypack: the portable mechanism is a committed-or-excluded `CLAUDE.md` whose first line is `@AGENTS.md`. The docs name this as the supported way to share one file with other tools:
```
@AGENTS.md

## Claude Code
(claude-specific additions)
```
- Alternative: `ln -s AGENTS.md CLAUDE.md`.
- With `@AGENTS.md` imported, Claude never reads AGENTS.md twice, whichever setting is used.
- Without CLAUDE.md, a user's own CLAUDE.local.md silently disables native AGENTS.md reading unless the setting is `claude-md-and-agents-md`. This is a trap, so the stub is more robust.
- Since only the managed section of AGENTS.md is committed, the generated `CLAUDE.md` stub (`@AGENTS.md`) should be git-excluded. UNVERIFIED trade-off: if a repo already has a hand-written CLAUDE.md, daypack must not overwrite it. It should append `@AGENTS.md` if it is missing, and ask the user.
- Import syntax: `@path`, relative to the importing file or absolute. Max recursion is 4 hops (the docs say "four hops"). Imports inside code spans or fenced blocks are not expanded. Imports resolving outside the working dir trigger a one-time per-project approval dialog (project-level files only; user-level `~/.claude/CLAUDE.md` imports load without a prompt).

## 4. MCP

Scopes:

| Scope | Stored in | Shared |
|---|---|---|
| local (default for `claude mcp add`) | `~/.claude.json` under `projects["<abs path>"].mcpServers` | no |
| project | `<repo>/.mcp.json` | via VCS |
| user | `~/.claude.json` top-level `mcpServers` | no |

- Precedence for a same-named server: local > project > user > plugin > claude.ai connectors. The whole entry from the winning source is used, with no field merge. `managedMcpServers` (v2.1.259+) outranks all.
- `.mcp.json` at the repo root only. It is not read from `.claude/`.

`.mcp.json` format:
```json
{"mcpServers":{
  "docs":{"type":"http","url":"${DOCS_URL:-https://x.example/mcp}",
          "headers":{"Authorization":"Bearer ${DOCS_TOKEN}"}},
  "db":{"command":"npx","args":["-y","some-server","--flag"],
        "env":{"DB_URL":"${DB_URL}"}}
}}
```
- Types: `stdio` (default when `type` is absent; needs `command`, `args`, `env`), `http` (alias `streamable-http`), `sse` (deprecated), `ws`. A `url` entry with no `type` is an error.
- LOCALLY VERIFIED shapes, key names only:
  - `~/.claude.json` top-level `mcpServers` has entries with keys `type`, `url` and sometimes `headers`.
  - A project `.mcp.json` in `/Users/azaharias/coding/web/molkky` has `mcpServers.<name>` with keys `type` and `url`, with `type` = `http`.
  - `projects["<path>"].mcpServers.<name>` has keys `type`, `url`, `headers`.
- Env expansion: `${VAR}` and `${VAR:-default}`, allowed in `command`, `args`, `env`, `url`, `headers`.
  - Unset with no default: the server still loads, a warning appears in `claude mcp list` and `/mcp`, and the literal `${VAR}` is passed through.
  - A set-but-empty variable counts as set, so the default is not used.
  - Some credential-named vars (`ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `AWS_BEARER_TOKEN_BEDROCK`, `HTTPS_PROXY`, `NPM_TOKEN`) expand to EMPTY inside remote `url` and `headers`.
  - daypack implication: env-var-reference secrets in `.mcp.json` work, so generated files hold references only. Avoid those reserved names.
- Approval of project `.mcp.json` servers:
  - Interactive sessions prompt once per server. The status before approval is "Pending approval".
  - Choices are stored as setting keys:
    - `enabledMcpjsonServers`: array of server names.
    - `disabledMcpjsonServers`: array, and any entry in any settings file blocks the server in all modes.
    - `enableAllProjectMcpServers`: boolean.
  - LOCALLY VERIFIED: `/Users/azaharias/coding/web/molkky/.claude/settings.local.json` has top-level keys `permissions` and `enabledMcpjsonServers`. That file is globally git-ignored via `~/.config/git/ignore` (`**/.claude/settings.local.json`). Each `~/.claude.json` project entry also carries (empty) `enabledMcpjsonServers` and `disabledMcpjsonServers` keys. Those look like a legacy location (UNVERIFIED which wins).
  - `claude mcp reset-project-choices` resets the choices.
  - Without hand approval: write `.claude/settings.local.json` (per-user, git-ignored):
    `{"enabledMcpjsonServers":["name1","name2"]}`
    or `{"enableAllProjectMcpServers":true}`.
  - The same keys in the committed `.claude/settings.json` are IGNORED in an untrusted folder (server stays Pending). Approvals from `~/.claude/settings.json`, managed settings, and `--settings` still apply in untrusted folders. `.claude/settings.local.json` trust behavior is not stated for these keys. See UNVERIFIED.
  - Non-interactive `claude -p`, Agent SDK and cloud sessions load project servers WITHOUT prompting.
  - Recommended daypack wiring: merge `enabledMcpjsonServers` into `.claude/settings.local.json` (JSON merge, keep other keys, ensure it is git-excluded) and leave the user's own approvals intact. Alternative: write the servers directly to local scope in `~/.claude.json`. That is not recommended because it is Claude's private state file.
- CLI: `claude mcp add [-s local|project|user] [--transport http|sse|stdio] [-e K=V] [-H "K: V"] <name> <url|-- cmd args>`, `add-json <name> '<json>'`, `list`, `get`, `remove`, `reset-project-choices`. Using `--scope project` writes `.mcp.json`.
- Subagents can scope MCP servers with the `mcpServers:` frontmatter key.

## 5. Gitignored local variants and precedence

| Item | Project, shared | Local / private | User |
|---|---|---|---|
| Instructions | `CLAUDE.md`, `.claude/CLAUDE.md` | `CLAUDE.local.md` (user must gitignore) | `~/.claude/CLAUDE.md` |
| Settings | `.claude/settings.json` | `.claude/settings.local.json` (Claude auto-adds it to the GLOBAL git excludes only when Claude itself first writes it; hand-created files must be ignored by the user) | `~/.claude/settings.json` |
| MCP | `.mcp.json` | local scope in `~/.claude.json` | `~/.claude.json` `mcpServers` |
| Skills | `.claude/skills` | none | `~/.claude/skills` |
| Agents | `.claude/agents` | none | `~/.claude/agents` |

- Settings precedence, highest first: managed > `--settings` CLI > `.claude/settings.local.json` > `.claude/settings.json` > `~/.claude/settings.json`. List keys such as `permissions.allow` merge across files.
- Skills precedence: enterprise > personal > project. This is the OPPOSITE of settings and MCP, where project/local beats user.
- Agents precedence: project > user. MCP precedence: local > project > user.
- daypack should write its git-ignores to `.git/info/exclude` and not rely on Claude's global-excludes behavior.
- `settings.local.json` is written at the git root, even when Claude Code is started in a subdirectory (v2.1.211+). In a worktree it is written at the main checkout root (see settings docs). Plan for that when writing it.

## 6. User-level (global) locations, for the import scan
- Skills: `~/.claude/skills/<name>/SKILL.md`. LOCALLY VERIFIED entries here are a mix of real dirs and symlinks to `../../.agents/skills/<name>`. `~/.claude/skills/synced/` holds claude.ai-synced skills (contains a `.bucket-*` marker file and a UUID dir) and should be skipped by the scan.
- Agents: `~/.claude/agents/*.md`. This dir does not exist on this machine, which is fine.
- MCP: user scope is `~/.claude.json` `.mcpServers`. Local scope is `~/.claude.json` `.projects["<abs path>"].mcpServers`. Entries may include `headers` with literal secrets, so the scan must treat `headers`, `env` and `args` values as secrets, convert them to references, and never echo them.
- Plugins: `~/.claude/plugins/installed_plugins.json`, `known_marketplaces.json`, and per-plugin skills, agents and `.mcp.json`. Out of scope for v1 import unless wanted.
- Also in `~/.claude.json` per project: `hasTrustDialogAccepted`, `allowedTools`, `enabledMcpjsonServers`, `disabledMcpjsonServers`, `mcpContextUris`.
- Also read: `~/.claude/settings.json`.
- Not stored in `~/.claude` (the docs list `CLAUDE_CONFIG_DIR` as relocating all of it): honor `CLAUDE_CONFIG_DIR` when set, including for `.claude.json`.

## 7. Hooks
Out of v1 per the decision. The only note for later: `.claude/settings.json` `hooks`, and hooks inside skill, agent and plugin frontmatter.

## UNVERIFIED (with what would settle it)
1. Whether `enabledMcpjsonServers` and `enableAllProjectMcpServers` are honored from an untracked `.claude/settings.local.json` in a never-trusted folder. The docs say only that the committed `.claude/settings.json` is ignored in untrusted folders. The molkky example shows `.claude/settings.local.json` is where Claude itself persists approvals, but that folder has the trust dialog accepted. Settle by a fresh clone and `claude mcp list` in a scratch dir after writing the key, or by reading the settings-reference entries for these two keys.
2. Which of `~/.claude.json` `projects[path].enabledMcpjsonServers` (present, empty) and `.claude/settings.local.json` `enabledMcpjsonServers` is authoritative today. Settle by testing both in a scratch repo.
3. Whether a symlinked `.claude/agents/<file>.md` is followed. The docs document symlinks only for skills dirs. Settle by testing; otherwise copy the file.
4. Whether a symlink of the whole `.claude/skills` directory works. The docs only promise per-skill dir symlinks.
5. Pages were truncated after about 100k chars (skills, sub-agents, mcp). Unread tails could hold extra caveats, for example about the `.mcp.json` `${VAR}` details in plugin context, or about sub-agents listing.
6. No evidence either way for `.agents/skills`; treated as unsupported. Settle by checking the llms.txt index or the changelog for "agents/skills".
7. A project-level gitignored variant for skills, agents or `.mcp.json` does not exist in the docs. `.mcp.local.json` was never seen. Treat that as nonexistent.
8. The exact on-disk effect of the `claude-md-and-agents-md` option when `CLAUDE.md` already exists was not tested at runtime. It is documented only.
