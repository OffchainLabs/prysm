package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// TestGloasMissingPayload verifies that a Gloas cluster continues to finalize
// when one validator's execution payload envelopes are always dropped (builder
// never delivers). The remaining validators should produce full payloads and
// the chain should make progress despite the empty-payload slots.
func TestGloasMissingPayload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := DefaultConfig()
	cfg.NumBeaconNodes = 3
	cfg.NumGethNodes = 3
	cfg.Timeout = 5 * time.Minute
	h := NewHarness(t, cfg)

	t.Log("Building binaries...")
	h.buildBinaries()

	t.Log("Generating genesis...")
	h.generateJWTSecret()
	h.generateGenesis()

	t.Log("Starting geth nodes...")
	for i := range cfg.NumGethNodes {
		h.startGeth(i)
	}
	time.Sleep(2 * time.Second)

	t.Log("Starting beacon nodes...")
	h.startBeacon(0)
	enr := h.waitForENR(0)
	for i := 1; i < cfg.NumBeaconNodes; i++ {
		h.startBeacon(i, enr)
	}
	t.Log("Waiting for beacon nodes to be ready...")
	for i := range cfg.NumBeaconNodes {
		h.waitForBeaconAPI(i)
	}

	// Set up interceptor for validator-0: drop all envelopes.
	proxyPort := interceptorProxyPort(0)
	targetAddr := fmt.Sprintf("127.0.0.1:%d", beaconRPCPort(0))
	proxy := NewValidatorInterceptor(t, proxyPort, targetAddr)
	require.NoError(t, proxy.Start())

	// Drop envelopes for every slot validator-0 might propose.
	slotsPerEpoch := uint64(fieldparams.SlotsPerEpoch)
	maxSlots := 10 * slotsPerEpoch
	for s := range maxSlots {
		proxy.SetRule(primitives.Slot(s), &SlotRule{DropEnvelope: true})
	}

	t.Log("Starting validators...")
	// Validator-0 connects through the interceptor proxy.
	h.startValidatorWithRPC(0, proxyPort)
	// Validators 1 and 2 connect directly to their beacon nodes.
	for i := 1; i < cfg.NumBeaconNodes; i++ {
		h.startValidator(i)
	}

	t.Cleanup(func() {
		// Stop validators first so they don't hit a dead proxy.
		h.stopValidators()
		proxy.Stop()
		h.stopBeaconsAndGeths()
		h.checkLogsForProblems()
		if t.Failed() {
			h.dumpLogs()
		}
	})

	t.Log("Cluster started. Validator-0 envelopes are being dropped.")

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	t.Logf("SlotsPerEpoch=%d, SecondsPerSlot=%d — waiting for finality at epoch >= 3", slotsPerEpoch, cfg.SecondsPerSlot)

	tracker := newChainTracker(t, slotsPerEpoch)
	for i := range cfg.NumBeaconNodes {
		tracker.track(ctx, i)
	}

	// Chain should still finalize despite missing envelopes from validator-0.
	waitFor(t, ctx, 2*time.Second, "finalized epoch >= 3", func() bool {
		return tracker.getFinalized(0) >= 3
	})

	finalizedEpoch := tracker.getFinalized(0)
	reorgCount := tracker.getReorgCount(0)

	// All beacon nodes should have blocks for the finalized range.
	totalSlots := finalizedEpoch * slotsPerEpoch
	for i := range cfg.NumBeaconNodes {
		missing := verifyAllBlocks(ctx, t, i, totalSlots)
		if len(missing) > 0 {
			t.Errorf("beacon-%d missing blocks at slots: %v", i, missing)
		}
	}

	t.Logf("\nFinal chain state:\n%s", tracker.render())
	t.Logf("PASS: finalized epoch %d, %d blocks, %d reorgs — chain survived missing envelopes",
		finalizedEpoch, tracker.blockCount(0), reorgCount)
}
