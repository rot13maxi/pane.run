# Pane V1

Surface lets agents create disposable user interfaces with a CLI. Instead of
generating and hosting a web app, an agent says what interaction it needs—pick, rank,
approve, enter, review—and gets back a URL. The human interacts with the page, the
agent reads the results, and the surface disappears when it's no longer needed.

## Principles

1. Zero configuration: installing the skill and CLI is enough to create a surface.
2. Intent over layout: agents choose primitives and light tone, density, and color
   scheme hints; the renderer chooses placement and responsive behavior. A remembered
   user preference may select light or dark; otherwise omission follows the user's
   operating-system preference.
3. Disposable by design: the default TTL is 24 hours and the maximum is 7 days.
4. Capability security: an unguessable public ID grants interaction access; a separate
   token grants read, update, close, and delete access.
5. State, not effects: UI controls only update a small JSON key-value document.
6. No programming language: validation is local and declarative; there are no scripts,
   formulas, arbitrary styling, or conditional business logic.

## V1 workflow

1. The agent chooses the shortest authoring path: a recipe such as `pane gallery`,
   `pane pick`, `pane rank`, `pane checklist`, or `pane approve` for the
   common case; `pane add` for a small custom composition; or a complete JSON
   specification as an escape hatch. Recipes and spec-free create accept
   `--theme light|dark|system`; agents use a remembered preference when known and
   otherwise omit it for the system default.
2. The CLI prints a public URL, surface ID, management token, and expiry time. It also
   writes a local receipt so later commands need only the ID.
3. A person opens the URL on desktop or mobile. Edits auto-save. Submit changes the
   status to `submitted` but does not lock the page.
4. `pane results <id>` returns status, revision, timestamps, and values without
   mixing the response with the authoring specification.
5. The agent may replace the specification with `pane update`; compatible values
   survive. It may close the surface to make it read-only or delete it early.

Recipes and incremental commands are agent UX, not separate platform concepts. They
compile to the same declarative specification accepted by the HTTP service. Incremental
commands may be chained explicitly as `pane create ... | pane add - ...`; only
`-` in the add command's surface-ID position reads a prior JSON result from standard
input. Add emits a compact, chainable result envelope. There is no implicit standard
input or remembered last surface. Shell scripts should enable `pipefail` so an earlier
failed command is not hidden by pipeline status. The service remains intentionally
unaware of shell commands and does not become a general application builder.

## Optional compatibility import

Surface V1 is the product's canonical creation, storage, and update format. A2UI is
supported only as a convenience for existing producers: the creation API and CLI can
translate a complete A2UI v0.9 or v0.9.1 Basic Catalog batch through a strict,
non-executable boundary. It is not a second authoring model or a streaming A2UI
runtime. Custom catalogs, agent callbacks, arbitrary functions, and external actions
remain outside the product boundary.

## Initial primitives

Content: heading, text, image, link, divider, and section.

Inputs: text, textarea, number, checkbox, toggle, select, and multi-select.

Compound interactions: checklist, gallery picker, ranking list, approval, and
comparison. Compound primitives render polished domain-appropriate controls while
writing ordinary JSON values under one stable component ID.

Actions: submit and reset. Submit is a completion signal rather than a permanent lock.

## Success criteria

- A new surface can be created and opened in seconds with no account.
- Generated pages are useful and touch-friendly without agent-authored layout code.
- CLI output is machine-readable and stable enough for an agent skill.
- Every human edit is recoverable through polling, even without explicit submission.
- The service runs as a single inexpensive binary with durable local storage.
