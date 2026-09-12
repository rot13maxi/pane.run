# Agent Surface V1

Agent Surface lets an agent materialize a temporary, purpose-built web interface when
chat is the wrong medium. The agent submits a declarative specification and receives
a responsive public URL. A person interacts with that page, the browser auto-saves
values, and the agent polls for structured results.

## Principles

1. Zero configuration: installing the skill and CLI is enough to create a surface.
2. Intent over layout: agents choose primitives and light tone/density hints; the
   renderer chooses placement and responsive behavior.
3. Disposable by design: the default TTL is 24 hours and the maximum is 7 days.
4. Capability security: an unguessable public ID grants interaction access; a separate
   token grants read, update, close, and delete access.
5. State, not effects: UI controls only update a small JSON key-value document.
6. No programming language: validation is local and declarative; there are no scripts,
   formulas, arbitrary styling, or conditional business logic.

## V1 workflow

1. The agent chooses the shortest authoring path: a recipe such as `surface gallery`,
   `surface pick`, `surface rank`, `surface checklist`, or `surface approve` for the
   common case; `surface add` for a small custom composition; or a complete JSON
   specification as an escape hatch.
2. The CLI prints a public URL, surface ID, management token, and expiry time. It also
   writes a local receipt so later commands need only the ID.
3. A person opens the URL on desktop or mobile. Edits auto-save. Submit changes the
   status to `submitted` but does not lock the page.
4. `surface read <id>` returns status, revision, timestamps, and values.
5. The agent may replace the specification with `surface update`; compatible values
   survive. It may close the surface to make it read-only or delete it early.

Recipes and incremental commands are agent UX, not separate platform concepts. They
compile to the same declarative specification accepted by the HTTP service. Incremental
commands may be chained explicitly as `surface create ... | surface add - ...`; only
`-` in the add command's surface-ID position reads a prior JSON result from standard
input. Add emits a compact, chainable result envelope. There is no implicit standard
input or remembered last surface. Shell scripts should enable `pipefail` so an earlier
failed command is not hidden by pipeline status. The service remains intentionally
unaware of shell commands and does not become a general application builder.

## Supported input formats

Surface V1 remains the canonical stored and update format. The creation API and CLI
may also import a complete A2UI v0.9 or v0.9.1 Basic Catalog batch through a strict,
non-executable translation boundary. Import does not turn the service into a streaming
A2UI runtime: custom catalogs, agent callbacks, arbitrary functions, and external
actions remain outside the product boundary.

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
