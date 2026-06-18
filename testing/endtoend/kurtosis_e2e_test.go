package endtoend

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	ev "github.com/OffchainLabs/prysm/v7/testing/endtoend/evaluators"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/helpers"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/kurtosis"
	e2etypes "github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	// ETHEREUM_PACKAGE is the identifier of the ethereum-package Starlark package used in these tests.
	ETHEREUM_PACKAGE      = "github.com/ethpandaops/ethereum-package"
	MINIMAL_EPOCHS_TO_RUN = 6 // enough to observe finalization
)

func TestEndToEnd_Kurtosis(t *testing.T) {
	ctx := t.Context()

	params.SetActiveTestCleanup(t, params.MinimalSpecConfig())

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
				ev.FinishedSyncing,
				ev.AllNodesHaveSameHead,
				ev.FinalizationOccurs(3),
			},
		},

		{
			enclaveName: "minimal-with-peers-check",
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

			conns, closeConns, err := kw.NewGRPCConnections()
			require.NoError(t, err, "Failed to dial Prysm beacon gRPC")
			t.Cleanup(closeConns)

			// Create a slot ticker starting from genesis so that evaluators can use it as a trigger.
			secondsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
			genesis := waitForGenesis(t, ctx, conns[0])
			ticker := helpers.NewEpochTicker(helpers.EpochTickerStartTime(genesis), secondsPerEpoch)

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

// waitForGenesis polls a beacon node's gRPC until it reports genesis, tolerating
// the brief window where the RPC server is still coming up after the enclave starts.
func waitForGenesis(t *testing.T, ctx context.Context, conn *grpc.ClientConn) *eth.Genesis {
	client := eth.NewNodeClient(conn)
	var genesis *eth.Genesis
	var err error
	for range 30 {
		genesis, err = client.GetGenesis(ctx, &emptypb.Empty{})
		if err == nil {
			return genesis
		}
		time.Sleep(2 * time.Second)
	}
	require.NoError(t, err, "Failed to get genesis from beacon node gRPC")
	return genesis
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
