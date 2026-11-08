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

## Library instructions

```sh
daypack library init                 # create the selected library, or refresh its guide
daypack --library /path/to/library library init
daypack library doctor              # read-only validation
daypack library doctor --fix        # refresh the guide, then validate
```

The binary generates a library-local root `AGENTS.md` describing its layout,
native schemas, commands and reference-only secrets policy. It installs no global
instructions or harness skills. Selection uses `--library`, then `DAYPACK_HOME`,
then `$XDG_CONFIG_HOME/daypack`, then `$HOME/.config/daypack`.

Content between `<!-- daypack:begin library-instructions -->` and
`<!-- daypack:end library-instructions -->` has an explicit content revision and
is refreshed from the binary. Keep user notes outside these markers; their bytes
are preserved. An unmarked file gets one appended region. Identical refreshes
preserve file bytes and modification time; malformed markers cause an error.

Re-running init on an existing directory updates only this guide, including in
an empty directory or a library with invalid records. `doctor --fix` requires an
existing library and performs the same refresh before ordinary validation. Both
leave artifact records, skill support files, `.gitignore`, `secrets.env` and other
files untouched. Neither reads secret values. Only the root guide is excluded
from artifact discovery; skill support files named `AGENTS.md` remain valid.
