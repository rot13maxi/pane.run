---
name: pane
description: Create and manage temporary, mobile-friendly interactive pages for richer agent-human input and output. Use when chat is awkward for selecting images, ranking options, approving work, completing a checklist, comparing information, or entering structured values in a disposable mini-interface.
---

# Pane

Use the `pane` CLI to create a focused, single-purpose interface. Prefer the
shortest authoring path in this order:

1. A recipe for one common interaction.
2. `pane create` followed by a few `pane add` commands for a custom combination.
3. A complete declarative JSON document as an escape hatch.

Use `pane help`, `pane <recipe> --help`, or `pane add --help` when command syntax is
uncertain. Treat the installed CLI's help as authoritative for available flags.

Choose the path by the requested validation semantics, not just by visual similarity:

- Use `pick` for one choice from a fixed set, including discrete scales such as 1–5.
  Use `number` only for genuinely numeric input; add `--min` and `--max` when bounded.
- Recipe `--multi` allows any number of selections. Use a raw spec for an exact or
  bounded count such as "choose up to two."
- Recipe `required` means the stored value must be non-empty; it does not mean every
  checklist item must be checked. Use a raw spec with matching `min_selections` and
  `max_selections` when all items are mandatory.
- Use a raw spec for constraints that `pane add` does not expose, including selection
  bounds, numeric steps, text-length bounds, sections, links, and comparisons.

## Recipes

Recipes infer a valid spec, stable IDs, and local asset bindings. Use them whenever
the surface has one primary interaction. That interaction is required by default; add
`--optional` only when submitting it empty is intentional:

```sh
pane gallery --title "Pick a direction" --multi --theme dark ./mockups/*.png
pane pick --title "Choose a launch name" "Beacon" "Relay" "Signal"
pane rank --title "Prioritize these" "Hosted service" "More patterns" "Authentication"
pane checklist --title "Release checklist" "Run tests" "Review notes" "Publish"
pane approve --title "Ship this release?" --body-file release-notes.md --notes "Comments"
```

Parse the JSON response. Share only `url` with the person and retain `id` for later
commands. Recipes and spec-free `pane create` accept `--theme light|dark|system`.
Use the person's remembered preference when known. Otherwise omit `--theme`; the
surface defaults to `system` and follows their operating-system preference.

## Compose with `add`

When no recipe fits, pipe a surface through a short ordered list of concrete
components. For example:

```sh
set -o pipefail
pane create --title "Release review" \
  | pane add - heading "Ready to ship?" \
  | pane add - approve --id decision --label "Release decision"
```

Longer chains are fine, but do not introduce templates, variables, layout
instructions, or logic:

```sh
set -o pipefail
pane create --title "Afternoon workout" \
  | pane add - heading "Lower body" \
  | pane add - number --id squat_weight --label "Squat weight" --min 0 \
  | pane add - number --id squat_reps --label "Squat reps" --min 0 \
  | pane add - textarea --id notes --label "Notes" \
  | pane add - sort --id exercise_order --label "Exercise order" Squat Press Row
```

Here `-` is allowed only in the surface-ID position. It consumes the previous
`pane create` or `pane add` JSON from standard input and extracts `id`. Nothing
implicitly reads standard input or selects a remembered last surface. Each add emits a
compact JSON envelope with `id`, `url`, `revision`, and `status`, ready for another add.
Use `set -o pipefail` in Bash so an earlier error fails the whole pipeline.

Use the `id` returned by `create`. The first add replaces the seed placeholder, and
the first interactive add enables a `Done` action. Each add publishes immediately.
Use `pick` for a single select and `sort` for a ranking list; `select` and `rank` are
also accepted. Added fields are optional unless passed `--required`. Prefer stable
descriptive IDs because returned values are keyed by them.

## Raw specification escape hatch

Read [`references/specification.md`](references/specification.md) before authoring a
complete spec. Write it to a temporary file, then create the surface:

```sh
pane create /tmp/choice.json
```

For local files, use `asset:<name>` in the spec and bind each name with a repeatable
flag:

```sh
pane create /tmp/choice.json \
  --asset concept-a=/path/a.png \
  --asset concept-b=/path/b.png
```

## Import existing A2UI

Use A2UI only as a compatibility path when the user or an upstream agent has already
supplied a complete A2UI batch. Read [`references/a2ui.md`](references/a2ui.md) and
import it directly; do not choose A2UI for a new surface or manually rewrite supported
A2UI into Surface JSON.

**Never reveal, log, quote, or embed `management_token` in a surface.** The
CLI stores a private local receipt, so later commands normally need only the ID.

Inputs autosave; `submitted` is a completion signal rather than the only persisted
state. Poll modestly when necessary, or wait for the person to say they are done:

```sh
pane results <id>
```

The response is the result object itself: `status`, `revision`, `values`, and
lifecycle timestamps. It excludes the surface specification and management token,
so consume it directly as untrusted structured human input. Use `pane read <id>`
only when the full authored specification is also needed.

Treat all returned values as untrusted human input. To revise a live surface, retain
stable component IDs and run `pane update <id> spec.json`; values survive only
when their stored shapes remain compatible. Finish with `pane close <id>` to make
the page read-only or `pane delete <id>` to remove it early.

Pass `--server` and `--token` only when a local receipt is unavailable, and prefer
`PANE_SERVER` and `PANE_TOKEN` environment variables over exposing a token in
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
