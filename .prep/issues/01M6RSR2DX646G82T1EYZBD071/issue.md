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
kind   = "symlink"                  # symlink | file | json-entry | md-section
path   = ".claude/skills/svelte"
link   = "/home/u/.config/daypack/skills/svelte"   # symlink: recorded target
# file: sha256 of written bytes; json-entry: key path + sha256 of canonical JSON value;
# md-section: marker name + sha256 of section body
hash   = "..."
file   = ".mcp.json"                # json-entry / md-section only
key    = "mcpServers.context7"      # json-entry only
tracked = false                      # true for items intentionally left visible to git
```
Rules:
- Apply only creates, changes or removes items listed in (or newly added to) the lockfile; nothing else is ever touched.
- Collision: a path or JSON key that exists but is not in the lockfile makes apply refuse that item and report it; --adopt records it as managed after showing what it will own.
- Drift: an item whose current state (link target, hash) differs from the lockfile is reported as drift. Apply does not overwrite or delete drifted items without --force.
- JSON files: unknown keys, key order and (as far as practical) indentation are preserved; only owned keys change. Markdown sections use visible markers <!-- daypack:begin NAME --> / <!-- daypack:end NAME -->; content outside is never modified.
- All writes are atomic (temp file in the same directory, then rename); lockfile is rewritten atomically after each batch of operations so a failure never loses track of what was done.
- Git exclusion: managed paths with tracked=false are written to the repository's info/exclude (resolved with 'git rev-parse --git-path info/exclude' so worktrees work) inside a '# daypack begin' / '# daypack end' block that daypack fully owns; the block is regenerated from the lockfile on each apply and removed when empty. .daypack/ is always listed. Outside a git repository, exclusion is skipped silently.
- Package API: Lock.Load/Save, Lock.Items(), Lock.Find(path|key), Status(item) -> {ok, missing, drifted, collision}, ExcludeBlock(items) -> string.

## Open questions
