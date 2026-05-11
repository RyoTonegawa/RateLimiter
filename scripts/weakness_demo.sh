#!/usr/bin/env bash
set -euo pipefail

unset LC_ALL
cd "$(dirname "$0")/.."

export GOCACHE="$PWD/.gocache"
export GOMODCACHE="$PWD/.gomodcache"

go run ./cmd/weakness-demo
