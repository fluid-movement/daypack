---
title: Shared .agents/skills adapter (used by Pi and other harnesses)
kind: code
parent: 01M6RSR1Y6XR8HFX1YFTZSWGRR
depends_on:
  - 01M6RSR2MYB5VPVPS8WHTWTDQG
  - 01M6RVX5BR6J3T9KRKD8WXS548
  - 01M6RSR2DX646G82T1EYZBD071
tags:
  - pi
---

Wire library skills into the project's shared .agents/skills directory (finding 01M6TESPYYQJZ5V7Y6BDE7SDJK). Written once for the Agent Skills directory standard: Pi, Codex, opencode, Gemini CLI, Cursor and Copilot read project .agents/skills (per the finding's matrix); Claude Code does not (see its adapter). Pi follows symlinked skills. Pi loads project .agents/skills only after project trust is granted; status/check reports untrusted projects. Same ownership/lockfile and git-exclusion rules as other adapters; this is the shared target other adapters declare.

## Open questions
