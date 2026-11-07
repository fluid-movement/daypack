---
title: Managed-vs-handwritten ownership tracking
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
depends_on:
  - 01M6RSRR7P0GKP661AK1RHJK0A
tags:
  - apply
priority: high
---

FR-11, FR-12, FR-14 basis. The ownership model is what makes 'hand-written config survives every apply' hold by construction. This issue defines the lockfile, the managed-item kinds, drift detection and git exclusion, as a Go package used by apply and status.

Decisions this implements: per-project lockfile, refuse collisions, git-excluded (decision 'Ownership: per-project lockfile, refuse collisions, git-excluded'); generated harness config is git-excluded (decision 'Generated harness config is git-excluded, never committed'); AGENTS.md managed section and pre-existing tracked .mcp.json are the exceptions (decisions 'AGENTS.md managed section is committed' and 'Claude Code MCP: entry-level ownership...').

Lockfile: .daypack/lock.toml at the project root (the .daypack directory itself is git-excluded). Format:
```toml
version = 1
[[item]]
target = "claude-skills-dir"        # target ID declared by an adapter
entry  = "skill:svelte"
kind   = "symlink"                  # symlink | file | json-entry | json-array-element | md-section
path   = ".claude/skills/svelte"
link   = "/home/u/.config/daypack/skills/svelte"   # symlink: recorded target
# file: sha256 of written bytes; json-entry: key path + sha256 of canonical JSON value (sorted keys, no whitespace);
# json-array-element: array key + element value (e.g. key = "enabledMcpjsonServers", value = "context7");
# md-section: marker name + sha256 of section body
hash   = "..."
file   = ".mcp.json"                # json-entry / md-section only
key    = "mcpServers.context7"      # json-entry only
tracked = false                      # true for items intentionally left visible to git
```
Rules:
- Apply only creates, changes or removes items listed in (or newly added to) the lockfile; nothing else is ever touched.
- Collision: a path, JSON key or section that exists but is not in the lockfile makes apply refuse that item and report it; --adopt records it as managed after showing what it will own. EXCEPTION (already converged): if the existing item exactly equals the desired item (same symlink target, same canonical JSON value, daypack-marked section with the same body, or an array element already present) it is adopted silently and recorded, with no flag. This is what makes a fresh clone or worktree work with a committed AGENTS.md section. Array elements the user added themselves are adopted but never removed on cleanup.
- Tracked managed content (tracked = true, e.g. the AGENTS.md section): if the current content equals the new desired content, converge silently; if it equals the lockfile hash, update it to the desired content; otherwise it is drift (a hand edit or a pull conflict) and is not overwritten without --force.
- Drift: an item whose current state (link target, hash) differs from the lockfile is reported as drift. Apply does not overwrite or delete drifted items without --force.
- JSON files: edits are byte-level (gjson/sjson style path edits), so unknown keys, key order, indentation and trailing newline are preserved and only owned paths change; the hash uses canonical JSON (sorted keys, no whitespace). Markdown sections use visible markers <!-- daypack:begin NAME --> / <!-- daypack:end NAME -->; content outside is never modified.
- All writes are atomic (temp file in the same directory, then rename); lockfile is rewritten atomically after each batch of operations so a failure never loses track of what was done.
- Git exclusion: managed paths with tracked=false are written to the repository's info/exclude (resolved with 'git rev-parse --git-path info/exclude'). Worktrees share that file, so each project root gets its own block: '# daypack begin <absolute project root>' ... '# daypack end <absolute project root>'. A worktree regenerates only its own block from its lockfile and removes it when empty; status flags blocks whose root no longer exists. .daypack/ is always listed. Outside a git repository, exclusion is skipped silently. The 'tracked' flag is computed by core during planning (git ls-files), never by adapters.
- Path safety (the manifest, lockfile and repo content are untrusted input): every path in the lockfile and in adapter items must be project-relative with no '..' and lie under a target root the adapter declares; apply refuses to run if .daypack/ is a symlink or if .daypack/lock.toml is tracked by git; before any write or remove, no parent directory of the target may be a symlink that leaves the project.
- Package API: Lock.Load/Save, Lock.Items(), Lock.Find(path|key), Status(item) -> {ok, missing, drifted, collision}, ExcludeBlock(items) -> string.

## Acceptance

1. [ ] A fresh apply writes .daypack/lock.toml listing every created item with kind, path and hash
2. [ ] Re-running apply with no manifest change leaves the lockfile and every managed file byte-identical
3. [ ] A pre-existing file or JSON key not in the lockfile is never modified; apply reports a collision and exits non-zero unless --adopt is given
4. [ ] Editing a managed file by hand is reported as drift and is not overwritten or deleted without --force
5. [ ] Removing an entry from the manifest removes exactly its items and its lockfile rows, and leaves hand-written siblings untouched
6. [ ] info/exclude contains a daypack block with exactly the managed untracked paths plus .daypack/, and works from a git worktree
7. [ ] A JSON file with unknown keys and custom indentation round-trips with only daypack-owned keys changed
8. [ ] A killed process between operations leaves a valid lockfile that matches what is on disk (tested with fault injection)

## Open questions
