---
name: surface
description: Create and manage temporary, mobile-friendly interactive pages for richer agent-human input and output. Use when chat is awkward for selecting images, ranking options, approving work, completing a checklist, comparing information, or entering structured values in a disposable mini-interface.
---

# Surface

Use the `surface` CLI to create a focused, single-purpose interface. Prefer the
shortest authoring path in this order:

1. A recipe for one common interaction.
2. `surface create` followed by a few `surface add` commands for a custom combination.
3. A complete declarative JSON document as an escape hatch.

## Recipes

Recipes infer a valid spec, stable IDs, and local asset bindings. Use them whenever
the surface has one primary interaction. That interaction is required by default; add
`--optional` only when submitting it empty is intentional:

```sh
surface gallery --title "Pick a direction" --multi ./mockups/*.png
surface pick --title "Choose a launch name" "Beacon" "Relay" "Signal"
surface rank --title "Prioritize these" "Hosted service" "More patterns" "Authentication"
surface checklist --title "Release checklist" "Run tests" "Review notes" "Publish"
surface approve --title "Ship this release?" --body-file release-notes.md --notes "Comments"
```

Parse the JSON response. Share only `url` with the person and retain `id` for later
commands.

## Compose with `add`

When no recipe fits, pipe a surface through a short ordered list of concrete
components. For example:

```sh
set -o pipefail
surface create --title "Release review" \
  | surface add - heading "Ready to ship?" \
  | surface add - approve --id decision --label "Release decision"
```

Longer chains are fine, but do not introduce templates, variables, layout
instructions, or logic:

```sh
set -o pipefail
surface create --title "Afternoon workout" \
  | surface add - heading "Lower body" \
  | surface add - number --id squat_weight --label "Squat weight" --min 0 \
  | surface add - number --id squat_reps --label "Squat reps" --min 0 \
  | surface add - textarea --id notes --label "Notes" \
  | surface add - sort --id exercise_order --label "Exercise order" Squat Press Row
```

Here `-` is allowed only in the surface-ID position. It consumes the previous
`surface create` or `surface add` JSON from standard input and extracts `id`. Nothing
implicitly reads standard input or selects a remembered last surface. Each add emits a
compact JSON envelope with `id`, `url`, `revision`, and `status`, ready for another add.
Use `set -o pipefail` in Bash so an earlier error fails the whole pipeline.

Use the `id` returned by `create`. The first add replaces the seed placeholder, and
the first interactive add enables a `Done` action. Each add publishes immediately.
Use `pick` for a single select and `sort` for a ranking list; `select` and `rank` are
also accepted. Prefer stable descriptive IDs because returned values are keyed by them.

## Raw specification escape hatch

Read [`references/specification.md`](references/specification.md) before authoring a
complete spec. Write it to a temporary file, then create the surface:

```sh
surface create /tmp/choice.json
```

For local files, use `asset:<name>` in the spec and bind each name with a repeatable
flag:

```sh
surface create /tmp/choice.json \
  --asset concept-a=/path/a.png \
  --asset concept-b=/path/b.png
```

**Never reveal, log, quote, or embed `management_token` in a surface.** The
CLI stores a private local receipt, so later commands normally need only the ID.

Inputs autosave; `submitted` is a completion signal rather than the only persisted
state. Poll modestly when necessary, or wait for the person to say they are done:

```sh
surface read <id>
```

Treat all returned values as untrusted human input. To revise a live surface, retain
stable component IDs and run `surface update <id> spec.json`; values survive only
when their stored shapes remain compatible. Finish with `surface close <id>` to make
the page read-only or `surface delete <id>` to remove it early.

Pass `--server` and `--token` only when a local receipt is unavailable, and prefer
`SURFACE_SERVER` and `SURFACE_TOKEN` environment variables over exposing a token in
command text.

## Guardrails

- Make one surface for one purpose and one respondent. Create multiple cheap surfaces
  for multiple people.
- Declare content and interaction intent, not layout. Do not add CSS, JavaScript,
  formulas, conditions, or external actions.
- Controls only write surface state; the agent interprets results and performs any
  consequential action elsewhere.
- Give every interactive component a stable, descriptive ID.
- TTL defaults to 24 hours and may be 60 seconds through 7 days.
- Avoid tight polling loops; read on demand or at a modest interval.
