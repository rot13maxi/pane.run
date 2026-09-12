# Agent Surface contributor guide

## Product boundary

Agent Surface is an ephemeral I/O substrate for agents, not an application builder.
Agents declare content and interaction primitives; the service owns layout, responsive
rendering, hosting, and a tiny key-value state store. Controls may update state but
must not trigger arbitrary external side effects.

V1 is anonymous and capability-based. Creation returns a public surface URL and a
private management token. Surfaces expire after 24 hours by default and may request a
TTL between 1 minute and 7 days.

## Repository layout

- `cmd/surface`: Go CLI.
- `cmd/surfaced`: HTTP service.
- `internal/schema`: shared specification, validation, and result types.
- `internal/store`: persistence and capability checks.
- `internal/server`: HTTP API and asset handling.
- `internal/render`: HTML/CSS/JavaScript page renderer.
- `docs`: product and protocol documentation.
- `examples`: runnable surface specifications.

## Engineering rules

- Go 1.22+, standard library first. New dependencies require a clear justification.
- Keep package boundaries acyclic: schema <- store/render <- server; CLI consumes the
  public HTTP protocol and must not import server internals.
- Treat all specifications, uploaded filenames, state values, URLs, and tokens as
  untrusted input. Escape rendered content and cap request sizes.
- Public IDs and management tokens must come from `crypto/rand`.
- State writes use optimistic revisions. Definition updates preserve values only for
  keys whose interactive component kind remains compatible.
- Tests belong beside packages; end-to-end tests exercise binaries or the HTTP API.
- Run `gofmt -w` on changed Go files and `go test ./...` before handoff.

## V1 non-goals

Authentication, accounts, multiple respondents per surface, webhooks, arbitrary
scripts or CSS, formulas, cross-field logic, external actions, audit logs, and
multi-node persistence.
