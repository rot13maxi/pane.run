# Local development

Local development uses the same HTTP handler, schema validation, renderer, CLI protocol, capability checks, optimistic revisions, and expiration behavior as the AWS deployment. It replaces DynamoDB and S3 with the repository's filesystem store, so no AWS account, containers, or service emulators are required.

## Start the service

Install Go 1.22 or newer and [`just`](https://just.systems/), then run:

```sh
just run
```

The default service URL is `http://localhost:8080`. Metadata is written atomically to `./data/surfaces.json`, and uploaded assets are stored under `./data/assets/`. The `data` directory is ignored by Git.

In a second terminal, create a test surface:

```sh
just example
```

The command prints the creation response, including the browser URL and private management token. You can also exercise any CLI recipe directly:

```sh
go run ./cmd/surface gallery --server http://localhost:8080 --title "Choose" examples/assets/style-directions/*.png
go run ./cmd/surface checklist --server http://localhost:8080 --title "Release" Build Test Deploy
```

Stop the service with Ctrl-C. Local metadata remains available on the next run. To start with empty state, move or remove `./data` yourself; the project does not provide an automatic destructive cleanup command.

## Configuration

The local recipes accept environment variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `LISTEN` | `127.0.0.1:8080` | Address passed to the HTTP server |
| `BASE_URL` | `http://localhost:8080` | Public URL written into API responses |
| `DATA` | `./data/surfaces.json` | Metadata file; assets use a sibling `assets` directory |

For example:

```sh
LISTEN=127.0.0.1:9090 \
BASE_URL=http://localhost:9090 \
DATA=/tmp/agent-surface-test/surfaces.json \
just run
```

Use the same `BASE_URL` when invoking `just example`:

```sh
BASE_URL=http://localhost:9090 just example
```

## Automated checks

```sh
just test   # all Go tests
just check  # gofmt, go vet, and all tests
just build  # CLI plus the ARM64 Lambda deployment zip
```

HTTP integration tests run the real handler against an ephemeral filesystem store. Hosted-page tests additionally verify that creation writes a complete HTML page and deletion removes it.

## Differences from AWS

Local mode dynamically serves `/s/*` and `/a/*` from the Go process. AWS mode writes those responses to S3 and serves them through CloudFront; only `/api/*` reaches Lambda. Local mode therefore tests product and protocol behavior, but not AWS IAM, CloudFront routing, DynamoDB conditional writes, or S3 lifecycle configuration.

For an integration environment that exercises those services, deploy a separate stack:

```sh
STACK=agent-surface-dev just deploy
```

The deploy command prints that stack's public URL. Pass it to the CLI with `--server`.
