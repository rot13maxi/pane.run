# HTTP protocol

Surface V1 JSON is the canonical protocol. Its endpoints use
`Content-Type: application/json`. Management endpoints require
`Authorization: Bearer <management-token>` except creation. Errors use
`{"error":{"code":"...","message":"..."}}`.

## Agent endpoints

- `POST /api/v1/surfaces` creates a surface. The body is a surface specification.
- `GET /api/v1/surfaces/{id}` returns its specification and result.
- `GET /api/v1/surfaces/{id}/results` returns only the agent-facing result: status,
  revision, values, and lifecycle timestamps.
- `PUT /api/v1/surfaces/{id}` replaces its specification and preserves compatible
  values.
- `POST /api/v1/surfaces/{id}/assets` uploads one bounded asset. `X-Filename` carries
  its original name. The response contains a stable asset URL.
- `POST /api/v1/surfaces/{id}/close` makes the page read-only.
- `DELETE /api/v1/surfaces/{id}` deletes it.

Creation returns `id`, `url`, `management_token`, `created_at`, and `expires_at`.

## Public endpoints

- `GET /s/{public-id}` renders the surface.
- `GET /api/v1/public/{public-id}/state` reads public state needed by the runtime.
- `PUT /api/v1/public/{public-id}/state` replaces values using an expected revision.
- `POST /api/v1/public/{public-id}/submit` marks the current values submitted and
  permanently makes the surface read-only.
- `POST /api/v1/public/{public-id}/reset` clears values and returns to active before
  submission.
- `GET /a/{public-id}/{asset-id}` serves an uploaded asset.

State updates are JSON objects with `revision` and `values`. A stale revision returns
HTTP 409 with the current result so the browser can reconcile rather than silently
overwriting a newer edit.

Once submitted, state writes, reset, repeat submission, definition updates, and asset
uploads return HTTP 409 with error code `submitted`. Closing or deleting the surface
remains permitted; closing preserves an already-submitted result and status.

## Optional A2UI compatibility import

`POST /api/v1/imports/a2ui?protocol=v0.9.1` accepts a bounded complete batch of
A2UI v0.9 or v0.9.1 envelopes as either a JSON array or a JSON/JSONL sequence.
The request content type is `application/a2ui+json`. Optional `title`,
`description`, and `ttl_seconds` query parameters supply Pane lifecycle
metadata, which A2UI does not define.

This endpoint is a one-way compatibility adapter for existing A2UI producers, not a
primary creation path or runtime protocol. The importer supports a non-executable
subset of the A2UI Basic Catalog and compiles it to a canonical Surface V1 document.
Supported components are
`Column`, `Row`, `Card`, `Text`, `Divider`, `Image`, `TextField`, `CheckBox`,
`ChoicePicker`, and `Button`. Containers are flattened because the renderer owns
layout. Buttons are accepted only for `submit` and `reset` events. Custom
catalogs, function calls, dynamic child templates, other actions, and deleted
surfaces are rejected.

The normal creation fields are returned with an additional `import` object
containing the source surface ID, selected protocol, and translation warnings.
Initial values resolved through A2UI data bindings are installed as revision-zero
Surface state.

## Waiting from the CLI

`pane wait <id> [--timeout DURATION]` polls the agent-facing results endpoint until
the result status is `submitted`, then prints that result. A zero or omitted timeout
waits indefinitely. On timeout, it writes `{"status":"timeout","id":"..."}` to
standard output and exits nonzero. This is intentionally a CLI behavior rather than
a separate HTTP endpoint; clients can use the same bounded polling approach.
