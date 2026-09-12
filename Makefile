.PHONY: build test check run

build:
	go build -o bin/pane ./cmd/pane
	go build -o bin/paned ./cmd/paned

test:
	go test ./...

check:
	gofmt -w $$(find . -name '*.go' -type f)
	go vet ./...
	go test ./...

run:
	go run ./cmd/paned -listen :8080 -data ./data/surfaces.json
