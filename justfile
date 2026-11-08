# List the available recipes.
default:
    @just --list

version_pkg := "github.com/fluid-movement/daypack/internal/cli"

# Build the daypack binary in bin/, stamped with the git version.
build:
    CGO_ENABLED=0 go build -trimpath -ldflags "-X {{version_pkg}}.version=$(git describe --tags --always --dirty)" -o bin/daypack ./cmd/daypack

# Cross-compile and package macOS and Linux (arm64, amd64) archives into dist/, as the release workflow does.
dist:
    #!/usr/bin/env bash
    set -euo pipefail
    tag="${TAG:-$(git describe --tags --always --dirty)}"
    rm -rf dist stage
    mkdir -p dist
    for goos in darwin linux; do
      for goarch in arm64 amd64; do
        name="daypack_${tag}_${goos}_${goarch}"
        mkdir -p "stage/${name}"
        CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "-s -w -X {{version_pkg}}.version=${tag}" -o "stage/${name}/daypack" ./cmd/daypack
        cp README.md "stage/${name}/"
        if [ -f LICENSE ]; then cp LICENSE "stage/${name}/"; fi
        tar -C stage -czf "dist/${name}.tar.gz" "${name}"
      done
    done
    rm -rf stage
    (cd dist && { sha256sum *.tar.gz 2>/dev/null || shasum -a 256 *.tar.gz; } > checksums.txt)
    ls dist

# Build and install daypack, warning if the destination is outside PATH.
install: build
    @dir="${DAYPACK_INSTALL_DIR:-$HOME/.local/bin}"; mkdir -p "$dir" && cp bin/daypack "$dir/daypack" && printf 'Installed daypack to %s/daypack\n' "$dir" && { case ":$PATH:" in *:"$dir":*) ;; *) printf 'Warning: %s is not on PATH\n' "$dir" >&2 ;; esac; }

# Remove daypack from the installation directory.
uninstall:
    rm -f "${DAYPACK_INSTALL_DIR:-$HOME/.local/bin}/daypack"

# Run all Go tests.
test:
    go test ./...

# Run all Go tests with the race detector.
test-race:
    go test -race ./...

# Run all Go tests with verbose output.
test-v:
    go test -v ./...

# Run all Go tests and print coverage.
cover:
    go test -cover ./...

# Check Go code with go vet.
vet:
    go vet ./...

# Check Go formatting and fail if files need gofmt.
fmt:
    @files="$(gofmt -l .)"; status=$?; if [ "$status" -ne 0 ]; then exit "$status"; fi; if [ -n "$files" ]; then printf '%s\n' "$files"; exit 1; fi

# Run golangci-lint (must be installed separately).
lint:
    @command -v golangci-lint >/dev/null 2>&1 || { printf 'golangci-lint is not installed; install it to run just lint\n' >&2; exit 1; }
    golangci-lint run

# Run local formatting, vet, and test checks.
check: fmt vet test

# Run formatting, vet, tests, and lint for CI.
ci: check lint

# Remove generated binaries.
clean:
    rm -rf bin/ dist/ stage/

# Run daypack with the supplied arguments.
run *args:
    go run ./cmd/daypack {{args}}

# Validate prep issue and knowledge records.
prep-check:
    prep check
