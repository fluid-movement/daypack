---
title: 'Library init: generated agent-facing docs for the config folder (low priority)'
kind: code
parent: 01M6RSQDN4E37HX2EM8F97SADA
tags:
  - library
priority: low
---

So an agent running inside the library folder knows how daypack works (only there, not globally): library init also writes an AGENTS.md describing the layout, schemas and commands of this daypack version, plus a skill under the folder's own .agents/skills and .claude/skills (not the library's skills/ directory, which holds distributable artifacts). Generated from the binary so it never drifts; user notes can be added outside generated markers.

## Open questions
