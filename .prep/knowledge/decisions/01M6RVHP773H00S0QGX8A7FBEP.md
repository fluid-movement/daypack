---
type: decision
title: Manifest may define MCP servers inline; everything else library-only
description: 'Project tomls reference library artifacts by name, and may additionally define MCP servers inline using the same schema as library MCP definitions. Inline hooks, skills, agents and instruction fragments are out of scope for v1. A name that exists both inline and in the library is an error (no silent shadowing). Literal secret values are rejected in inline definitions; secrets are references only. Reason: MCP servers are the thing that varies per project (DB names, URLs) and the user currently only has global MCP config; the rest can stay library-only.'
date: "2026-11-06"
---


