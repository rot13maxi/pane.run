# Import existing A2UI

This is an optional, one-way compatibility path. Use it only for an existing complete
A2UI v0.9 or v0.9.1 batch. For a new surface, prefer recipes, incremental composition,
or the native Surface specification.

```sh
surface create messages.jsonl --format a2ui --title "Review options"
producer | surface create - --format a2ui --title "Review options"
```

Pass `--description` and `--ttl` when needed. A2UI does not define Agent Surface
lifecycle metadata, so the title defaults to `Imported A2UI surface` and the normal
24-hour TTL applies when flags are omitted. `--asset` is unavailable for A2UI imports.

The service accepts a complete ordered batch as a JSON array or JSON/JSONL sequence.
It translates a strict, non-executable subset of the v0.9 Basic Catalog into a native
Surface V1 document and installs resolved input bindings as revision-zero state.

Supported components:

- Containers: `Column`, `Row`, and `Card`. These are flattened because Surface owns
  layout; importing a `Row` produces a warning.
- Content: `Text`, `Divider`, and HTTP(S) `Image`.
- Inputs: `TextField` with `shortText`, `longText`, or `number`; `CheckBox`; and
  `ChoicePicker` with mutually exclusive or multiple selection.
- Actions: `Button` events named exactly `submit` or `reset`.

The import fails closed on custom catalogs, function calls, dynamic child templates,
unknown components, other button events, missing references, graph cycles, mismatched
surface IDs, or a deleted surface. A2UI validation checks are not translated and
produce a warning. Treat the returned `import.warnings` as material loss information.

After import, use `surface results <id>` for the agent-facing response. Use the
ordinary `update`, `close`, and `delete` commands for lifecycle management, and
reserve `surface read` for cases that require the full canonical specification.
Updates use canonical Surface V1 JSON, not incremental A2UI messages. Share only the
returned public `url`; never expose the management token.
