#!/usr/bin/env bash
# Regenerate every package's log.go. Thin wrapper around the real generator,
# which lives in build/gen/logs.go (`make gen logs`).
set -euo pipefail

ROOT_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT_DIR"

exec go run ./build/gen --mode=force logs
