package endtoend

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/client/beacon"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/kurtosis"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

const (
	DOPPELGANGER_NETWORK_CONFIG = "testing/endtoend/network-config/doppelganger.yaml"

	// DOPPELGANGER_VALIDATOR_SERVICE is added at runtime with cl-1's keys, but
	// points to cl-2 so Prysm can detect the duplicate validator activity.
	DOPPELGANGER_VALIDATOR_SERVICE = "vc-doppelganger-prysm"

	// ETHEREUM_PACKAGE defaults num_validator_keys_per_node to 128, so the
	// first participant's generated keystore artifact is 1-prysm-geth-0-127.
	DOPPELGANGER_KEYSTORE_ARTIFACT = "1-prysm-geth-0-127"

	DOPPELGANGER_BEACON_RPC    = "cl-2-prysm-geth:4000"
	DOPPELGANGER_BEACON_REST   = "http://cl-2-prysm-geth:3500"
	DOPPELGANGER_LOG           = "Duplicate instances exists in the network for validator keys"
	DOPPELGANGER_START_EPOCH   = 4
	DOPPELGANGER_WAITING_EPOCH = 3
)

// TestEndToEnd_Kurtosis_DoppelgangerProtection tests that a doppelganger validator
// is detected when we add a validator with the same keys as an existing validator in the network.
// Note that this test cannot be run with Assertoor as it should stream logs to the console, which Assertoor does not support.
func TestEndToEnd_Kurtosis_DoppelgangerProtection(t *testing.T) {
	ctx := t.Context()

	LoadPrysmDockerImages(t)

	kw, err := kurtosis.NewKurtosisWrapper(t, ctx, "doppelganger")
	require.NoError(t, err, "Failed to create Kurtosis wrapper")

	require.NoError(t, kw.CreateEnclave(), "Failed to create Kurtosis enclave")
	t.Cleanup(func() {
		if err := kw.DestroyEnclave(); err != nil {
			t.Logf("Failed to cleanup enclave: %v", err)
		}
	})

	require.NoError(t, kw.RunPackageWithNetworkConfig(
		ETHEREUM_PACKAGE,
		DOPPELGANGER_NETWORK_CONFIG,
	), "Failed to run ethereum package")

	params.SetActiveTestCleanup(t, params.MinimalSpecConfig())

	restURLs, err := kw.NewBeaconRESTEndpoints()
	require.NoError(t, err, "Failed to resolve beacon REST endpoints")

	client, err := beacon.NewClient(restURLs[0])
	require.NoError(t, err, "Failed to create beacon API client")

	waitForNodeReady(t, ctx, client)

	genesisTime := fetchGenesisTime(t, ctx, client)
	secondsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	epochDuration := time.Duration(secondsPerEpoch) * time.Second
	startAt := genesisTime.Add(DOPPELGANGER_START_EPOCH * epochDuration)
	waitFor := epochDuration * (DOPPELGANGER_WAITING_EPOCH + DOPPELGANGER_START_EPOCH)

	t.Logf("Waiting until epoch %d (%s) to start doppelganger validator", DOPPELGANGER_START_EPOCH, startAt)

	delay := time.Until(startAt)
	if delay <= 0 {
		t.Fatalf("Doppelganger start time %s is in the past", startAt)
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		require.NoError(t, ctx.Err())
	case <-timer.C:
	}

	t.Logf("Adding doppelganger validator at epoch %d (%s)", DOPPELGANGER_START_EPOCH, time.Now())
	require.NoError(t, kw.AddPrysmDoppelgangerValidator(kurtosis.PrysmDoppelgangerValidatorConfig{
		ServiceName:      DOPPELGANGER_VALIDATOR_SERVICE,
		Image:            VALIDATOR_IMAGE_NAME,
		KeystoreArtifact: DOPPELGANGER_KEYSTORE_ARTIFACT,
		BeaconRPC:        DOPPELGANGER_BEACON_RPC,
		BeaconREST:       DOPPELGANGER_BEACON_REST,
	}), "Failed to add doppelganger validator")

	t.Logf("Waiting for doppelganger validator to detect duplicate keys (epoch %d, %s)", DOPPELGANGER_START_EPOCH, time.Now())
	require.NoError(t, kw.WaitForServiceLog(ctx, DOPPELGANGER_VALIDATOR_SERVICE, DOPPELGANGER_LOG, waitFor))
}
