---
type: finding
title: 'Pi (earendil-works) project conventions: skills, MCP, AGENTS.md, cross-harness .agents/skills matrix'
description: Verified Pi 1.2.0 paths/formats for .agents/skills, .pi/mcp.json, AGENTS.md, env expansion, trust gating, plus other harnesses' support
date: "2026-11-07"
issue: 01M6RVX5BR6J3T9KRKD8WXS548
---

# Pi project conventions (verified against Pi 1.2.0 installed locally, 2026-11-07)

## Which project
"Pi" = the Pi coding agent by Mario Zechner, npm `@earendil-works/pi-coding-agent`, repo https://github.com/earendil-works/pi (packages/coding-agent), docs https://pi.dev/docs. Installed: `/Users/azaharias/.local/bin/pi`, `pi --version` = 1.2.0, runtime under `~/.pi/agent/install/releases/1.2.0/`.
PRIMARY SOURCE: the docs bundled with that install, `~/.pi/agent/install/releases/1.2.0/node_modules/@earendil-works/pi-coding-agent/docs/{mcp,skills,configuration,security,cli,settings}.md` (the pi.dev/docs/*.md URLs 404 via fetch; https://pi.dev/docs index lists the same pages). Behaviour below marked [src] was confirmed by reading the installed JS (`dist/core/{skills,package-manager,resource-loader,resolve-config-value}.js`).
Agent dir `<agent-dir>` = `~/.pi/agent`, overridable with env `PI_CODING_AGENT_DIR`.
Note: `~/.pi/` itself (outside `agent/`) is NOT config; only `~/.pi/agent/` is.

## Paths
| Purpose | User level | Project level |
|---|---|---|
| Skills (Pi native) | `~/.pi/agent/skills/` | `<project>/.pi/skills/` |
| Skills (shared Agent Skills) | `~/.agents/skills/` | `<dir>/.agents/skills/` for cwd and each ancestor, stopping at the git repo root (dir containing `.git`) when there is one; otherwise up to filesystem root |
| MCP servers | `~/.pi/agent/mcp.json` | `<project>/.pi/mcp.json` (cwd only, no ancestor walk documented) |
| Settings | `~/.pi/agent/settings.json` | `.pi/settings.json` |
| Instructions/context | `~/.pi/agent/AGENTS.md` (also `AGENTS.override.md`, `AGENTS.MD`, `CLAUDE.md`, `CLAUDE.MD`) | `AGENTS.md` in cwd AND every ancestor dir |
| System prompt | `~/.pi/agent/SYSTEM.md`, `APPEND_SYSTEM.md` | `.pi/SYSTEM.md`, `.pi/APPEND_SYSTEM.md` (project wins, not merged) |
| Trust store | `~/.pi/agent/trust.json` | - |
| MCP OAuth tokens | `~/.pi/agent/mcp-auth.json` | - |

## Project trust (important for daypack)
Pi gates protected project resources behind "project trust": `.pi/settings.json`, `.pi/mcp.json`, `.pi/{extensions,skills,prompts,themes}`, `.pi/SYSTEM.md`, `.pi/APPEND_SYSTEM.md`, AND project `.agents/skills` in cwd or ancestors. A bare `.pi/` dir does not trigger a prompt. Decision order: CLI `--approve`/`--no-approve`; extension `project_trust` event; saved decision in `~/.pi/agent/trust.json` for cwd or nearest parent (`/trust` saves); setting `defaultProjectTrust` = `"ask"` (default) | `"always"` | `"never"`. In print/json/rpc modes with no override and `ask`/`never`, protected resources are SKIPPED silently. [src: docs/security.md, package-manager.js `projectTrusted ? collectAncestorAgentsSkillDirs : []`]
Context files (AGENTS.md/CLAUDE.md) load regardless of trust. User-level paths (`~/.agents/skills`, `~/.pi/agent/*`) never need trust.
Consequence: generated project-level `.pi/mcp.json` and `.agents/skills` only take effect in trusted projects; daypack should say so (and may print `pi` `/trust` hint). Docs say credentialed servers belong in user-level file.

## Skills
Format: Agent Skills spec (https://agentskills.io/specification). Dir containing `SKILL.md` with YAML frontmatter:
```
---
name: pdf-tools            # lowercase letters/digits/hyphens, <=64 chars; dir-name match not enforced by Pi but portable
description: What it does and when to use it.   # required, <=1024; missing description => skill not loaded
---
# body
```
Other frontmatter: `license`, `compatibility`, `metadata`, `allowed-tools` (experimental), `disable-model-invocation: true` (only via `/skill:name`). Invalid fields warn only; malformed SKILL.md not loaded.
Discovery: recursive under each skills root, a directory with SKILL.md is a leaf (no descent). Skips dot-entries (other than root) and `node_modules`. HONOURS `.gitignore`, `.ignore`, `.fdignore` inside the skills dir tree [src skills.js IGNORE_FILE_NAMES] -> a generated skill tree must not be ignored by a .gitignore pattern that applies within it. `/reload` re-scans; `/skill:name` forces activation.
Symlinks: SUPPORTED [src]: symlinked SKILL.md files and symlinked skill directories are followed via `statSync` (broken symlinks silently skipped). Duplicate real paths (same file via two symlinks/roots) are de-duplicated silently. Docs do not state this explicitly; confirmed from code in 1.2.0.
Precedence/collisions: same `name` => FIRST discovered wins, loser reported as "collision" diagnostic (not an error). Registration order [src package-manager.js ~2030-2065]: project `.pi/skills` -> project `.agents/skills` (cwd first, then ancestors outward) -> user `~/.pi/agent/skills` -> user `~/.agents/skills`. So project beats user, matching the agentskills.io convention. (UNVERIFIED: that the later merge in skills.js preserves exactly this order; settle with two same-named skills at project and user level and `/skill:` / startup diagnostics.)
Extra locations: `settings.json` `skills: string[]` (files or dirs), Pi packages (https://.../docs/packages.md), `--skill`-style CLI options.

## MCP
Built in since Pi 1.x (docs/mcp.md): stdio and streamable HTTP; legacy SSE rejected. File: JSON, top-level `mcpServers` (same as Claude Desktop/Code/Cursor shape; docs say "Copy the existing mcpServers entry").
```json
{
  "mcpServers": {
    "filesystem": { "command": "npx", "args": ["-y", "@modelcontextprotocol/server-filesystem", "."],
                    "env": { "TOKEN": "${MY_TOKEN}" }, "cwd": "~/x", "description": "one sentence" },
    "docs": { "url": "https://example.com/mcp",
              "headers": { "Authorization": "Bearer ${DOCS_TOKEN}" } }
  }
}
```
Fields: stdio `command` (single executable, NOT a shell string) / `args` / `env` / `cwd`; HTTP `url` / `headers` / `oauth`; common `type` (optional; `stdio`|`http`|`streamable-http`; inferred from command/url; `sse` rejected), `timeout` (seconds, default 60), `enabled` (false keeps entry, no connect), `exposure` (`codemode` default | `deferred` | `direct` | `hidden`; alias `codemode-deferred`), `toolExposure` (map of tool name or `*` pattern -> exposure; exact beats pattern, first pattern wins), `description`. Leading `~/` expands to home in command/args/cwd. Server names: letters, digits, `_`, `-` only; names differing only by `-` vs `_` collide. Tools surface as `mcp__<server>__<tool>`.
Default exposure `codemode` means tools are NOT declared to the model directly; they are reached via the `codemode` script tool / `tool_search`. daypack should consider setting `"exposure": "direct"` (or deferred) if parity with Claude Code (tools visible) is wanted, and always emit `description`.
Precedence user vs project: project entry with the same server name REPLACES the user entry entirely. Exception: a project entry with none of `command`/`url`/`type` is a partial override of only `enabled`, `exposure`, `toolExposure`, keeping user `env`/`headers`/`auth`. Invalid entries are reported and skipped; other servers still connect. Project file read only after trust.
Env expansion: YES. `env` and `headers` values accept `${VAR}` (also bare `$VAR`); `$$` = literal `$`, `$!` = literal `!`. [src resolve-config-value.js: name must match `[A-Za-z_][A-Za-z0-9_]*`; lookup is the server's configured env, then `process.env`; empty string counts as unset.] OAuth `clientSecret` also accepts env/command. Expansion confirmed documented for `env`, `headers`, `oauth.clientSecret`. NOT documented for `url`, `command`, `args`, `cwd` -> do not rely on `${VAR}` there (UNVERIFIED).
Command helper (equivalent to headersHelper): YES, per-value. A value that starts with `!` runs the rest as a shell command and uses stdout; command must be the WHOLE value: `"Authorization": "!echo Bearer $(gh auth token)"`. Works in `env` and `headers`. Results are cached for the process lifetime [src]. This is finer-grained than Claude Code's single `headersHelper` script. Other auth: `pi mcp add ... --bearer-token-env-var NAME` writes `Authorization: Bearer ${NAME}`; OAuth with dynamic registration needs no config (`pi mcp login <server>`; tokens in `~/.pi/agent/mcp-auth.json`); `oauth: {clientId, clientSecret, callbackPort|callbackUrl, scope, clientName, clientRegistration:"cimd", authServerMetadataUrl}`.
Behaviour when a referenced env var is unset: resolver returns undefined; whether the server is skipped, errors, or gets an empty value is UNVERIFIED (settle: `FOO= pi mcp list --json` with an entry using `${NOPE}`).
CLI (works without a session, usable by daypack): `pi mcp add <name> [-l|--local] [--env K=V] [--cwd d] [--exposure m] [--description t] -- cmd args...`, `pi mcp add <name> --url U [--header K=V] [--bearer-token-env-var N] [--oauth-*]`, `pi mcp remove <name>`, `pi mcp list [--json]` (connects; exit 1 on invalid/unconnected; good verify step), `pi mcp login|logout`. `-l` writes `./.pi/mcp.json`. Run `/reload` in a live session after file changes.
Disabling/replacing: `--no-mcp`; an installed extension that registers `/mcp` (e.g. `pi-mcp-adapter`) REPLACES built-in MCP and then Pi does not read mcp.json at all; `"extensions": ["-builtin:mcp"]` in settings disables it. Third-party doc claiming `.mcp.json` is read by Pi (docs.rs agent-config) applies to pi-mcp-adapter, not the built-in support: Pi built-in does NOT read a root `.mcp.json`.

## AGENTS.md
Pi loads, per directory, the FIRST existing of `AGENTS.override.md`, `AGENTS.md`, `AGENTS.MD`, `CLAUDE.md`, `CLAUDE.MD` (so AGENTS.md beats CLAUDE.md in the same dir) [src loadContextFileFromDir]. Sources: `~/.pi/agent/` first, then every ancestor from filesystem root down to cwd (outermost first, nearest last); all are concatenated (not replaced), except `AGENTS.override.md` replaces only AGENTS.md/CLAUDE.md in the SAME dir. Linked git worktrees: a worktree's own AGENTS.md shadows the main repo's to avoid double-loading. No project-trust required. Pi does NOT read nested subdirectory AGENTS.md below cwd (only cwd and ancestors). Plain markdown, no frontmatter, no `@import` mechanism documented (UNVERIFIED for `@file` includes; settle by test). `--no-context-files` style flag exists to disable (exact flag name UNVERIFIED).
Implication for daypack managed section: a marked section in project `AGENTS.md` is read by Pi as-is. If both AGENTS.md and CLAUDE.md exist, Pi reads only AGENTS.md; Claude Code reads only CLAUDE.md (see matrix), so CLAUDE.md should `@AGENTS.md`-import or the managed section be written to both.

## Local install observations (read only, no secret values printed)
- `~/.pi/agent/mcp.json` exists with one server `radius` (url `https://radius.pi.dev/mcp`, keys `url`, `auth.provider`, `enabled`). Note `auth.provider` is not in the documented schema -> Pi may write extra keys; daypack must preserve unknown keys and unmanaged servers when editing the file.
- `~/.pi/agent/` contains `settings.json` (keys: defaultModel, defaultProvider, theme, packages, ...), `auth.json`, `models.json`, `skills/` (extension-guide, research, teach), `extensions/`, `sessions/`; no `AGENTS.md` there (`~/.pi/AGENTS.md` exists but is outside the agent dir, not read as global context).
- `~/.agents/skills/` contains real directories (agent-browser, find-skills, rex, stack, svelte-rules); not symlinks.

## Cross-harness reference (project `.agents/skills` and AGENTS.md)
Skill paths (S) / AGENTS.md (A). "walk" = ancestors up to repo root.
| Harness | project .agents/skills | user ~/.agents/skills | symlinked skills | AGENTS.md | Source |
|---|---|---|---|---|---|
| Pi 1.2 | yes, cwd + ancestors to git root (needs project trust) | yes | yes [src] | yes, cwd+ancestors+user dir; else CLAUDE.md | local docs |
| Codex | yes, every dir cwd -> repo root | yes | yes (followed) | yes (origin of the spec) | https://learn.chatgpt.com/docs/build-skills (redirect of developers.openai.com/codex/skills) |
| opencode | yes (compat), cwd+parents to project root | yes | not stated | yes | https://opencode.ai/docs/skills/ (also reads .claude/skills; precedence: .claude < .agents < ~/.config/opencode < .opencode < config) |
| Gemini CLI | yes, workspace alias, beats `.gemini/skills` | yes (alias of ~/.gemini/skills, beats it) | not stated | NOT by default; needs `context.fileName` in `.gemini/settings.json` (agents.md says "requires config") | https://geminicli.com/docs/cli/skills/ , https://agents.md |
| Cursor | yes, also nested dirs | yes | not stated | yes (listed at agents.md; not on skills page) | https://cursor.com/docs/context/skills , https://agents.md |
| GitHub Copilot | yes (also .github/skills, .claude/skills) | yes (also ~/.copilot/skills) | not stated | listed at agents.md (coding agent) | https://docs.github.com/en/copilot/concepts/agents/about-agent-skills |
| Claude Code | NO (only `.claude/skills`, `~/.claude/skills`, nested `.claude/skills`) | NO | yes (skill dir may be a symlink; loaded once) | NO natively: reads CLAUDE.md only (use `@AGENTS.md` import in CLAUDE.md) | https://code.claude.com/docs/en/skills |
| Others listed at agents.md as AGENTS.md readers | - | - | - | Jules, Factory, Aider (needs `read: AGENTS.md`), goose, Zed, Warp, VS Code, Devin, Junie, Amp, RooCode, Kilo Code, Windsurf, Augment, Semgrep, Ona | https://agents.md |
Convention source: https://agentskills.io/client-implementation/adding-skills-support recommends scanning `<project>/.agents/skills/` and `~/.agents/skills/` for interop, "project overrides user", gate project skills on trust, optionally respect .gitignore.
Takeaway for the shared `.agents/skills` adapter: one directory serves Pi, Codex, opencode, Gemini CLI, Cursor, Copilot; Claude Code needs its own `.claude/skills` target (symlink or copy; Claude Code and Pi both follow symlinks, so `.claude/skills/<n>` -> `../../.agents/skills/<n>` or a shared store is viable). Because several harnesses and Claude Code (if both targets exist) scan overlapping dirs, expect duplicate-name situations: Pi dedupes by real path (silent) and by name (warning); opencode takes the later source.

## Header/command helper summary
Claude Code `headersHelper` (script returning headers JSON) -> Pi: per-value `"!command"` in `headers`/`env` (stdout, cached for process lifetime), plus `${VAR}`. Mapping rule for daypack (env-var source only in v1): emit `"Authorization": "Bearer ${VAR}"` and `"env": {"K": "${VAR}"}`; no helper needed; hooks out of v1.

## UNVERIFIED (with what would settle it)
1. Exact merged precedence order of same-named skills across project `.pi/skills`, project `.agents/skills`, user dirs (read from code order, not run). Settle: create same-named skill in each, start `pi`, read collision diagnostics.
2. Behaviour for unset `${VAR}` in mcp.json env/headers (skip, error, empty). Settle: `pi mcp list --json` with unset var.
3. Whether `${VAR}`/`$VAR` expands in `url`, `command`, `args`, `cwd` (docs only promise `env`, `headers`, `oauth.clientSecret`). Settle: test `pi mcp list`.
4. Whether `.pi/mcp.json` is searched in ancestor dirs (docs say "project servers from `.pi/mcp.json`"; trust docs say "from the current working directory"). Settle: run pi in a subdir of a project with `.pi/mcp.json` at root.
5. `@file` import support inside AGENTS.md for Pi; exact CLI flag to disable context files.
6. Symlink support documented only implicitly for Pi; behaviour read from 1.2.0 code (statSync follow). Symlink support for opencode, Gemini CLI, Cursor, Copilot not stated in docs.
7. Codex/Cursor/opencode AGENTS.md precedence details and Gemini `context.fileName` exact key were not fetched in depth; only agents.md summary used.
8. Whether Claude Code reads AGENTS.md: skills page does not say; widely used CLAUDE.md `@AGENTS.md` import not re-verified here (check https://code.claude.com/docs/en/memory).
9. Pi versions before 1.2 / the user's claim "native MCP since 1.0" not checked against changelog (`~/.pi/agent/install/releases/1.2.0/node_modules/@earendil-works/pi-mcp/CHANGELOG.md` and pi-coding-agent CHANGELOG.md would settle).
