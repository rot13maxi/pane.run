---
name: surface
description: Create and manage temporary, mobile-friendly interactive pages for richer agent-human input and output. Use when chat is awkward for selecting images, ranking options, approving work, completing a checklist, comparing information, or entering structured values in a disposable mini-interface.
---

# Surface

Use the `surface` CLI to create a focused, single-purpose interface. Read
[`references/specification.md`](references/specification.md) before authoring a spec.

Write the declarative JSON spec to a temporary file, then create the surface:

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

Parse the JSON response. Share only `url` with the person and retain `id` for later
commands. **Never reveal, log, quote, or embed `management_token` in a surface.** The
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
