# AWS deployment

The hosted deployment keeps complete, escaped HTML documents and uploaded assets in a private S3 bucket. CloudFront provides the public HTTPS endpoint and routes `/api/*` to an API Gateway HTTP API backed by one Go Lambda. DynamoDB stores specifications, capability hashes, and optimistic result revisions.

```text
CLI / browser
      |
      v
 CloudFront
   |      \
   |       `-- /api/* --> API Gateway --> Go Lambda --> DynamoDB
   `-- /s/*, /a/* -----------------------> private S3
```

CloudFront caching is disabled in V1. It supplies a single public HTTPS origin, private S3 access, asset security headers, and API routing.

## Prerequisites

- Go 1.22 or newer
- `just`
- AWS CLI v2 with working credentials

The deploying principal needs permission to manage CloudFormation, Lambda, IAM, API Gateway, DynamoDB, S3, CloudFront, and CloudWatch Logs. It also needs access to an S3 packaging bucket; `just deploy` creates an account-and-region-specific bucket when one is not supplied.

## Provisioned resources

The vanilla CloudFormation template creates:

- one on-demand DynamoDB table with encryption, point-in-time recovery, and TTL;
- one encrypted private S3 bucket with an eight-day lifecycle rule;
- one ARM64 `provided.al2023` Lambda and its least-privilege execution role;
- one API Gateway HTTP API with a Lambda proxy integration;
- one CloudFront distribution and Origin Access Control for S3;
- one CloudFront Function that preserves the viewer hostname for generated URLs;
- one response-header policy that sandboxes untrusted uploaded assets; and
- one CloudWatch log group with 14-day retention.

There is no VPC, NAT gateway, load balancer, provisioned database capacity, or continuously running compute. The packaging bucket is created outside the stack and retained for subsequent deployments.

## Deploy

```sh
just deploy
```

The command builds the CLI and ARM64 Lambda bootstrap, packages the function through CloudFormation, deploys the `agent-surface` stack in `us-east-1`, waits for completion, and prints the public CloudFront URL. A new distribution can take several minutes to become reachable.

Configuration is provided through environment variables:

```sh
STACK=my-surface AWS_REGION=us-west-2 just deploy
ARTIFACT_BUCKET=my-existing-artifact-bucket just deploy
```

Inspect the stack outputs with:

```sh
just outputs
```

Use the `SurfaceURL` output as the CLI server:

```sh
surface pick --server https://example.cloudfront.net --title "Choose" Alpha Beta
```

## Storage and expiration

DynamoDB enforces `expires_at` on every application read and write; its TTL feature performs eventual record cleanup. Generated pages fetch authoritative state when loaded, disable interaction on `410 Gone`, and hydrate current saved values. S3 removes page and asset objects after eight days, just beyond the maximum seven-day surface TTL. Consequently, an expired page's inert HTML may remain retrievable until lifecycle cleanup, while its state and interactions are unavailable immediately at expiry.

CloudFront caching is disabled for both generated pages and the API. The S3 bucket is private and can only be read through this stack's distribution.

## Architecture notes

The Lambda continues to use the repository's HTTP handler, schema validation, renderer, and protocol. The hosted store uses strongly consistent DynamoDB reads and conditional writes for revisions. S3 writes the object keys returned by the API directly:

- `s/{public_id}` for a complete HTML document
- `a/{public_id}/{asset_id}` for an uploaded asset

Asset uploads currently pass through API Gateway and Lambda to preserve the existing CLI protocol. API Gateway's request-size ceiling is therefore the effective hosted upload limit; a future presigned-upload flow can remove that constraint.


## Updating and operating the stack

Running `just deploy` again packages the current Lambda and updates the same stack. `--no-fail-on-empty-changeset` makes unchanged deployments succeed. CloudFormation waits for the update before the recipe prints `SurfaceURL`.

Useful diagnostics:

```sh
just outputs
aws logs tail /aws/lambda/agent-surface-surface --follow
aws cloudformation describe-stack-events --stack-name agent-surface
```

Adjust the commands when `STACK` or `AWS_REGION` differs from its default. CloudFront access logging and AWS WAF are intentionally not enabled in V1.

The distribution uses its generated `cloudfront.net` hostname. A custom domain is not currently parameterized; adding one requires an ACM certificate in `us-east-1`, a CloudFront alias, and the corresponding DNS record.

## Local testing

`just run` starts the same application protocol with filesystem persistence and no AWS dependencies. See [`local-development.md`](local-development.md). Use a separately named deployed stack when DynamoDB, S3, Lambda, or CloudFront behavior itself needs integration testing.
