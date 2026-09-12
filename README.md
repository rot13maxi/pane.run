# Agent Surface

A lightweight, disposable web UI substrate for richer agent/human interaction.

This repository contains the V1 Go CLI and hosting service. See
[`docs/product.md`](docs/product.md) for scope and [`docs/protocol.md`](docs/protocol.md)
for the HTTP contract.

## Create a surface

Start with a recipe. Recipes cover common agent/human interactions without making
the agent write or retain a specification file:

```sh
surface gallery --title "Pick a direction" ./mockups/*.png
surface pick --title "Choose a launch name" "Beacon" "Relay" "Signal"
surface rank --title "Prioritize the backlog" "Hosted service" "More patterns" "Authentication"
surface checklist --title "Release checklist" "Run tests" "Review notes" "Publish"
surface approve --title "Ship this release?" --body-file release-notes.md
```

For a custom combination, create an empty surface and add concrete primitives:

```sh
surface create --title "Afternoon workout"
surface add <id> heading "Lower body"
surface add <id> number --id squat_weight --label "Squat weight"
surface add <id> number --id squat_reps --label "Squat reps"
surface add <id> sort --id exercise_order --label "Exercise order" Squat Press Row
```

The first add replaces the empty surface placeholder. The first interactive component
also enables a `Done` action automatically. `pick` is an agent-friendly alias for a
single select, and `sort` is an alias for a ranking list.

Use a complete JSON document when a recipe or a few `surface add` commands would be
more cumbersome. JSON remains the stable protocol and update format:

```sh
surface create examples/gallery.json
surface update <id> examples/gallery.json
```

## Development

```sh
go test ./...
go run ./cmd/surfaced -listen :8080 -data ./data/surfaces.json
go run ./cmd/surface pick --server http://localhost:8080 --title "Pick one" Alpha Beta
```
