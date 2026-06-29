package endtoend

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/client/beacon"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/kurtosis"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

const (
	// LATE_SYNC_NODE_DELAY is how long after genesis the skip_start sync nodes are
	// started, so the chain has advanced and finalized: the normal-sync node then
	// has history to catch up over P2P, and the checkpoint-sync node has a
	// finalized checkpoint to sync from.
	LATE_SYNC_NODE_DELAY = 6 * time.Minute

	// SYNC_NODE_SERVICE is the skip_start node for the P2P (genesis) sync test.
	SYNC_NODE_SERVICE = "cl-3-prysm-geth"

	// CHECKPOINT_SYNC_NODE_SERVICE is the skip_start node for the checkpoint sync test.
	CHECKPOINT_SYNC_NODE_SERVICE = "cl-4-prysm-geth"
)

// TestEndToEnd_Kurtosis_MinimalConfig mirrors TestEndToEnd_MinimalConfig, but runs the test in a Kurtosis enclave instead of locally.
func TestEndToEnd_Kurtosis_MinimalConfig(t *testing.T) {
	ctx := t.Context()

	// Prerequisite for Kurtosis: Load images needed.
	LoadPrysmDockerImages(t)

	tests := []struct {
		enclaveName string
		configPath  string
		epochsToRun uint64
	}{
		{
			enclaveName: "minimal",
			configPath:  "testing/endtoend/network-config/minimal.yaml",
			epochsToRun: 15,
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
				tt.configPath,
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
			secondsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
			deadline := genesisTime.Add(time.Duration(tt.epochsToRun*secondsPerEpoch) * time.Second)

			require.NoError(t, kw.RegisterPlaybooks(ctx), "Failed to register Assertoor playbooks")

			// Resume late-joining beacon node for normal sync and checkpoint sync test.
			stoppedNodes, err := kw.StoppedPrysmCLName()
			require.NoError(t, err, "Failed to locate the skip_start sync node")
			require.Equal(t, 2, len(stoppedNodes))
			require.Equal(t, true, slices.Contains(stoppedNodes, SYNC_NODE_SERVICE), "Expected stopped nodes to contain %s", SYNC_NODE_SERVICE)
			require.Equal(t, true, slices.Contains(stoppedNodes, CHECKPOINT_SYNC_NODE_SERVICE), "Expected stopped nodes to contain %s", CHECKPOINT_SYNC_NODE_SERVICE)

			delay := time.Until(genesisTime.Add(LATE_SYNC_NODE_DELAY))
			scheduleLateSyncNodeStart(t, ctx, kw, delay, SYNC_NODE_SERVICE, CHECKPOINT_SYNC_NODE_SERVICE)

			require.NoError(t, kw.WaitForAssertoor(ctx, deadline), "Assertoor checks failed")
		})
	}
}

// scheduleLateSyncNodeStart starts the given skip_start beacon nodes after delay.
func scheduleLateSyncNodeStart(t *testing.T, ctx context.Context, kw *kurtosis.KurtosisWrapper, delay time.Duration, names ...string) {
	t.Logf("Will start late sync nodes %v after %s", names, delay)

	done := make(chan error, len(names))
	go func() {
		select {
		case <-ctx.Done():
			return // run ended before the nodes were due to start
		case <-time.After(delay):
		}
		for _, name := range names {
			t.Logf("Starting late sync node %q", name)
			done <- kw.StartService(name)
		}
	}()

	t.Cleanup(func() {
		// Non-blocking: report any start that actually ran and failed.
		for range names {
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("Failed to start late sync node: %v", err)
				}
			default:
			}
		}
	})
}
