---
type: decision
title: Library secrets use .env with a generated empty example
description: Use .env as the canonical private library file and generate .env.example from MCP references; retain legacy secrets.env read fallback. The user chose the established .env convention and wants an example showing which tokens to add.
date: "2026-11-08"
issue: 01M6XDC080RGFSJ6S4Q5YZR1M5
---

The private store is `<library>/.env`; `<library>/secrets.env` remains a read fallback only when `.env` is absent, with a doctor note recommending a manual rename. Keep the existing conservative shell-sourceable format, owner-only checks, and explicit shell loading; runtime config continues to use inherited environment references.

Generate a commit-safe `.env.example` containing deduplicated variable names referenced by library MCP definitions, including references with defaults, with empty values. Init and doctor --fix manage generated content without editing private files or library artifact records. Ignore `.env` and legacy `secrets.env`, but keep `.env.example` trackable.

Reason supplied by the user: `.env` is the established pattern and `.env.example` shows which tokens must be added. Retaining only `secrets.env` would miss that convention; silently renaming or rewriting a user's private store would violate the requested preservation boundary.
