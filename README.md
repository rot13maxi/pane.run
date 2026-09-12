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

For a custom combination, pipe an empty surface through concrete additions. A small
review can be expressed in one explicit chain:

```sh
set -o pipefail
surface create --title "Release review" \
  | surface add - heading "Ready to ship?" \
  | surface add - approve --id decision --label "Release decision"
```

A longer composition works the same way:

```sh
set -o pipefail
surface create --title "Afternoon workout" \
  | surface add - heading "Lower body" \
  | surface add - number --id squat_weight --label "Squat weight" \
  | surface add - number --id squat_reps --label "Squat reps" \
  | surface add - sort --id exercise_order --label "Exercise order" Squat Press Row
```

The `-` has special meaning only in the surface-ID position of `surface add`: it
reads the prior command's JSON from standard input and extracts `id`. Commands never
implicitly use standard input or a remembered "last surface." Each add prints a compact
JSON envelope containing `id`, `url`, `revision`, and `status`, so its output can feed
the next add. In Bash, enable `pipefail` so a failure anywhere in the chain fails the
whole pipeline; when scripting, inspect the final envelope before sharing its URL.

The first add replaces the empty surface placeholder. The first interactive component
also enables a `Done` action automatically. `pick` is an agent-friendly alias for a
single select, and `sort` is an alias for a ranking list.

Use a complete JSON document when a recipe or a few `surface add` commands would be
more cumbersome. JSON remains the stable protocol and update format:

```sh
surface create examples/gallery.json
surface update <id> examples/gallery.json
```

## Local development

Run the filesystem-backed service, then create a surface from another terminal:

```sh
just run
just example
```

Use `just test`, `just check`, and `just build` for automated checks and deployment artifacts. See [`docs/local-development.md`](docs/local-development.md) for configuration, persistence, example commands, and differences from AWS.

## Deploy to AWS

A pay-per-use AWS deployment is defined in [`infra/template.yaml`](infra/template.yaml). It uses CloudFront, private S3, API Gateway, Lambda, and DynamoDB. With the prerequisites installed and AWS credentials configured:

```sh
just deploy
```

See [`docs/deployment.md`](docs/deployment.md) for configuration, architecture, and expiration details.
