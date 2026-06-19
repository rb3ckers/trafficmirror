#!/usr/bin/env bash
set -euo pipefail

GOARCH=${GOARCH:-amd64}

env CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build
