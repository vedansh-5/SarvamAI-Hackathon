#!/bin/sh
set -eu

cd "$(dirname "$0")/../.."
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT

GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -trimpath -ldflags="-s -w" -o "$out/bootstrap" ./cmd/vans-lambda
(cd "$out" && zip -q vans-server.zip bootstrap)

aws lambda update-function-code --profile vans-deploy --region ap-south-1 \
  --function-name vans-server --zip-file "fileb://$out/vans-server.zip" \
  --query 'LastModified' --output text
aws lambda wait function-updated-v2 --profile vans-deploy --region ap-south-1 --function-name vans-server
aws lambda update-function-configuration --profile vans-deploy --region ap-south-1 --function-name vans-server \
  --description "$(git rev-parse --short HEAD)$(git diff --quiet HEAD || echo '+changes') $(date -u +%Y-%m-%d)" --query Description --output text
aws lambda wait function-updated-v2 --profile vans-deploy --region ap-south-1 --function-name vans-server
curl -fsS "$(aws lambda get-function-url-config --profile vans-deploy --region ap-south-1 --function-name vans-server --query FunctionUrl --output text)v1/health"
echo
