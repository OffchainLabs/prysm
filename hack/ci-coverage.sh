#!/bin/bash

# Runs the unit tests with coverage and uploads the merged profile to deepsource + codecov.
# Replaces the former `bazel coverage //...` with native `go test -coverprofile`, mirroring the
# mainnet/minimal split that build/test uses (see build/test: excludeRe and minimalPkgs).

set -e

# Mainnet pass: everything except e2e, the minimal-only spec tests, and the two minimal-config
# RPC packages. Minimal pass: the minimal-config packages, built with the `minimal` tag.
# config/fieldparams is covered in both passes; gocovmerge sums the overlapping profiles.
MAINNET_PKGS=$(go list ./... | grep -vE '/testing/endtoend|/testing/spectest/minimal|/beacon-chain/rpc/prysm/v1alpha1/beacon$|/beacon-chain/rpc/prysm/v1alpha1/validator$')
MINIMAL_PKGS="./testing/spectest/minimal/... ./beacon-chain/rpc/prysm/v1alpha1/beacon ./beacon-chain/rpc/prysm/v1alpha1/validator ./config/fieldparams"

# Run coverage tests (norace, matching the old bazel --features=norace).
go test -tags=develop -covermode=atomic -coverprofile=/tmp/cover-mainnet.out ${MAINNET_PKGS}
go test -tags=develop,minimal -covermode=atomic -coverprofile=/tmp/cover-minimal.out ${MINIMAL_PKGS}

# Merge the two profiles into one (for deepsource + codecov).
go run ./tools/gocovmerge /tmp/cover-mainnet.out /tmp/cover-minimal.out > /tmp/cover.out

# Download deepsource CLI
curl https://deepsource.io/cli | sh

# Upload to deepsource (requires DEEPSOURCE_DSN environment variable)
./bin/deepsource report --analyzer test-coverage --key go --value-file /tmp/cover.out

# Provide permission to execute script.
chmod +x ./hack/codecov.sh

# Upload to codecov (requires CODECOV_TOKEN environment variable)
./hack/codecov.sh -f /tmp/cover.out
