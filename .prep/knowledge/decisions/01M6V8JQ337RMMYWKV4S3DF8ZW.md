---
type: decision
title: 'Trust model: library entries trusted, inline MCP servers gated'
description: 'Library entries come from the user''s own library and are trusted: they apply without confirmation, including in auto-apply on new worktrees. Inline MCP servers live in the committed manifest, may come from an untrusted clone, and are arbitrary commands: they always require confirmation on the first apply on a machine/worktree (no lockfile) and whenever their command or url changes; auto-apply never applies them and prints a notice. Reason: keeps NFR-3 without gating every library server in every fresh worktree (which would break auto-apply).'
date: "2026-11-07"
---


