# pane.run

Pane gives your agent a simple, temporary web page to show you things and collect your input—without building an app.

Pane is for moments when your agent needs something from you and chat is the wrong modality: comparing things you have to see, putting options in order, reviewing a document, or filling in labeled fields and checks. With one command, your agent creates a shareable page you can open on any device. When you’re done, it reads your response and gets back to work.

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

## Examples

- Compare — See options side by side and choose one: `pane gallery --title "Choose a direction" ./concepts/*.png`
- Rank — Put ideas, tasks, or tradeoffs in order: `pane rank --title "What should we build next?" "Hosted service" "More patterns" "Accounts"`
- Approve — Read the details and give a clear go or no-go: `pane create release-review.json`
- Capture structured input — Enter numbers, checks, and short text in labeled fields instead of formatting a reply in chat: `pane create workout.json`

## Agent entry points

- [Agent instructions](https://pane.run/llms.txt)
- [Protocol](https://github.com/rot13maxi/pane.run/blob/main/docs/protocol.md)
- [Specification](https://github.com/rot13maxi/pane.run/blob/main/docs/specification.md)
- [Source](https://github.com/rot13maxi/pane.run)
