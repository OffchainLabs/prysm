package endtoend

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/client/beacon"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/kurtosis"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"golang.org/x/sync/errgroup"
)

const (
	// ETHEREUM_PACKAGE is the identifier of the ethereum-package Starlark package used in these tests.
	ETHEREUM_PACKAGE = "github.com/ethpandaops/ethereum-package"

	// DEFAULT_LATE_SYNC_NODE_DELAY is how long after genesis the skip_start sync nodes are
	// started, so the chain has advanced and finalized.
	DEFAULT_LATE_SYNC_NODE_DELAY = 6 * time.Minute

	// SYNC_NODE_SERVICE is the skip_start node for the P2P (genesis) sync test.
	SYNC_NODE_SERVICE = "cl-3-prysm-geth"

	// CHECKPOINT_SYNC_NODE_SERVICE is the skip_start node for the checkpoint sync test.
	CHECKPOINT_SYNC_NODE_SERVICE = "cl-4-prysm-geth"

	BEACON_CHAIN_IMAGE_TARGET = "cmd/beacon-chain/oci_image_tarball_e2e/tarball.tar"
	VALIDATOR_IMAGE_TARGET    = "cmd/validator/oci_image_tarball_e2e/tarball.tar"
	FAULTPROXY_IMAGE_TARGET   = "testing/middleware/engine-api-proxy/cmd/faultproxy/oci_image_tarball_e2e/tarball.tar"

	BEACON_CHAIN_IMAGE_NAME = "gcr.io/offchainlabs/prysm/beacon-chain:latest"
	VALIDATOR_IMAGE_NAME    = "gcr.io/offchainlabs/prysm/validator:latest"
	FAULTPROXY_IMAGE_NAME   = "prysm-faultproxy:local"
)

type KurtosisTestSuites struct {
	enclaveName       string
	configPath        string
	epochsToRun       uint64
	runSyncTest       bool
	lateSyncNodeDelay time.Duration
	extraPlaybooks    []string
	skipPlaybooks     []string
	serviceEvents     []kurtosis.EpochServiceEvent
	assertoorEvents   []kurtosis.AssertoorEvent
}

func (k *KurtosisTestSuites) Run(t *testing.T) {
	// Note: Subtests can be run in parallel as they use separate enclaves.
	t.Parallel()

	if k.runSyncTest && k.lateSyncNodeDelay <= 0 {
		k.lateSyncNodeDelay = DEFAULT_LATE_SYNC_NODE_DELAY
	}

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
	secondsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	deadline := genesisTime.Add(time.Duration(k.epochsToRun*secondsPerEpoch) * time.Second)

	require.NoError(t, kw.RegisterPlaybooks(ctx, k.extraPlaybooks, k.skipPlaybooks), "Failed to register Assertoor playbooks")

	k.scheduleServiceEvents(t, kw, genesisTime, secondsPerEpoch)

	require.NoError(t, k.runAssertoorChecks(t, ctx, kw, genesisTime, secondsPerEpoch, deadline), "Assertoor checks failed")
}

func (k *KurtosisTestSuites) scheduleServiceEvents(t *testing.T, kw *kurtosis.KurtosisWrapper, genesisTime time.Time, secondsPerEpoch uint64) {
	if k.runSyncTest {
		// Resume late-joining beacon node for normal sync and checkpoint sync test.
		stoppedNodes, err := kw.StoppedPrysmCLName()
		require.NoError(t, err, "Failed to locate the skip_start sync node")
		require.Equal(t, 2, len(stoppedNodes))
		require.Equal(t, true, slices.Contains(stoppedNodes, SYNC_NODE_SERVICE), "Expected stopped nodes to contain %s", SYNC_NODE_SERVICE)
		require.Equal(t, true, slices.Contains(stoppedNodes, CHECKPOINT_SYNC_NODE_SERVICE), "Expected stopped nodes to contain %s", CHECKPOINT_SYNC_NODE_SERVICE)

		delay := time.Until(genesisTime.Add(k.lateSyncNodeDelay))
		kw.ScheduleServiceAction(delay, kurtosis.ServiceStart, SYNC_NODE_SERVICE, CHECKPOINT_SYNC_NODE_SERVICE)
	}

	if len(k.serviceEvents) > 0 {
		kw.ScheduleServiceEvents(genesisTime, secondsPerEpoch, k.serviceEvents...)
	}
}

// runAssertoorChecks runs the steady-state monitors and every one-shot concurrently,
// reporting each run as it finishes; the first failure cancels the rest.
func (k *KurtosisTestSuites) runAssertoorChecks(t *testing.T, ctx context.Context, kw *kurtosis.KurtosisWrapper, genesisTime time.Time, secondsPerEpoch uint64, deadline time.Time) error {
	events := kurtosis.SortedAssertoorEvents(k.assertoorEvents)

	// Register each one-shot playbook once, up front.
	playbookIDs := make(map[string]string)
	for _, event := range events {
		if _, ok := playbookIDs[event.Playbook]; ok {
			continue
		}
		testID, err := kw.RegisterPlaybook(ctx, event.Playbook)
		if err != nil {
			return fmt.Errorf("register Assertoor playbook %s: %w", event.Playbook, err)
		}
		playbookIDs[event.Playbook] = testID
	}

	g, gctx := errgroup.WithContext(ctx)

	// 1. Steady-state monitors.
	g.Go(func() error {
		if err := kw.WaitForAssertoor(gctx, deadline); err != nil {
			return fmt.Errorf("Assertoor steady-state checks: %w", err)
		}
		return nil
	})

	// A late one-shot needs a few extra epochs to settle, so give each run its own deadline.
	settle := time.Duration(5*secondsPerEpoch) * time.Second

	// 2. One-shot events. Dedicate a goroutine to each one.
	for _, event := range events {
		event := event
		testID := playbookIDs[event.Playbook]
		g.Go(func() error {
			runAt := genesisTime.Add(time.Duration(event.Epoch*secondsPerEpoch) * time.Second)
			select {
			case <-gctx.Done():
				return gctx.Err()
			case <-time.After(time.Until(runAt)):
			}

			config := map[string]any{"targetEpoch": event.Epoch}
			for k, v := range event.Config {
				config[k] = v
			}
			runID, err := kw.ScheduleAssertoorTest(gctx, testID, config)
			if err != nil {
				return fmt.Errorf("schedule Assertoor playbook %s at epoch %d: %w", event.Playbook, event.Epoch, err)
			}
			t.Logf("Scheduled Assertoor playbook %s at epoch %d (run %d)", event.Playbook, event.Epoch, runID)

			runDeadline := runAt.Add(settle)
			if runDeadline.Before(deadline) {
				runDeadline = deadline
			}
			if err := kw.WaitForAssertoorRunIDs(gctx, runDeadline, runID); err != nil {
				return fmt.Errorf("Assertoor playbook %s at epoch %d (run %d): %w", event.Playbook, event.Epoch, runID, err)
			}
			t.Logf("PASSED Assertoor playbook %s at epoch %d (run %d)", event.Playbook, event.Epoch, runID)
			return nil
		})
	}

	return g.Wait()
}

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

// LoadFaultproxyImage loads the faultproxy snooper drop-in image into the local
// Docker daemon. Only the optimistic-sync test needs it.
func LoadFaultproxyImage(t *testing.T) {
	loadDockerImage(t, FAULTPROXY_IMAGE_TARGET)
	verifyImageLoaded(t, FAULTPROXY_IMAGE_NAME)
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
