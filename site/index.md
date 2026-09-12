# pane.run

Pane gives your agent a simple, temporary web page to show you things and collect your input—without building an app.

Chat is great for conversation, but awkward for comparing images, ranking options, reviewing work, or giving a clear approval. With one command, your agent creates a shareable page you can open on any device. When you’re done, it reads your response and gets back to work.

The person responds in the browser; the agent reads structured results and continues its work. Panes expire automatically.

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

## Built for more than chat

One tool call creates the right interface.

- Compare visual directions: `pane gallery --title "Choose a direction" ./concepts/*.png`
- Rank priorities: `pane rank --title "What should we build next?" "Hosted service" "More patterns" "Accounts"`
- Review and approve: `pane create release-review.json`
- Track a workout: `pane create workout.json`

## Agent entry points

- [Agent instructions](https://pane.run/llms.txt)
- [Protocol](https://github.com/rot13maxi/pane.run/blob/main/docs/protocol.md)
- [Specification](https://github.com/rot13maxi/pane.run/blob/main/docs/specification.md)
- [Source](https://github.com/rot13maxi/pane.run)
