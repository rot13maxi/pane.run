# HTTP protocol

All JSON endpoints use `Content-Type: application/json`. Management endpoints require
`Authorization: Bearer <management-token>` except creation. Errors use
`{"error":{"code":"...","message":"..."}}`.

## Agent endpoints

- `POST /api/v1/surfaces` creates a surface. The body is a surface specification.
- `GET /api/v1/surfaces/{id}` returns its specification and result.
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
- `POST /api/v1/public/{public-id}/submit` marks the current values submitted.
- `POST /api/v1/public/{public-id}/reset` clears values and returns to active.
- `GET /a/{public-id}/{asset-id}` serves an uploaded asset.

State updates are JSON objects with `revision` and `values`. A stale revision returns
HTTP 409 with the current result so the browser can reconcile rather than silently
overwriting a newer edit.
