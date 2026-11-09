---
type: decision
title: Add table-scoped TOML ownership for Codex config; preserve explicit harness selections
description: Extend core ownership instead of owning or re-encoding the entire Codex config file.
date: "2026-11-09"
issue: 01M6ZW8G0B62Q0V52SQ32VA6GZ
---

Follow [Ownership](/decisions/01M6RVX4S7600YVGAAE9282MYS.md). Add a TOML-table item kind through contract, lock, reconciliation, execution, status and plan validation. A Codex MCP item owns precisely mcp_servers.<name> and its descendants in .codex/config.toml. Unowned server tables and unrelated config, comments and ordering must survive byte-for-byte. Hash the owned table semantically; preserve the surrounding document through syntax-aware edits. Reject malformed/ambiguous layouts without writes, including invalid TOML or ancestors of incompatible types. Support ordinary explicit tables, nested env/header subtables and inline server tables, or safely report an unsupported layout before mutation; never fall back to whole-file replacement. Quoted keys/names must identify real TOML segments, not accidental dotted paths.

Whole-file File ownership is rejected because it would claim user configuration and make unrelated edits drift. JSONEntry is rejected because Codex reads TOML. Marker-only string substitution without TOML validation is rejected because TOML table scope and quoted keys make it unsafe. Leave empty pre-existing containers/files intact on removal; remove only owned tables. Apply existing adopt/force/drift/eject, symlink containment, atomic writes and tracked/untracked exclude rules.

Add codex to the production registry and known harnesses. With no --harness selection, init follows its existing all-known-harnesses policy (now all three); explicit selections and copied manifests retain their meaning. Extend the already scanned ~/.agents/skills import root with Codex provenance and deduplicate its existing Pi provenance, rather than introduce legacy ~/.codex/skills discovery or Codex MCP/agent import. Keep import source retention/library-link safeguards. Runtime trust is an advisory note only: never modify the user's trust store or self-trust a project.
