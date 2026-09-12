# Installation and releases

## Install the CLI

The short form downloads the installer served by pane.run:

```sh
curl -fsSL https://pane.run/install.sh | sh
```

For an inspect-first installation:

```sh
curl -fsSLO https://pane.run/install.sh
less install.sh
sh install.sh
```

The POSIX shell installer supports macOS and Linux on AMD64 and ARM64. It downloads the matching archive and `checksums.txt` from the latest [`rot13maxi/pane.run` GitHub Release](https://github.com/rot13maxi/pane.run/releases), verifies SHA-256 before extraction, and installs `pane` to `~/.local/bin`. It never invokes `sudo`.

Configuration:

```sh
PANE_VERSION=v0.2.0 sh install.sh
PANE_INSTALL_DIR=/usr/local/bin sh install.sh
```

Environment assignments applied to `curl` do not carry through a pipe to `sh`; download the script first when setting installer variables.

## Install the Codex skill

Skill installation is a separate, explicit operation:

```sh
pane skill install codex
```

The CLI contains the matching Pane skill files. It installs them at `$CODEX_HOME/skills/pane` when `CODEX_HOME` is set, otherwise at `~/.codex/skills/pane`. Existing installations are preserved unless replacement is requested:

```sh
pane skill install --force codex
```

Inspection commands do not modify anything:

```sh
pane skill path codex
pane skill print
```

Restart or begin a new Codex session after installing or replacing a skill so it is rediscovered.

## Produce a release

Set a semantic version matching the intended Git tag:

```sh
VERSION=v0.2.0 just release
```

This writes four archives and their checksums under `dist/`:

- `pane-darwin-amd64.tar.gz`
- `pane-darwin-arm64.tar.gz`
- `pane-linux-amd64.tar.gz`
- `pane-linux-arm64.tar.gz`
- `checksums.txt`

Pushing a `v*` tag triggers `.github/workflows/release.yml`. The workflow runs all checks, builds these artifacts with version/commit/date metadata, and creates a GitHub Release using the tag.

## Agent-readable site

The deployed site exposes:

- `/` as HTML for normal browser requests;
- `/` as Markdown when `Accept: text/markdown` is sent;
- `/index.md` as an explicit Markdown homepage;
- `/llms.txt` as a compact agent-oriented index; and
- `/install.sh` as the installer.

The HTML page also advertises its Markdown alternative with a `<link rel="alternate" type="text/markdown">` element. User-agent sniffing is intentionally avoided.
