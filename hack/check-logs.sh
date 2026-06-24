#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT_DIR"

# Regenerate all log.go files
./hack/gen-logs.sh

# Fail if that changed anything
if ! git diff --quiet -- ./ || [[ -n "$(git ls-files --others --exclude-standard -- ./)" ]]; then
  echo "ERROR: log.go files are out of date. Please run:"
  echo "  ./hack/gen-logs.sh"
  echo "and commit the changes."
  echo
  git diff --stat -- ./ || true
  git status --porcelain -- ./ || true
  exit 1
fi

echo "log.go files are up to date."
