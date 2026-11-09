---
title: 'Claude Code: hooks merged into project settings (post-v1)'
kind: code
parent: 01M6ZYR5JF2KKXZM3S731V5PPF
tags:
  - post-v1
priority: low
---

FR-22. Out of v1 (see decision "Hooks are out of v1"). Do not implement as part of the base.

When picked up later: add hooks from the manifest to the project's Claude Code settings (settings.local.json, git-excluded) without disturbing other settings; owned keys tracked in the lockfile; commands that hooks run must be visible in the preview (NFR-3). Requires a hooks artifact type in the library and manifest.

## Open questions
