package endtoend

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/client/beacon"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/bazelbuild/rules_go/go/tools/bazel"
)

const (
	// ETHEREUM_PACKAGE is the identifier of the ethereum-package Starlark package used in these tests.
	ETHEREUM_PACKAGE = "github.com/ethpandaops/ethereum-package"

	BEACON_CHAIN_IMAGE_TARGET = "cmd/beacon-chain/oci_image_tarball_e2e/tarball.tar"
	VALIDATOR_IMAGE_TARGET    = "cmd/validator/oci_image_tarball_e2e/tarball.tar"

	BEACON_CHAIN_IMAGE_NAME = "gcr.io/offchainlabs/prysm/beacon-chain:latest"
	VALIDATOR_IMAGE_NAME    = "gcr.io/offchainlabs/prysm/validator:latest"
)

// waitForNodeReady blocks until the beacon node reports healthy (200 from
// /eth/v1/node/health) or ctx is done.
func waitForNodeReady(t *testing.T, ctx context.Context, client *beacon.Client) {
	var err error
	for range 30 {
		if _, err = client.Get(ctx, "/eth/v1/node/health"); err == nil {
			return
		}
		time.Sleep(2 * time.Second)
	}
	require.NoError(t, err, "Beacon node never became healthy")
}

// fetchConfig fetches the chain config the enclave is actually running.
func fetchConfig(t *testing.T, ctx context.Context, client *beacon.Client) *params.BeaconChainConfig {
	spec, err := client.GetConfigSpec(ctx)
	require.NoError(t, err, "Failed to fetch config spec")

	data, ok := spec.Data.(map[string]any)
	require.Equal(t, true, ok, "Config spec has unexpected structure")

	var b strings.Builder
	for k, v := range data {
		if s, ok := v.(string); ok {
			fmt.Fprintf(&b, "%s: %s\n", k, s)
		}
	}

	cfg, err := params.UnmarshalConfig([]byte(b.String()), nil)
	require.NoError(t, err, "Failed to parse hydrated config")

	return cfg
}

// fetchGenesisTime returns the network's genesis time. The caller should wait
// for node readiness first, so a single request suffices.
func fetchGenesisTime(t *testing.T, ctx context.Context, client *beacon.Client) time.Time {
	genesis, err := client.GetGenesis(ctx)
	require.NoError(t, err, "Failed to get genesis")

	secs, err := strconv.ParseInt(genesis.GenesisTime, 10, 64)
	require.NoError(t, err, "Failed to parse genesis time")

	return time.Unix(secs, 0)
}

// LoadPrysmDockerImages loads the Prysm beacon-chain and validator Docker images
// into the local Docker daemon with verification.
func LoadPrysmDockerImages(t *testing.T) {
	// Load the beacon-chain image.
	loadDockerImage(t, BEACON_CHAIN_IMAGE_TARGET)
	verifyImageLoaded(t, BEACON_CHAIN_IMAGE_NAME)

	// Load the validator image.
	loadDockerImage(t, VALIDATOR_IMAGE_TARGET)
	verifyImageLoaded(t, VALIDATOR_IMAGE_NAME)
}

// loadDockerImage loads a Docker image from a Bazel runfile path into the local Docker daemon.
func loadDockerImage(t *testing.T, runfilePath string) {
	filePath, err := bazel.Runfile(runfilePath)
	require.NoError(t, err, "Failed to find runfile: %s", runfilePath)

	cmd := exec.Command("docker", "load", "-i", filePath)
	require.NoError(t, cmd.Run(), "Failed to load docker image from file: %s", filePath)
}

// verifyImageLoaded checks if a Docker image with the given name exists in the local Docker daemon.
func verifyImageLoaded(t *testing.T, imageName string) {
	cmd := exec.Command("docker", "image", "inspect", imageName)
	require.NoError(t, cmd.Run(), "Failed to verify image: %s", imageName)
}
