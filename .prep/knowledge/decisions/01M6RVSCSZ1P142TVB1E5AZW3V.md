---
type: decision
title: Hooks are out of v1
description: 'Hooks (FR-22) move out of v1: v1 manages skills, MCP servers, instruction fragments and subagents only, and daypack writes no project settings files for hooks. Reason: hooks run arbitrary commands and require editing settings files; the base is easier to get right with fewer file types and the user does not currently need them. Hooks return as a later issue; the manifest format stays extensible.'
date: "2026-11-06"
---


