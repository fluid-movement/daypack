---
title: 'Distribution: single self-contained binary, install story'
kind: code
depends_on:
  - 01M6RVX5E50998CXQHV942P828
tags:
  - packaging
---

NFR-2. Single static Go binary. 'go install github.com/fluid-movement/daypack/cmd/daypack@latest' works; one GitHub Actions release workflow on tag builds macOS and Linux (arm64, amd64) archives with plain go build (no GoReleaser, no Homebrew tap in v1). README documents install and the secrets.env shell-profile line. Name availability was checked (finding).

## Open questions
