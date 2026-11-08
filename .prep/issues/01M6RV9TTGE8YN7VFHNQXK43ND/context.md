## State of main

Reuse, do not duplicate:
- `internal/library`: `Load(path) (*Library, error)`, `(*Library).List(ArtifactType) []Artifact` (Artifact has Type, Name, Path, Description, MCP), `Resolve("type:name")` returning `*NotFoundError{Type, Name, Closest}` (Levenshtein closest in same type), `Validate() []Problem`, `Doctor`. Types: `Skill`, `MCP`, `Fragment`, `Agent`. Description comes from SKILL.md / agent / fragment frontmatter already parsed by the loader.
- `internal/manifest`: `FindProject(start)` (errors when no daypack.toml, stops at .git), `LoadManifest(path)`, `(*Manifest).Entries()` (effective harnesses per entry, `Inline != nil` for inline MCP), `Validate(lib)` / `ValidateForApply(lib)` returning `[]Problem` (already handles unknown harness, harness not selected, duplicates, inline/library MCP collision, unresolved refs).
- `internal/cli`: `SetHandler`, `Options{JSON, Quiet, ProjectDir, LibraryDir}`, `Result{Items, Problems, Message, Code}`, `Failure{Code, Cause, Hint}`, exit consts `Success=0, Error=1, Usage=2, Attention=3`; JSON envelope `{ok, command, items, problems, ...}` is produced by `render` in `output.go`. Root registers commands in `NewRootCommand` (`root.AddCommand(libraryCommand(...), applyCommand(...))`). No `list` or `check` command exists yet.
- `internal/testutil`: `NewFixture(t, false)` (library with skills/testing, mcp/docs, fragments/conventions, agents/reviewer; project daypack.toml selecting all with harnesses claude-code and pi), `RunCLI(t, f, args...)` -> `CLIResult{Code, Stdout, Stderr}`, `AssertJSONGolden`.

Gap to close: the closest-name suggestion exists only inside `NotFoundError.Error()` text ("... not found (closest: x)"); `manifest.validate` copies `err.Error()` into Problem.Message with empty Hint. Unknown harness names get no suggestion.

## Plan

1. New file `internal/library/catalog.go` (or `internal/manifest/usage.go`; prefer manifest because it joins both):
   ```go
   type UsageEntry struct {
       Type library.ArtifactType `json:"type"`
       Name, Description, Source string // Source: "library" | "project" (inline MCP)
       Used bool                       // referenced by project
       Harnesses []string              // effective harnesses when Used; nil otherwise
       Invalid bool                    // library entry has validation errors
   }
   func Catalog(lib *library.Library, m *Manifest) []UsageEntry
   ```
   m may be nil (outside a project): no marks, no inline entries. Order: type order skill, mcp, fragment, agent; names sorted within type; inline MCP entries (Source "project", description empty or from definition) appended to the mcp group. Built from `lib.List` and `m.Entries()`; a manifest reference not in the library is not listed (validation reports it).
2. Suggestions: in `internal/manifest/validate.go`, when `Resolve` returns `*library.NotFoundError`, set `Problem.Hint = "did you mean <type>:<closest>?"` (omit hint if Closest is empty) while keeping the message unchanged. For unknown harness, hint `did you mean "<closest>"?` using the adapter names (`claude-code`, `pi`; add a small unexported `closest(name, candidates)` helper; the library `distance` is unexported, so either export a `library.Closest(name string, candidates []string) string` wrapper or copy nothing and call that). Add the check at every harness site in `checkHarnesses`. Inline/library collision error already exists; keep and cover with a test.
3. CLI: new `internal/cli/list.go` with `listCommand() *cobra.Command`, `Use: "list [type]"`, `Args: cobra.MaximumNArgs(1)`, registered at top level in `NewRootCommand`. Type arg validated against `skill|mcp|fragment|agent` (invalid -> `&Failure{Code: Usage, Hint: "types: skill, mcp, fragment, agent"}`; also accept plural forms skills/fragments/agents as convenience). Handler: `library.Load(opts.LibraryDir)` (missing library -> Error with the existing "run daypack library init" message); project discovery via `manifest.FindProject(opts.ProjectDir)`; if not found treat as no project (no marks, still exit 0); if found but manifest fails to load, still list library, add the failure to `Problems`, exit Attention (3)... but unknown names must not hide the listing.
4. Output. Text: grouped by type with a heading, one line per entry `  [x] name  description  (claude-code, pi)` where used entries are marked and show effective harnesses, inline MCP shown as `(project-local)`. Honour `--quiet` (names only, `type:name` per line). JSON: `Items` is the `[]UsageEntry` list (flat, ordered as above; same data as text) plus `Problems` for manifest problems; envelope via existing `render` (text rendering needs a branch in `render` for list items, or a `Message` built by the handler; prefer building `Result.Message` text in the handler so output.go barely changes, with `Items` still set for JSON; check that `Message` is suppressed by `--quiet` and print quiet output differently).
5. Validation hook for check: expose `manifest.Validate` unchanged for any `check`/`status` command (not in this issue's scope unless it exists by implementation time; if `check` exists, ensure it surfaces the Hint). Do not add a duplicate validator. `list` itself reports manifest problems but is informational: exit 0 when the project is valid or absent, 3 when the manifest has validation errors.
6. Not in scope: JSON Schema, shell completions, editor tooling.

Exit codes: 0 ok, 1 library missing/unreadable or internal error, 2 bad type argument/flags, 3 manifest has validation errors (list still printed).

## Tests (use internal/testutil)

- `internal/manifest/usage_test.go`: Catalog with fixture-like library (build via `library.Load` on `NewFixture(t,false).Library`) and loaded manifest: all four types present, used=true with harnesses [claude-code pi]; per-entry harness restriction (`harnesses = ["pi"]`) reflected; nil manifest gives no marks; inline MCP appears as Source "project"; deterministic order.
- `internal/manifest/validate_test.go` additions: `skills = ["testin"]` yields a problem with Hint containing `skill:testing`; `harnesses = ["claud-code"]` hint suggests `claude-code`; entry harness `pi` when manifest selects only claude-code errors; inline MCP named `docs` collides with library `docs`.
- `internal/cli/list_test.go` with `testutil.RunCLI(t, f, "list")`: exit 0, stdout has group headings and "Offline test skill"; `list skill` shows only skills; `list bogus` exit 2; `list --json` stdout parses with `ok:true`, `command:"daypack list"`, items equal text data (use `AssertJSONGolden` with `NormalizeFixturePaths`); outside project (`f.Project` without daypack.toml, run with a fresh temp project dir, e.g. delete daypack.toml) exit 0, no entry has used=true; missing library dir exit 1; manifest with unknown name exit 3 with hint in `problems` while items still listed; `--quiet` prints names only.
- Existing tests must keep passing (`go test ./...`, `go vet ./...`).
