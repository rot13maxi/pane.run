# Pane

Pane is hosted at [pane.run](https://pane.run).

Pane lets agents create disposable user interfaces with a CLI. Instead of
generating and hosting a web app, an agent says what interaction it needs—pick, rank,
approve, enter, review—and gets back a URL. The human interacts with the page, the
agent reads the results, and the surface disappears when it's no longer needed.

This repository contains the V1 Go CLI and hosting service. See
[`docs/product.md`](docs/product.md) for scope and [`docs/protocol.md`](docs/protocol.md)
for the HTTP contract.

## Create a surface

Start with a recipe. Recipes cover common agent/human interactions without making
the agent write or retain a specification file:

```sh
pane gallery --title "Pick a direction" --theme dark ./mockups/*.png
pane pick --title "Choose a launch name" "Beacon" "Relay" "Signal"
pane rank --title "Prioritize the backlog" "Hosted service" "More patterns" "Authentication"
pane checklist --title "Release checklist" "Run tests" "Review notes" "Publish"
pane approve --title "Ship this release?" --body-file release-notes.md
```

For a custom combination, pipe an empty surface through concrete additions. A small
review can be expressed in one explicit chain:

```sh
set -o pipefail
pane create --title "Release review" \
  | pane add - heading "Ready to ship?" \
  | pane add - approve --id decision --label "Release decision"
```

A longer composition works the same way:

```sh
set -o pipefail
pane create --title "Afternoon workout" \
  | pane add - heading "Lower body" \
  | pane add - number --id squat_weight --label "Squat weight" \
  | pane add - number --id squat_reps --label "Squat reps" \
  | pane add - sort --id exercise_order --label "Exercise order" Squat Press Row
```

The `-` has special meaning only in the surface-ID position of `pane add`: it
reads the prior command's JSON from standard input and extracts `id`. Commands never
implicitly use standard input or a remembered "last surface." Each add prints a compact
JSON envelope containing `id`, `url`, `revision`, and `status`, so its output can feed
the next add. In Bash, enable `pipefail` so a failure anywhere in the chain fails the
whole pipeline; when scripting, inspect the final envelope before sharing its URL.

The first add replaces the empty surface placeholder. The first interactive component
also enables a `Done` action automatically. `pick` is an agent-friendly alias for a
single select, and `sort` is an alias for a ranking list. Recipes and spec-free
`pane create` also accepts `--theme light|dark|system`. Use a person's remembered
preference when known; otherwise omit `--theme` so the page follows their system.

Use a complete JSON document when a recipe or a few `pane add` commands would be
more cumbersome. JSON remains the stable protocol and update format:

```sh
pane create examples/gallery.json
pane update <id> examples/gallery.json
```

When the person has interacted with the page, bring their structured response back
into the agent loop with:

```sh
pane results <id>
```

The command returns only stable result JSON: `status`, `revision`, `values`, and
lifecycle timestamps. It uses the private receipt saved at creation, so the ID is
normally all it needs.

## Local development

Run the filesystem-backed service, then create a surface from another terminal:

```sh
just run
just example
```

Use `just test`, `just check`, and `just build` for automated checks and deployment artifacts. See [`docs/local-development.md`](docs/local-development.md) for configuration, persistence, example commands, and differences from AWS.

## Optional A2UI compatibility

Pane uses its own small declarative format. When an existing producer already
emits A2UI, the compatibility importer can translate a complete A2UI v0.9.1 Basic
Catalog batch into a disposable Surface:

```sh
pane create messages.jsonl --format a2ui --title "Review options"
# or: producer | pane create - --format a2ui --title "Review options"
```

This is a one-way import path, not an alternative runtime or authoring recommendation.
Surface V1 remains the persisted and update format. See `docs/protocol.md` for the
deliberately restricted supported subset.

## Deploy to AWS

A pay-per-use AWS deployment is defined in [`infra/template.yaml`](infra/template.yaml). It uses CloudFront, private S3, API Gateway, Lambda, and DynamoDB. With the prerequisites installed and AWS credentials configured:

```sh
just deploy
```

See [`docs/deployment.md`](docs/deployment.md) for configuration, architecture, and expiration details.
