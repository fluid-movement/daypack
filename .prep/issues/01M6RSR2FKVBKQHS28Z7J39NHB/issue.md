---
title: 'Apply engine: convergent, repeatable'
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
depends_on:
  - 01M6RSR2HBPNRJ10XJVE01GG3P
  - 01M6RSR2DX646G82T1EYZBD071
  - 01M6RSR28PM3XB3VXM0HDPJ61N
  - 01M6RSR21N92V3W1HVT3TBWGCD
  - 01M6RVX5E50998CXQHV942P828
tags:
  - apply
priority: high
---

FR-9, FR-11, FR-13. The apply engine reconciles a project with its manifest. It is harness-neutral: it works on the adapter interface and the lockfile only.

Algorithm (deterministic):
1. Find the project (nearest daypack.toml), load manifest and library, validate; stop on validation errors.
2. Resolve entries: each (type:name, harness set) pair; unresolved entries are collected, not fatal (see Safe failure).
3. For every harness in each entry's set, ask the adapter for the desired items (target, rendering). Merge by (target ID, entry) so a target shared by several harnesses is produced once.
4. Load the lockfile and inspect the current state of every locked item.
5. Compute a plan of operations: create, update, remove, adopt, and non-writing outcomes (collision, drift, unresolved, unsupported).
6. Render the plan with the plan renderer (plan/preview issue) and apply the confirmation gate. With --dry-run stop here (exit 0 if no pending changes, 3 if there are).
7. Execute operations in a fixed order: removals first, then creates/updates ordered by target ID then entry name; update the lockfile after each batch (ownership issue), then regenerate the exclude block.
8. Print the result summary: what changed, what was skipped and why.

Flags: --dry-run, --yes (skip the confirmation described in the preview issue), --force (overwrite/delete drifted managed items), --adopt (take over colliding items). Exit codes follow the CLI contract issue.

Safe failure (decision: no global project registry in v1): an unresolvable manifest entry is reported and skipped and never causes deletion of already-applied managed items; apply exits non-zero when anything was skipped.

Convergence: applying twice changes nothing the second time; changing the manifest and re-applying adds, updates and removes accordingly; removing a harness from the manifest removes items only it used.

## Acceptance

1. [ ] Apply on an empty project with a valid manifest creates all items and a lockfile; a second apply makes zero filesystem changes (verified by comparing file trees and mtimes)
2. [ ] Removing a skill, an MCP server and a harness from the manifest and re-applying removes exactly their items, and shared targets still used by another harness are kept
3. [ ] An unresolvable manifest entry is skipped and reported, previously applied items stay, and the exit code is non-zero
4. [ ] Collisions and drift are never overwritten without --adopt / --force, and the report names the path and the reason
5. [ ] --dry-run writes nothing (no files, no lockfile, no exclude block) and lists the plan
6. [ ] Operation order is deterministic: the same inputs give identical plan output across runs
7. [ ] A shared target (.agents/skills used by pi and a fake second harness) is reconciled once

Exit codes (CLI contract): 0 applied or nothing to do; 1 invalid input (manifest/library validation failure) or I/O error; 3 something was skipped or needs attention (collision, drift, unresolved entry, gated entry not confirmed).

## Open questions
