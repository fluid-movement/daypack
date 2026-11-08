## Current state (verified in repo)
- Go module `github.com/fluid-movement/daypack`, `go 1.27.3`; entry point `cmd/daypack/main.go` calls `cli.Run`; cobra root is built in `internal/cli/root.go` (`root.AddCommand(libraryCommand, applyCommand)`, ~line 89).
- No cgo anywhere (no `import "C"`); deps are pure Go (cobra, toml, gjson/sjson, yaml.v3, x/sys, x/term). Locally `CGO_ENABLED=1` by default, so the static build must set `CGO_ENABLED=0` explicitly.
- No `version` command or version variable exists. No `README.md` exists at repo root (only `docs/secrets.md`, `docs/testing.md`). The secrets.env shell-profile snippet already lives in `docs/secrets.md` ("Load into your shell"); the README must show it or link/copy it, keeping the two in sync.
- `.github/workflows/ci.yml` runs on push (checkout v5.0.0, setup-go v6.0.0 with `go-version-file: go.mod`, vet, test, golangci-lint v2.15.0). Pin actions by exact version like this.
- `justfile` has `build` (`go build -o bin/daypack ./cmd/daypack`), `install`, `check`, `ci`. Related knowledge: decision [Go, single static binary](/decisions/01M6RVX4FQSPF4NYC0ED2TJEDJ.md); finding [Name availability](/findings/01M6V89J5AKE5YRWAHXFZPQY8Y.md) (npm irrelevant; no wrapper).

## Implementation plan (files)
1. Version stamping: add `var version = "dev"` (e.g. in `internal/cli/version.go`, or a small `internal/version` package) and a `daypack version` cobra command (plus `--version` on root is optional) printing `daypack <version>`. Honor the JSON/output contract from `internal/cli/output.go` (check how other commands do `--json`). For `go install ...@latest` builds where ldflags are absent, fall back to `runtime/debug.ReadBuildInfo().Main.Version` when `version == "dev"`. Add a unit test in `internal/cli` (default prints `dev` or build-info value; overriding the variable changes output).
2. `.github/workflows/release.yml`: trigger `on: push: tags: ['v*']`; `permissions: contents: write`. Matrix over `goos: [darwin, linux]` x `goarch: [arm64, amd64]` (no Windows, no GoReleaser, no Homebrew). Steps: checkout, setup-go (`go-version-file: go.mod`), `CGO_ENABLED=0 GOOS GOARCH go build -trimpath -ldflags "-s -w -X github.com/fluid-movement/daypack/internal/cli.version=${GITHUB_REF_NAME}" -o daypack ./cmd/daypack`, package `daypack_${tag}_${goos}_${goarch}.tar.gz` (binary + LICENSE/README if present), upload artifact; a final job downloads all, writes `checksums.txt` (sha256sum), and creates the GitHub release (`gh release create "$tag" --generate-notes dist/*` with `GH_TOKEN`). Keep the ldflags `-X` path in sync with the variable's real package path. Optionally run `go test ./...` first (needs: job).
3. `justfile`: make `build` stamp version (`-ldflags "-X ...version=$(git describe --tags --always --dirty)"`), and add a recipe such as `release-build` / `dist` that cross-compiles the 4 targets into `dist/` for local verification, using the same flags as the workflow.
4. `README.md` (new, at repo root): what daypack is (one line), Install section: (a) `go install github.com/fluid-movement/daypack/cmd/daypack@latest`; (b) download a release archive for macOS/Linux arm64/amd64, verify against `checksums.txt`, extract and put on PATH; (c) from source via `just install`. Then the secrets.env shell-profile snippet (same as `docs/secrets.md`) and a link to `docs/secrets.md`. State `daypack version` usage.

## Verify offline (required, no publishing)
- `CGO_ENABLED=0 go build ./cmd/daypack` succeeds for each of darwin/linux x arm64/amd64 (`GOOS=... GOARCH=... go build`); `go vet`, `just check` pass.
- On the host binary: `go version -m bin/daypack` shows `CGO_ENABLED=0`; `file`/`otool -L`/`ldd` show no dynamic libs beyond the system baseline (Linux build: "statically linked").
- Build with `-ldflags "-X ...version=v0.0.0-test"` and run `daypack version` -> contains `v0.0.0-test`; without ldflags -> `dev` (or build-info version).
- Workflow YAML is syntactically valid (`actionlint` if installed, else a YAML parse) and the tarball packaging commands can be dry-run locally via the justfile recipe, including sha256 checksum generation.
- README install commands are textually consistent with the module path in `go.mod`; the shell snippet matches `docs/secrets.md`.

## Needs a live release (do NOT do; note as out-of-scope follow-up)
- Pushing a `v*` tag and confirming the workflow publishes the 4 archives + checksums.
- `go install github.com/fluid-movement/daypack/cmd/daypack@latest` resolving from the public module proxy (requires a pushed tag and a public repo).
Do not push tags, create releases, or publish anything as part of this issue.
