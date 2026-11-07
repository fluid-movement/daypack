---
title: Harness adapter interface and shared targets
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
tags:
  - apply
priority: high
---

FR-15, NFR-6, user requirement: core and adapters are cleanly separated, so adding a harness (e.g. opencode) is a contained change: one new adapter, no edits to core, manifest schema or lockfile.

Design requirements:
- Core (library, manifest, lockfile, reconcile, preview, git exclusion, status) knows nothing about specific harnesses. The manifest lists harnesses by name only.
- An adapter declares the TARGETS it writes: each target has a stable ID (e.g. agents-skills-dir, claude-skills-dir, agents-md, claude-mcp-json, pi-mcp-json), supports one or more artifact types, and says how an artifact is rendered into it (symlink, copy, or an entry/section inside a shared file) including any format translation (e.g. MCP JSON shape per harness).
- Shared targets: harnesses that use the same target (e.g. Pi and others reading project .agents/skills, or any harness reading AGENTS.md) declare the same target ID. Core resolves the set of selected harnesses to the UNION of targets and reconciles each target once. The lockfile records ownership per target, not per harness. Removing a harness from the manifest removes a target only if no remaining harness uses it.
- Adding a harness is: write an adapter that declares its targets (reusing existing target definitions where paths are shared) and any new rendering. A conformance test suite that every adapter must pass (idempotent apply, removal, hand-written content preserved, no secret values written) is part of this issue.
- Adapter capabilities are explicit (which artifact types a harness supports), and the preview/check say clearly when a manifest lists something a selected harness cannot use, instead of failing silently.

Acceptance should include a third, throwaway/fake adapter in the test suite that shares a target with an existing adapter, proving the contract.

Per-artifact targeting (decision "Per-artifact harness targeting in the manifest"):
- Core resolves, for each manifest entry, the set of harnesses it applies to (all selected by default, or its harnesses list), then asks each of those adapters for the targets that entry renders into. Reconciliation works on (target, entry) pairs, so a skill for claude-code only writes .claude/skills, a skill for pi writes the shared .agents/skills, and an entry for both writes both.
- Shared targets stay shared: when an entry is restricted to one harness but its target is read by others (e.g. .agents/skills), it is still written once and status/check reports which other known harnesses will also see it. No attempt to hide it.
- Removing a harness from an entry removes its targets only if no remaining harness of that entry (or other entry) uses them.
- Conformance tests cover restricted and unrestricted entries, including a restricted entry whose target is shared.

Interface sketch (Go; names indicative, shapes binding):
```go
type Adapter interface {
    Name() string                          // "claude-code", "pi"
    Supports(t ArtifactType) bool          // capability matrix, drives 'unsupported' reports
    Desired(e Entry, proj Project) ([]Item, error)   // items this entry needs for this harness
}
type Item struct {
    Target string        // stable target ID shared across adapters, e.g. "agents-skills-dir"
    Entry  string        // "skill:svelte"
    Kind   ItemKind      // Symlink | File | JSONEntry | MarkdownSection
    Path   string        // project-relative
    // kind-specific payload: link target, bytes, JSON key + value, section name + body
}
```
- Items with the same (Target, Entry) from different adapters must be identical; core verifies this and fails loudly if two adapters disagree.
- Adapters never read or write the filesystem or the lockfile; they only describe desired items. Core executes them. This keeps adapters pure and testable with golden files.
- Registration: adapters are listed in one registry file; adding a harness is one adapter package plus one registry line.

## Open questions
