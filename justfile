set shell := ["bash", "-euo", "pipefail", "-c"]

stack := env_var_or_default("STACK", "pane-run")
region := env_var_or_default("AWS_REGION", "us-east-1")
listen := env_var_or_default("LISTEN", "127.0.0.1:8080")
base_url := env_var_or_default("BASE_URL", "http://localhost:8080")
data := env_var_or_default("DATA", "./data/surfaces.json")
domain := env_var_or_default("PANE_DOMAIN", "pane.run")
hosted_zone := env_var_or_default("PANE_HOSTED_ZONE_ID", "Z07208213M91D6YKP9C29")
certificate := env_var_or_default("PANE_CERTIFICATE_ARN", "arn:aws:acm:us-east-1:439226424161:certificate/84fc23fc-4edf-42c0-b989-c3c7f71070a7")

_default:
    @just --list

build:
    mkdir -p build bin
    go build -trimpath -o bin/pane ./cmd/pane
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o build/bootstrap ./cmd/pane-lambda
    go run ./cmd/package-lambda build/bootstrap build/lambda.zip

run:
    go run ./cmd/paned -listen "{{listen}}" -base-url "{{base_url}}" -data "{{data}}"

example:
    go run ./cmd/pane pick --server "{{base_url}}" --title "Local test" Alpha Beta Gamma

test:
    go test ./...

check:
    gofmt -w $(find . -name '*.go' -not -path './.git/*' -type f)
    go vet ./...
    go test ./...

deploy: build
    aws acm wait certificate-validated --region us-east-1 --certificate-arn "{{certificate}}"
    account="$(aws sts get-caller-identity --query Account --output text)"; \
    artifact_bucket="${ARTIFACT_BUCKET:-pane-run-artifacts-${account}-{{region}}}"; \
    if ! aws s3api head-bucket --bucket "${artifact_bucket}" >/dev/null 2>&1; then \
      if [ "{{region}}" = us-east-1 ]; then \
        aws s3api create-bucket --bucket "${artifact_bucket}" --region "{{region}}" >/dev/null; \
      else \
        aws s3api create-bucket --bucket "${artifact_bucket}" --region "{{region}}" --create-bucket-configuration LocationConstraint="{{region}}" >/dev/null; \
      fi; \
    fi; \
    aws cloudformation package \
      --region "{{region}}" \
      --template-file infra/template.yaml \
      --s3-bucket "${artifact_bucket}" \
      --s3-prefix "{{stack}}" \
      --output-template-file build/packaged.yaml; \
    aws cloudformation deploy \
      --region "{{region}}" \
      --stack-name "{{stack}}" \
      --template-file build/packaged.yaml \
      --capabilities CAPABILITY_IAM \
      --parameter-overrides DomainName="{{domain}}" HostedZoneId="{{hosted_zone}}" CertificateArn="{{certificate}}" \
      --no-fail-on-empty-changeset; \
    content_bucket="$(aws cloudformation describe-stacks --region "{{region}}" --stack-name "{{stack}}" --query 'Stacks[0].Outputs[?OutputKey==`BucketName`].OutputValue' --output text)"; \
    aws s3 cp site/index.html "s3://${content_bucket}/index.html" --content-type 'text/html; charset=utf-8' --cache-control no-store; \
    aws cloudformation describe-stacks \
      --region "{{region}}" \
      --stack-name "{{stack}}" \
      --query 'Stacks[0].Outputs[?OutputKey==`PaneURL`].OutputValue' \
      --output text

outputs:
    aws cloudformation describe-stacks --region "{{region}}" --stack-name "{{stack}}" --query 'Stacks[0].Outputs' --output table
