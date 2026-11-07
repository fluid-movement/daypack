---
title: Auto-apply on fresh clone or worktree
kind: code
parent: 01M6RSR1RXHMJNSEFMHSYVQ4E0
depends_on:
  - 01M6RSR2FKVBKQHS28Z7J39NHB
tags:
  - apply
priority: high
---

Decision: generated config is git-excluded, so every fresh clone or git worktree needs an apply. 'daypack hook install' installs a git post-checkout hook in the repository (via 'git rev-parse --git-path hooks', so it works for worktrees; refuses to overwrite an existing hook, instead printing the line to add) that runs 'daypack apply --auto'.

--auto semantics (NFR-3, never apply untrusted content unattended): applies only library-based entries; if the manifest contains inline MCP servers, or a plan would create a gated (new executable) item, it applies nothing for those and prints one line telling the user to run 'daypack apply'. It never prompts, never uses --force/--adopt, and exits 0 so checkout is not blocked. A repo without daypack.toml is a silent no-op. 'daypack hook uninstall' removes only the hook daypack installed.

## Open questions
