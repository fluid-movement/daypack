---
type: decision
title: Codex MCP uses native environment-name fields with explicit unsupported cases
description: Render supported references without reading credentials or introducing wrappers.
date: "2026-11-09"
issue: 01M6ZW8G0B62Q0V52SQ32VA6GZ
---

Source: https://learn.chatgpt.com/docs/extend/mcp?surface=cli . The documented native fields distinguish static env/http_headers values from env_vars, bearer_token_env_var and env_http_headers names; general string interpolation is not established.

Choose these mappings from the current library.MCPDefinition, without widening its schema:
- stdio env K="${K}" -> env_vars=["K"]; literal non-secret env values -> env. env K="${OTHER}" cannot rename through env_vars: return UnsupportedError for env.K.
- HTTP Authorization="Bearer ${TOKEN}" -> bearer_token_env_var="TOKEN" (match Authorization case-insensitively); an entire header value "${VAR}" -> env_http_headers[header]="VAR"; non-secret literal headers -> http_headers.
- References in command, args or url, default expressions, composed env/header references other than that Bearer form, and bare $VAR expansions are unsupported. Report sorted field names, never values. Reject the whole Codex item rather than partially installing a broken server. Keep other adapters' semantics unchanged. Do not emit library description/type into the Codex server table.

Reason: native environment indirection meets the no-plaintext-secret requirement without executing helpers, resolving credentials at apply time or embedding a launcher shim. Alternatives rejected: copying ${VAR} literally and pretending Codex expands it; secret expansion into TOML; shell wrappers to support renames/defaults. Harness launch must inherit required environment variables through daypack's existing secret handoff workflow; this adapter does not source .env itself. Keep existing validation and secret-looking literal rejection, and add sentinel tests across output/config/lock/error paths.
