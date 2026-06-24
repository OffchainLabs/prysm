#!/bin/bash

# Continuous Integration script to check that BUILD.bazel files and deps.bzl
# are as expected when generated from gazelle.

set -euo pipefail

exit_code=0

# Duplicate redirect 5 to stdout so that it can be captured, but still printed
# nicely.
exec 5>&1

echo "Checking deps.bzl is in sync with go.mod..."
bazel --batch --bazelrc=.buildkite-bazelrc run //:gazelle -- update-repos -from_file=go.mod -to_macro=deps.bzl%prysm_deps -prune=true

if git diff --exit-code deps.bzl; then
  echo "OK: deps.bzl is in sync with go.mod"
else
  echo ""
  echo "FAIL: deps.bzl is out of sync with go.mod"
  exit_code=1
fi

echo ""
echo "Checking BUILD.bazel files..."
build_changes=$(bazel --batch --bazelrc=.buildkite-bazelrc run //:gazelle -- fix --mode=diff | tee >(cat - >&5)) || true

if [ -z "$build_changes" ]; then
  echo "OK: BUILD.bazel files are in sync"
else
  echo "FAIL: BUILD.bazel files are out of sync"
  exit_code=1
fi

echo ""
if [ $exit_code -eq 0 ]; then
  echo "All gazelle checks passed"
else
  echo "Gazelle checks failed. Please run:"
  echo "  bazel run //:gazelle -- update-repos -from_file=go.mod -to_macro=deps.bzl%prysm_deps -prune=true"
  echo "  bazel run //:gazelle -- fix"
fi

exit $exit_code
