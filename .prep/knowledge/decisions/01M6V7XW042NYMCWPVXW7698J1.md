---
type: decision
title: No global project registry in v1; rename safety is failure behaviour only
description: 'daypack keeps no registry of projects. Renaming or removing a library artifact can break manifests that reference it; v1 accepts this (private single-user tool, to be re-evaluated by how often it breaks). v1 mitigations are limited to safe failure: apply never deletes working managed items because a manifest name is unresolved, unresolved names and dangling links are reported clearly by apply/status, and library remove/rename tell the user that manifests referencing the old name will need updating. Aliases, tombstones, apply --fix and a registry with apply --all are post-v1.'
date: "2026-11-07"
---


