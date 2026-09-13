# Surface specification V1

A surface is one JSON document. The agent declares ordered content and interaction
primitives; the renderer owns their final responsive layout. Unknown component kinds
are invalid, and interactive components require a stable `id` used as their state key.

Most agents should not need to author this document directly. The recipe commands
(`gallery`, `pick`, `rank`, `checklist`, and `approve`) generate specifications for
common interactions, while `pane add` composes a small custom surface one primitive
at a time. Recipe interactions are required by default and accept `--optional`. For
add commands, `pick` aliases `select` and `sort` aliases `ranking`. The first added
component replaces the seed placeholder; the first interactive one adds Submit. Adds
can be chained with `pane create ... | pane add - ...`. The `-` is explicit and
only consumes a prior JSON result when used in the add command's surface-ID slot; no
command implicitly reads standard input or remembers the last surface. Add output is a
compact envelope with `id`, `url`, `revision`, and `status`. Shell scripts should use
`pipefail` for reliable pipeline failure detection. Use raw JSON as the escape hatch
for unusual or highly structured surfaces, and as the canonical format for
reproducible updates.

```json
{
  "version": "1",
  "title": "Choose a direction",
  "description": "Pick the designs you prefer.",
  "ttl_seconds": 86400,
  "presentation": { "tone": "professional", "density": "comfortable", "color_scheme": "dark" },
  "components": [],
  "actions": { "submit": { "label": "Done" }, "reset": { "label": "Reset" } }
}
```

`ttl_seconds` is optional. It defaults to 86,400 seconds and must be between 60
seconds and 604,800 seconds. Supported tones are `neutral`, `warm`, `playful`, and
`professional`. Density is `comfortable` or `compact`. `color_scheme` is `light`,
`dark`, or `system`; omitting it defaults to `system`. Recipes and spec-free
`pane create` exposes this as `--theme`. Agents should use a remembered user
preference when known and otherwise omit the flag. Presentation fields are semantic
hints, not layout or arbitrary styling instructions.

## Components

Every component has `kind`. Interactive components also require `id` and `label`, and
may include `help` and `required`.

| Kind | Purpose | Kind-specific fields | Stored value |
| --- | --- | --- | --- |
| `heading` | Heading | `content`, optional `level` 1-3 | none |
| `text` | Text block | `content` | none |
| `image` | Image | exactly one of `url` or `asset`, optional `alt` | none |
| `link` | External link | `label`, HTTP(S) `url` | none |
| `divider` | Visual separator | none | none |
| `section` | Semantic grouping | `label`, `components` | none |
| `input_text` | Single-line text | `placeholder`, `min_length`, `max_length` | string |
| `textarea` | Multi-line text | `placeholder`, `min_length`, `max_length` | string |
| `number` | Numeric entry | `min`, `max`, `step` | number |
| `checkbox` | Boolean choice | none | boolean |
| `toggle` | Boolean choice | none | boolean |
| `select` | One choice | `options` | option value string |
| `multi_select` | Multiple choices | `options`, selection bounds | string array |
| `checklist` | Checkable item list | `items`, selection bounds | item value array |
| `gallery` | Image picker | `items`, selection bounds | item value array |
| `ranking` | Sort/prioritize items | `items` | complete item value array |
| `approval` | Approve or reject | none | `approved` or `rejected` |
| `comparison` | Read-only table | `columns`, `rows` | none |

Selection bounds are `min_selections` and `max_selections`. Options contain `value`,
`label`, and optional `description`. Items contain those fields plus optional `image`;
gallery items require an image. Values within a component must be unique.

A ranking result is a permutation of the authored item values: every item appears
exactly once, with no duplicates, omissions, or write-ins. Select and multi-select
results likewise accept only authored option values.

Ranking state starts in the authored item order, so the displayed initial order is a
real result even when the respondent submits without moving an item.

Sections are semantic groups, not agent-controlled layouts. They may nest to four
levels. A surface may contain at most 200 components and 200 state keys.

## State and validation

The state document is a JSON object keyed by interactive component IDs. Browser edits
replace this object using an optimistic `revision`. Partial autosaves validate every
present value but do not enforce `required`; submission additionally enforces required
fields. This allows progress to remain readable before the person is finished.

On a definition update, the service retains a value only when its ID still exists,
its old and new component kinds share the same JSON value shape, and the value remains
valid under the new component's bounds and allowed choices. Text and textarea, for
example, are compatible. A checkbox changed to text is not.

See [`../examples/gallery.json`](../examples/gallery.json) and
[`../examples/workout.json`](../examples/workout.json) for complete documents.
