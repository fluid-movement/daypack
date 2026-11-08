---
title: Library root may be a symlink (dotfiles/stow); only guard paths inside it
kind: code
parent: 01M6RSQDN4E37HX2EM8F97SADA
tags:
  - library
priority: high
---

library init / doctor --fix (generated AGENTS.md writer, internal/library instructions code) fail with 'symlink path is not allowed' when the library directory itself, or any ancestor of it (e.g. macOS /var -> /private/var), is a symlink. The user will keep ~/.config/daypack in ~/dotfiles via a symlink, so this must work. Resolve the library root with filepath.EvalSymlinks once and apply the symlink guard only to paths inside the library (AGENTS.md and its staging file must still not be symlinks or escape the root). Add regression tests: symlinked root, symlinked ancestor, symlinked AGENTS.md still refused.

## Open questions
