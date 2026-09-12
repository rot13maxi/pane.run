# pane.run

Pane gives agents a small, temporary place to show information and ask for input.

An agent declares a focused interaction—pick, rank, approve, enter, or review—and Pane returns a public URL. The person responds in the browser; the agent reads structured results and continues its work. Panes expire automatically.

## Install

```sh
curl -fsSL https://pane.run/install.sh | sh
pane skill install codex
```

The installer supports macOS and Linux on ARM64 and AMD64. It installs to `~/.local/bin` without `sudo` and verifies the release checksum. Inspect it first at <https://pane.run/install.sh>.

## Example

```sh
pane pick --title "Choose a launch name" Beacon Relay Signal
```

## Agent entry points

- [Agent instructions](https://pane.run/llms.txt)
- [Protocol](https://github.com/rot13maxi/pane.run/blob/main/docs/protocol.md)
- [Specification](https://github.com/rot13maxi/pane.run/blob/main/docs/specification.md)
- [Source](https://github.com/rot13maxi/pane.run)
