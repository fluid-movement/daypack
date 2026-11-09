---
title: 'eject: leave plain files and remove daypack (post-v1)'
kind: code
parent: 01M6ZYR5JF2KKXZM3S731V5PPF
tags:
  - post-v1
priority: low
---

NFR-4. Command that converts a project's managed items to ordinary files (materialises symlinks, removes markers and the lockfile and git exclusions) so the project keeps working without daypack. Also document that removing daypack alone leaves projects working.

## Open questions
