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
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/kurtosis"
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

	// Health check waits maximum 1 minute (30 retries * 2 seconds interval)
	MAX_HEALTH_CHECK_RETRY = 30
	HEALTH_CHECK_INTERVAL  = 2 * time.Second
)

// KurtosisTestSuites defines a suite of end-to-end tests to run against a Kurtosis enclave.
type KurtosisTestSuites struct {
	enclaveName string
	configPath  string
	epochsToRun uint64
}

func (k *KurtosisTestSuites) Run(t *testing.T) {
	// Note: Subtests can be run in parallel as they use separate enclaves.
	t.Parallel()

	ctx := t.Context()

	kw, err := kurtosis.NewKurtosisWrapper(t, ctx, k.enclaveName)
	require.NoError(t, err, "Failed to create Kurtosis wrapper")

	require.NoError(t, kw.CreateEnclave(), "Failed to create Kurtosis enclave")
	t.Cleanup(func() {
		if t.Failed() {
			// Dump logs so that we can see what went wrong before the enclave is destroyed.
			kw.DumpFailedAssertoorLogs()
		}
		if err := kw.DestroyEnclave(); err != nil {
			t.Logf("Failed to cleanup enclave: %v", err)
		}
	})

	require.NoError(t, kw.RunPackageWithNetworkConfig(
		ETHEREUM_PACKAGE,
		k.configPath,
	), "Failed to run ethereum package")

	restURLs, err := kw.NewBeaconRESTEndpoints()
	require.NoError(t, err, "Failed to resolve beacon REST endpoints")

	// Create a beacon API client to
	// 1. Fetch genesis information.
	// 2. Fetch config spec for hydrating params.
	client, err := beacon.NewClient(restURLs[0])
	require.NoError(t, err, "Failed to create beacon API client")

	// Gate on node readiness once, then every API call below is a single request.
	waitForNodeReady(t, ctx, client)

	// Hydrate params with the config the enclave is actually running, so
	// the timeout below is computed against the real network config.
	cfg := fetchConfig(t, ctx, client)
	params.SetActiveTestCleanup(t, cfg)

	// Set deadline for assertoor.
	genesisTime := fetchGenesisTime(t, ctx, client)
	deadline := genesisTime.Add(params.EpochsDuration(primitives.Epoch(k.epochsToRun), params.BeaconConfig()))

	// Register Assertoor playbooks and wait for them to complete.
	require.NoError(t, kw.RegisterPlaybooks(ctx), "Failed to register Assertoor playbooks")
	require.NoError(t, kw.WaitForAssertoor(ctx, deadline), "Assertoor checks failed")
}

// waitForNodeReady blocks until the beacon node reports healthy (200 from
// /eth/v1/node/health) or ctx is done.
func waitForNodeReady(t *testing.T, ctx context.Context, client *beacon.Client) {
	var err error
	for range MAX_HEALTH_CHECK_RETRY {
		if _, err = client.Get(ctx, "/eth/v1/node/health"); err == nil {
			return
		}
		time.Sleep(HEALTH_CHECK_INTERVAL)
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

	cmd := exec.Command("docker", "load", "-i", filePath) // #nosec G204
	require.NoError(t, cmd.Run(), "Failed to load docker image from file: %s", filePath)
}

// verifyImageLoaded checks if a Docker image with the given name exists in the local Docker daemon.
func verifyImageLoaded(t *testing.T, imageName string) {
	cmd := exec.Command("docker", "image", "inspect", imageName)
	require.NoError(t, cmd.Run(), "Failed to verify image: %s", imageName)
}
