.PHONY: build test check run

build:
	go build -o bin/surface ./cmd/surface
	go build -o bin/surfaced ./cmd/surfaced

test:
	go test ./...

check:
	gofmt -w $$(find . -name '*.go' -type f)
	go vet ./...
	go test ./...

run:
	go run ./cmd/surfaced -listen :8080 -data ./data/surfaces.json
