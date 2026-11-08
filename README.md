# daypack

daypack reconciles a project's agent configuration with a manifest, using a personal library of reusable artifacts. It ships as a single static binary.

## Install

### go install

```sh
go install github.com/fluid-movement/daypack/cmd/daypack@latest
```

### Release archive

Download the archive for your platform (macOS or Linux, arm64 or amd64) and `checksums.txt` from the GitHub releases page, then verify and extract:

```sh
sha256sum --check --ignore-missing checksums.txt   # on macOS: shasum -a 256 --check --ignore-missing checksums.txt
tar -xzf daypack_<version>_<os>_<arch>.tar.gz
install daypack_<version>_<os>_<arch>/daypack ~/.local/bin/daypack
```

Make sure the destination directory is on your `PATH`.

### From source

```sh
git clone https://github.com/fluid-movement/daypack
cd daypack
just install
```

Check the installation with `daypack version`.

## Secrets

Place this in your POSIX sh, bash or zsh profile so the harnesses you launch inherit your secrets. See [docs/secrets.md](docs/secrets.md) for details, including `--library` selections.

```sh
daypack_secrets_file="${DAYPACK_HOME:-${XDG_CONFIG_HOME:-$HOME/.config}/daypack}/secrets.env"
if [ -f "$daypack_secrets_file" ]; then
    case $- in
        *a*) . "$daypack_secrets_file" ;;
        *) set -a
           . "$daypack_secrets_file"
           set +a ;;
    esac
fi
unset daypack_secrets_file
```
