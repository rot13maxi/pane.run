# Surface V1 specification

A surface is a JSON document. The service owns responsive layout; the agent supplies
ordered content, interaction primitives, and optional presentation hints.

This is the canonical protocol and the escape hatch, not the default authoring path.
First prefer a top-level `pane gallery`, `pick`, `rank`, `checklist`, or `approve`
recipe. For a small mixed surface, use `pane create --title ...` and then
`pane add <id> <kind> ...`. Write JSON directly when that is clearer or when the
full document needs to be reproducible or updated atomically. Recipe interactions are
required by default; pass `--optional` only to permit empty submission. In incremental
composition, `pick` aliases `select` and `sort` aliases `ranking`. The first add
replaces the seed placeholder, and the first interactive add enables Submit. Chain
composition explicitly with `pane create ... | pane add - ...`. Only `-` in the
add command's surface-ID position consumes the prior JSON on standard input; there is
no implicit stdin or remembered last surface. Add prints a compact envelope containing
`id`, `url`, `revision`, and `status` for the next command. Bash scripts should enable
`pipefail` so any failed stage fails the pipeline.

```json
{
  "version": "1",
  "title": "Choose a direction",
  "description": "Pick the designs you prefer.",
  "ttl_seconds": 86400,
  "presentation": { "tone": "professional", "density": "comfortable", "color_scheme": "dark" },
  "components": [
    {
      "kind": "select",
      "id": "direction",
      "label": "Direction",
      "required": true,
      "options": [
        { "value": "editorial", "label": "Editorial" },
        { "value": "minimal", "label": "Minimal" }
      ]
    }
  ],
  "actions": { "submit": { "label": "Done" }, "reset": { "label": "Reset" } }
}
```

`ttl_seconds` defaults to 86,400 and must be 60–604,800. Tone is `neutral`, `warm`,
`playful`, or `professional`; density is `comfortable` or `compact`. `color_scheme`
is `light`, `dark`, or `system`, and omission defaults to `system`. Recipes and
spec-free create expose it as `--theme`. Use a remembered user preference when known;
otherwise omit the flag. These are semantic hints, not styling or layout controls.

## Components

| Kind | Fields | Stored value |
| --- | --- | --- |
| `heading` | `content`, optional `level` 1–3 | — |
| `text` | `content` | — |
| `image` | exactly one of `url` or `asset`, optional `alt` | — |
| `link` | `label`, HTTP(S) `url` | — |
| `divider` | — | — |
| `section` | `label`, nested `components` | — |
| `input_text` | `label`, optional placeholder/length bounds | string |
| `textarea` | `label`, optional placeholder/length bounds | string |
| `number` | `label`, optional `min`, `max`, `step` | number |
| `checkbox`, `toggle` | `label` | boolean |
| `select` | `label`, `options` | option value string |
| `multi_select` | `label`, `options`, selection bounds | string array |
| `checklist` | `label`, `items`, selection bounds | item value array |
| `gallery` | `label`, image `items`, selection bounds | item value array |
| `ranking` | `label`, `items` | complete item value array |
| `approval` | `label` | `approved` or `rejected` |
| `comparison` | `label`, `columns`, `rows` | — |

Every component has `kind`. Interactive components require a unique stable `id` and
`label`; IDs must match `[A-Za-z][A-Za-z0-9_-]{0,63}`. Interactive components may
also have `help` and `required`. Selection bounds are `min_selections` and
`max_selections`. Options have `value`, `label`, and optional `description`; items
add optional `image`, and gallery items require one. Each options or items list must
contain 1–100 entries with unique values.

A ranking result is a permutation of the authored item values: every item appears
exactly once, with no duplicates, omissions, or write-ins. Select and multi-select
results likewise accept only authored option values.

Ranking state starts in the authored item order, so the displayed initial order is a
real result even when the respondent submits without moving an item.

`components` must be non-empty. Sections are semantic groups, may nest four levels,
and do not control layout. A surface supports at most 200 components and 200 state
keys. Partial autosaves validate present values without enforcing `required`;
submission enforces required values. Definition updates preserve a value only when
its stable ID, JSON shape, constraints, and allowed choices remain compatible.

## Raw JSON workflow

```sh
pane create spec.json
pane wait <id> --timeout 30m
pane results <id>
pane update <id> spec.json
pane close <id>
pane delete <id>
```

`pane wait` polls until submission and returns the shallow agent-facing result object;
omit its timeout to wait indefinitely. `pane results` returns the same shape
immediately as a snapshot. Use `pane read` only when inspecting the full stored
specification and management document is necessary.

To upload local media, reference `asset:<name>` in the JSON and bind it during create:

```sh
pane create spec.json --asset name=/absolute/path/image.png
```

The create response contains `url`, `id`, and `management_token`. Share only `url`.
Never expose, log, or place the management token in a spec or surface; rely on the
CLI's private receipt, or environment variables when transferring management access.
