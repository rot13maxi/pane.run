# Agent Surface

A lightweight, disposable web UI substrate for richer agent/human interaction.

This repository contains the V1 Go CLI and hosting service. See
[`docs/product.md`](docs/product.md) for scope and [`docs/protocol.md`](docs/protocol.md)
for the HTTP contract.

## Development

```sh
go test ./...
go run ./cmd/surfaced -listen :8080 -data ./data/surfaces.json
go run ./cmd/surface create examples/gallery.json --server http://localhost:8080
```
