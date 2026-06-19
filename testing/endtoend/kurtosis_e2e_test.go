package endtoend

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/client/beacon"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	ev "github.com/OffchainLabs/prysm/v7/testing/endtoend/evaluators"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/helpers"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/kurtosis"
	e2etypes "github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/bazelbuild/rules_go/go/tools/bazel"
)

const (
	// ETHEREUM_PACKAGE is the identifier of the ethereum-package Starlark package used in these tests.
	ETHEREUM_PACKAGE      = "github.com/ethpandaops/ethereum-package"
	MINIMAL_EPOCHS_TO_RUN = 6 // enough to observe finalization
)

func TestEndToEnd_Kurtosis(t *testing.T) {
	ctx := t.Context()

	// Prerequisite for Kurtosis: Load images needed.
	LoadPrysmDockerImages(t)

	tests := []struct {
		enclaveName string
		configPath  string
		evaluators  []e2etypes.Evaluator
	}{
		{
			enclaveName: "minimal",
			configPath:  "testing/endtoend/network-config/default.yaml",
			evaluators: []e2etypes.Evaluator{
				ev.PeersCheck,
				ev.FinishedSyncing,
				ev.AllNodesHaveSameHead,
				ev.FinalizationOccurs(3),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.enclaveName, func(t *testing.T) {
			// Note: Subtests can be run in parallel as they use separate enclaves.
			t.Parallel()

			kw, err := kurtosis.NewKurtosisWrapper(t, ctx, tt.enclaveName)
			require.NoError(t, err, "Failed to create Kurtosis wrapper")

			require.NoError(t, kw.CreateEnclave(), "Failed to create Kurtosis enclave")
			t.Cleanup(func() {
				if err := kw.DestroyEnclave(); err != nil {
					t.Logf("Failed to cleanup enclave: %v", err)
				}
			})

			require.NoError(t, kw.RunPackageWithNetworkConfig(
				ETHEREUM_PACKAGE,
				tt.configPath,
			), "Failed to run ethereum package")

			// conns are used by evaluators (gRPC).
			conns, closeConns, err := kw.NewGRPCConnections()
			require.NoError(t, err, "Failed to dial Prysm beacon gRPC")
			t.Cleanup(closeConns)

			restURLs, err := kw.NewBeaconRESTEndpoints()
			require.NoError(t, err, "Failed to resolve beacon REST endpoints")

			// Create a beacon API client to
			// 1. Fetch genesis information.
			// 2. Fetch config spec for hydrating params.
			client, err := beacon.NewClient(restURLs[0])
			require.NoError(t, err, "Failed to create beacon API client")

			// Hydrate params with the config the enclave is actually running, so
			// evaluators compute expectations against the real network config.
			cfg := fetchConfig(t, ctx, client)
			params.SetActiveTestCleanup(t, cfg)

			// Fetch genesis time and set up an epoch ticker to drive epoch-based evaluations.
			genesisTime := fetchGenesisTime(t, ctx, client)
			secondsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
			ticker := helpers.NewEpochTicker(helpers.EpochTickerStartTime(genesisTime), secondsPerEpoch)

			// TODO: NewEvaluationContext receives deposit balancer.
			ec := e2etypes.NewEvaluationContext(nil)

			// In every epoch, run all evaluators whose policy matches the current epoch.
			for currentEpoch := range ticker.C() {
				var wg sync.WaitGroup

				for _, eval := range tt.evaluators {
					if !eval.Policy(primitives.Epoch(currentEpoch)) {
						continue
					}
					wg.Go(func() {
						t.Run(fmt.Sprintf(eval.Name, currentEpoch), func(t *testing.T) {
							err := eval.Evaluation(ec, conns...)
							assert.NoError(t, err, "Evaluation failed for epoch %d: %v", currentEpoch, err)
						})
					})
				}

				wg.Wait()

				// Notify the ticker when test has failed or we've reached the desired number of epochs.
				if t.Failed() || currentEpoch >= MINIMAL_EPOCHS_TO_RUN-1 {
					ticker.Done()
					break
				}
			}
		})
	}
}

// fetchConfig fetches the chain config the enclave is actually running.
func fetchConfig(t *testing.T, ctx context.Context, client *beacon.Client) *params.BeaconChainConfig {
	// Poll the spec endpoint until the node serves it (readiness gate).
	var specData any
	var err error
	for range 30 {
		spec, e := client.GetConfigSpec(ctx)
		if e == nil {
			specData, err = spec.Data, nil
			break
		}
		err = e
		time.Sleep(2 * time.Second)
	}
	require.NoError(t, err, "Failed to fetch config spec")

	data, ok := specData.(map[string]any)
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

// fetchGenesisTime returns the network's genesis time.
// polled it), so a single request suffices.
func fetchGenesisTime(t *testing.T, ctx context.Context, client *beacon.Client) time.Time {
	genesis, err := client.GetGenesis(ctx)
	require.NoError(t, err, "Failed to get genesis")

	secs, err := strconv.ParseInt(genesis.GenesisTime, 10, 64)
	require.NoError(t, err, "Failed to parse genesis time")

	return time.Unix(secs, 0)
}

const (
	BEACON_CHAIN_IMAGE_TARGET = "cmd/beacon-chain/oci_image_tarball_e2e/tarball.tar"
	VALIDATOR_IMAGE_TARGET    = "cmd/validator/oci_image_tarball_e2e/tarball.tar"

	BEACON_CHAIN_IMAGE_NAME = "gcr.io/offchainlabs/prysm/beacon-chain:latest"
	VALIDATOR_IMAGE_NAME    = "gcr.io/offchainlabs/prysm/validator:latest"
)

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
