---
type: decision
title: Library secrets file is .env with .env.example
description: Rename secrets.env to .env and generate .env.example.
date: "2026-11-08"
issue: 01M6XDC080RGFSJ6S4Q5YZR1M5
---

Chosen (by the user): the library's private secrets file becomes <library>/.env, with a generated .env.example (empty values for env vars referenced by library MCP definitions). Reason: .env/.env.example is the established pattern and shows which tokens must be added; the secrets.env name had no recorded reason. Legacy secrets.env stays readable as a fallback.
