---
title: Library storage format and location
kind: code
parent: 01M6RSQDN4E37HX2EM8F97SADA
depends_on:
  - 01M6RVX5E50998CXQHV942P828
tags:
  - library
priority: high
---

NFR-5, FR-3. The library is plain files in one directory, versionable in git and synced across machines. This issue defines the on-disk format and the Go package that reads, validates and resolves it. Language/format decisions: Go, TOML for daypack-owned files, native formats for skills/agents/fragments.

Location: $DAYPACK_HOME if set, else $XDG_CONFIG_HOME/daypack, else ~/.config/daypack.

Layout:
```
<library>/
  skills/<name>/SKILL.md (+ any files; an Agent Skills folder stored as-is)
  mcp/<name>.toml
  fragments/<name>.md        (optional frontmatter: description)
  agents/<name>.md           (Claude Code subagent file as-is: frontmatter name+description)
```
Artifact references are written type:name with type one of skill, mcp, fragment, agent. Names: lowercase letters, digits and hyphens, 1-64 chars, unique per type. A skill's or agent's frontmatter name must equal its file/folder name (validated).

MCP definition (mcp/<name>.toml), one schema also used for inline manifest definitions:
```toml
description = "Docs for Svelte"          # optional, shown by list
type = "stdio"                           # stdio | http
command = "npx"                          # stdio only
args = ["-y", "@sveltejs/mcp"]
url = "https://example.com/mcp"          # http only
env = { API_BASE = "https://x.test", API_KEY = "${EXAMPLE_API_KEY}" }   # stdio
headers = { Authorization = "Bearer ${EXAMPLE_TOKEN}" }                 # http
```
Required secret variables are not declared separately: they are derived from the ${VAR} references (without a :-default) found in env, headers, url, command and args, so there is one source of truth for status and verification.

Secret rule (decision: MCP secrets are references only): in env and headers a value is either a ${VAR} / ${VAR:-default} reference or a plain literal; a literal under a key whose name matches (?i)(key|token|secret|password|passwd|auth|credential) is a validation error telling the user to use a reference. Fields not valid for the declared type are errors.

Go package API (used by CLI and adapters): Load(libraryPath) -> Library; Resolve(ref) -> Artifact (type, name, path, description, parsed MCP def); List(type); Validate() -> []Problem with file path and field. Unknown files in the library are ignored with a warning in doctor, not errors.

Out of scope here: library write commands (library CLI issue), import, generated agent docs.

## Acceptance

1. [ ] Library loads from DAYPACK_HOME, then XDG_CONFIG_HOME/daypack, then ~/.config/daypack, and a missing library gives a clear error pointing at 'daypack library init'
2. [ ] Resolve('skill:svelte') returns the skill folder path and its frontmatter description; unknown names return a typed not-found error that carries the closest existing name
3. [ ] An MCP definition with a literal value under a secret-looking env/header key fails validation with the file, key and a hint to use ${VAR}
4. [ ] An MCP definition with command on an http type (or url on a stdio type) fails validation
5. [ ] Names violating the naming rules or duplicated within a type are reported by Validate with file paths
6. [ ] A skill whose frontmatter name differs from its folder name is reported by Validate
7. [ ] Unit tests cover each rule using fixture libraries; no test touches the real home directory

Additional rules after review:
- A skill's SKILL.md must have a non-empty description in its frontmatter (Pi does not load a skill without one); Validate errors otherwise.
- Validate warns (not errors) when an MCP definition's args or url contain values that look like secrets (known prefixes such as ghp_, sk-, xox; query parameters named key/token/secret/password; long high-entropy strings), telling the user to move them to an env variable and reference it.
- ${VAR} references are allowed anywhere in an MCP definition at the library level; whether a harness can expand them in a given field is an adapter concern (the Pi adapter reports an 'unsupported' line for fields Pi is not known to expand).

## Open questions
