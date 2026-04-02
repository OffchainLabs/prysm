package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
)

// TestGloasSelfBuild verifies that a Gloas cluster can produce blocks
// with self-built execution payloads and reach finality.
func TestGloasSelfBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := DefaultConfig()
	cfg.Timeout = 5 * time.Minute
	h := NewHarness(t, cfg)
	h.Start()

	// Give nodes a moment to start up.
	time.Sleep(5 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	slotsPerEpoch := uint64(fieldparams.SlotsPerEpoch)
	t.Logf("SlotsPerEpoch=%d, SecondsPerSlot=%d — waiting for finality at epoch >= 3", slotsPerEpoch, cfg.SecondsPerSlot)

	// Start tracking blocks on all beacon nodes.
	tracker := newChainTracker(t, slotsPerEpoch)
	for i := 0; i < cfg.NumBeaconNodes; i++ {
		tracker.track(ctx, i)
	}

	// Wait for finalization at epoch >= 3 on beacon-0.
	waitFor(t, ctx, 2*time.Second, "finalized epoch >= 3", func() bool {
		return tracker.getFinalized(0) >= 3
	})
	finalizedEpoch := tracker.getFinalized(0)
	reorgCount := tracker.getReorgCount(0)

	// --- Post-finalization checks ---

	// 1. No reorgs should have occurred.
	if reorgCount > 0 {
		t.Errorf("Expected 0 chain reorgs, got %d", reorgCount)
	}

	// 2. Every slot should have a block on all beacon nodes.
	totalSlots := finalizedEpoch * slotsPerEpoch
	for i := 0; i < cfg.NumBeaconNodes; i++ {
		missing := verifyAllBlocks(ctx, t, i, totalSlots)
		if len(missing) > 0 {
			t.Errorf("beacon-%d missing blocks at slots: %v", i, missing)
		} else {
			t.Logf("beacon-%d has all blocks for slots 1..%d", i, totalSlots)
		}
	}

	// 3. Spin up a new beacon+geth node and verify it syncs to head.
	t.Log("Starting sync node...")
	syncIndex := h.AddSyncNode()
	tracker.track(ctx, syncIndex)

	const maxStallPolls = 30
	var lastSlot uint64
	stallCount := 0
	waitFor(t, ctx, 2*time.Second, "sync node catches up", func() bool {
		slot, err := headSlotHTTP(ctx, syncIndex)
		if err != nil {
			return false
		}
		target, _ := headSlotHTTP(ctx, 0)
		if target > 0 && slot >= target {
			return true
		}
		if slot > lastSlot {
			lastSlot = slot
			stallCount = 0
		} else {
			stallCount++
			if stallCount >= maxStallPolls {
				t.Fatalf("Sync node beacon-%d stalled at slot %d for %d polls (target: %d)",
					syncIndex, slot, stallCount, target)
			}
		}
		return false
	})

	// 4. Verify all nodes agree on the same head slot.
	allNodes := cfg.NumBeaconNodes + 1 // original + sync node
	slots := make([]uint64, allNodes)
	for i := range allNodes {
		s, err := headSlotHTTP(ctx, i)
		if err != nil {
			t.Fatalf("beacon-%d head query failed: %v", i, err)
		}
		slots[i] = s
	}
	allMatch := true
	for i := 1; i < len(slots); i++ {
		if slots[i] != slots[0] {
			allMatch = false
		}
	}
	if !allMatch {
		t.Errorf("Head slot mismatch: %v", slots)
	} else {
		t.Logf("All %d nodes agree on head slot %d", allNodes, slots[0])
	}

	// 5. Deposit a builder and verify it appears in the beacon state.
	depositBuilder(t, ctx, 0, 0)

	// 6. Send a blob TX and verify it's included in a block.
	sendBlobAndVerify(t, ctx, 0, 0)

	t.Logf("\nFinal chain state:\n%s", tracker.render())
	t.Logf("PASS: finalized epoch %d, %d blocks, %d reorgs, sync ok, blobs ok, builder deposited",
		finalizedEpoch, tracker.blockCount(0), reorgCount)
}

// waitFor polls check at interval until it returns true or ctx expires.
func waitFor(t *testing.T, ctx context.Context, interval time.Duration, desc string, check func() bool) {
	t.Helper()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timed out waiting for: %s", desc)
		case <-ticker.C:
			if check() {
				return
			}
		}
	}
}

// headSlotHTTP queries the beacon REST API for the head slot.
func headSlotHTTP(ctx context.Context, beaconIndex int) (uint64, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/eth/v1/beacon/headers/head", beaconGRPCPort(beaconIndex))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var result struct {
		Data struct {
			Header struct {
				Message struct {
					Slot string `json:"slot"`
				} `json:"message"`
			} `json:"header"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}
	return strconv.ParseUint(result.Data.Header.Message.Slot, 10, 64)
}

// verifyAllBlocks checks that every slot in [1, totalSlots] has a block on the
// given beacon node. Retries once for sync propagation.
func verifyAllBlocks(ctx context.Context, t *testing.T, beaconIndex int, totalSlots uint64) []uint64 {
	t.Helper()
	missing := probeSlots(ctx, beaconIndex, 1, totalSlots)
	if len(missing) == 0 {
		return nil
	}
	t.Logf("beacon-%d: %d slots pending sync, retrying in 5s...", beaconIndex, len(missing))
	time.Sleep(5 * time.Second)
	var stillMissing []uint64
	for _, slot := range missing {
		if err := probeBlock(ctx, beaconIndex, slot); err != nil {
			stillMissing = append(stillMissing, slot)
		}
	}
	return stillMissing
}

func probeSlots(ctx context.Context, beaconIndex int, from, to uint64) []uint64 {
	var missing []uint64
	for slot := from; slot <= to; slot++ {
		if err := probeBlock(ctx, beaconIndex, slot); err != nil {
			missing = append(missing, slot)
		}
	}
	return missing
}

func probeBlock(ctx context.Context, beaconIndex int, slot uint64) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/eth/v2/beacon/blocks/%d", beaconGRPCPort(beaconIndex), slot)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slot %d: HTTP %d", slot, resp.StatusCode)
	}
	return nil
}
