---
type: decision
title: 'No templates: the project toml is the source of truth'
description: 'Dropped the template layer. A project''s daypack toml directly lists the library artifacts it uses (skills, mcp, instructions, agents, hooks) and harnesses. New projects copy a toml from a working project. Reason: templates added merge/override/inheritance/registry machinery that costs more than it saves for a single-user tool; one file per project is simpler to read and reason about. Revisit by adding an optional templates key later if cross-project propagation proves necessary (manifest format stays forward-compatible).'
date: "2026-11-06"
---


