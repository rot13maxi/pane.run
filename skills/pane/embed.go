// Package paneskill exposes the Pane agent skill bundled with the CLI.
package paneskill

import "embed"

// Files contains the complete Pane skill tree.
//
//go:embed SKILL.md agents/*.yaml references/*.md
var Files embed.FS
