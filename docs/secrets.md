# Environment secrets

Daypack v1 configuration contains references, such as `${DOCS_TOKEN}`, rather
than credential values. A harness reads its inherited environment at runtime;
configuration works without the daypack binary. Daypack never auto-loads dotenv,
expands secrets during validation, or alters its parent shell's environment.
There is no daypack dotenv precedence: the harness's inherited environment is
authoritative.

The selected library directory is `--library` when explicitly supplied, otherwise
`DAYPACK_HOME`, then `$XDG_CONFIG_HOME/daypack`, then `$HOME/.config/daypack`.
Its optional private file is exactly `<selected-library>/secrets.env`. Keep this
outside the current project/worktree. `daypack library init` adds `/secrets.env`
to the library `.gitignore` but deliberately creates no secrets file. Never
commit that file. Both tracked files and files without an effective ignore rule
are rejected by the storage helper and reported by `daypack library doctor`.

Use owner-only permissions (`chmod 600 '/your selected library/secrets.env'`).
The writer refuses existing files with any group/other permission bits; it never
silently changes their permissions. Doctor warns about these permissions, with
exit status 0, and reports Git hygiene and non-regular-file errors with exit
status 1. It checks metadata without reading secret contents or changing files.
The writer rejects symlinks, including directory components, and special files.

## Load into your shell

Place this in your POSIX sh, bash or zsh profile, using the same explicit path
if you select the library with `--library`. Only source a trusted local file;
never enable shell tracing around credentials. This example preserves an already
enabled `allexport` (`set -a`) option:

```sh
daypack_secrets_file="${DAYPACK_HOME:-${XDG_CONFIG_HOME:-$HOME/.config}/daypack}/secrets.env"
if [ -f "$daypack_secrets_file" ]; then
    case $- in
        *a*) . "$daypack_secrets_file" ;;
        *) set -a
           . "$daypack_secrets_file"
           set +a ;;
    esac
fi
unset daypack_secrets_file
```

For a `--library '/some explicit path'` selection, replace the first assignment
with `daypack_secrets_file='/some explicit path/secrets.env'`.
`set -a` exports the assignments so child processes inherit them. Sourcing
**overwrites same-named shell variables**; you control when to source it. Reload
that profile (or open a new shell), then launch/restart the harness from that
shell. Already-running harnesses need to inherit/reload the updated environment.
GUI-launched harnesses need separate configuration of their launcher's environment.

## File format and collisions

The supported shell-sourceable subset is intentionally conservative:

- Blank lines and whole-line `#` comments (optionally indented).
- `NAME=value` assignments, with names matching `[A-Za-z_][A-Za-z0-9_]*`.
- Empty values, unquoted ASCII letters/digits and `_ . / : @ % + , = -`,
  and single-quoted literal segments. The standard `'\''` splice embeds an
  apostrophe. Segments can be concatenated.

```sh
DOCS_TOKEN='a literal value with spaces, $dollars, `backticks`, #hash, = and \backslash'
APOSTROPHE='it'\''s literal'
EMPTY=''
BASE_URL=https://example.test
```

No `export` prefix, double quotes, expansions, command substitutions, multiline
values, NUL, CR, inline comments or duplicate assignments are supported. The
writer always uses single-quoted assignments and escapes apostrophes; sourcing
its output does not evaluate value contents.

`library.WriteSecrets(root, projectRoot, values)` is the importer-facing storage
API. It validates the complete batch before writing; a differing value for an
existing name aborts the entire batch and reports only that name. Identical
values preserve bytes and modification time. Existing comments, bytes and order
survive; missing names append in sorted order. Empty batches create nothing.
New/replacement files are mode 0600 regardless of umask and are staged privately
in the library filesystem, then atomically published. Failed validation or
publication leaves the previous file intact. Concurrent import processes are
not coordinated; run one writer at a time. A caller must store its complete
per-server batch successfully before publishing the sanitized MCP definition.
Harness scanning, name allocation and token replacement belong to the importer.

## Reference-only MCP definitions

A stdio definition in the library can pass a reference in its environment:

```toml
type = 'stdio'
command = 'docs-server'
[env]
DOCS_TOKEN = '${DOCS_TOKEN}'
```

An HTTP definition (also supported under `[inline.mcp.docs]` in a manifest):

```toml
type = 'http'
url = 'https://${DOCS_HOST:-docs.example.test}/mcp'
[headers]
Authorization = 'Bearer ${DOCS_TOKEN}'
```

References work in command, args, URL, env and headers, including embedded and
multiple references. `${NAME:-default}` has a non-secret fallback; defaults must
never contain credentials. Unset required variables and explicit empty values
are distinct. The internal `secrets.EnvSource.ReadCommand` template fails for
unset variables and succeeds for set-but-empty values; harnesses handle their
own reference/default semantics. Generation never reads values or executes that
command. `Source` maps a validated reference name to an argv read command, leaving
room for later providers and runtime adapters. MCP environment adapters emit
`${NAME}` directly and need no wrapper. Keychain and external secret managers
are later work; v1 adds no provider syntax to manifests or library files.

Secret-looking env/header keys reject literal values; suspicious args/URLs only
warn. This is a heuristic, not a guarantee that arbitrary literals are safe.
Keep credentials exclusively in the external private file or environment, never
in project files, locks, command arguments, defaults or diagnostic output.

## Generated library instructions

`daypack library init` creates a root `AGENTS.md` from text compiled into the
binary, with a content revision and the managed markers
`<!-- daypack:begin library-instructions -->` and
`<!-- daypack:end library-instructions -->`. Notes outside that region retain
their bytes. An unmarked handwritten file gets one appended region; malformed
markers are rejected. The guide is local to the selected library, universal
across harnesses, and is excluded only at the library root from artifact discovery
and import sources. No global instructions or harness skills are installed.

Re-running `daypack library init` on an existing directory, or running
`daypack library doctor --fix`, creates missing instructions or refreshes stale
managed content. Identical runs preserve bytes and modification time. Existing
empty directories receive only the guide; invalid artifacts do not prevent a
refresh. `--fix` refuses a missing library and then runs normal validation,
retaining unrelated diagnostics. Plain doctor remains read-only.

These refreshes leave records, support files, `.gitignore`, `secrets.env` and
unrelated files untouched. They neither repair secrets nor read secret values.
Fresh init alone creates the artifact folders and the `/secrets.env` ignore rule.
