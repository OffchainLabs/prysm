package client

import (
	"encoding/json"
	"testing"
	"time"

	eventClient "github.com/OffchainLabs/prysm/v7/api/client/event"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/event"
)

func TestProcessEvent_AttestationReadySetsHighestSlot(t *testing.T) {
	v := &validator{slotFeed: new(event.Feed)}
	data, err := json.Marshal(&structs.AttestationReadyEvent{Slot: "123", BeaconBlockRoot: "0xabc"})
	require.NoError(t, err)
	v.ProcessEvent(t.Context(), &eventClient.Event{EventType: eventClient.EventAttestationReady, Data: data})
	require.Equal(t, primitives.Slot(123), v.highestSlot())
}

func TestProcessEvent_AttestationReadyBadSlotIgnored(t *testing.T) {
	v := &validator{slotFeed: new(event.Feed)}
	data, err := json.Marshal(&structs.AttestationReadyEvent{Slot: "not-a-number"})
	require.NoError(t, err)
	v.ProcessEvent(t.Context(), &eventClient.Event{EventType: eventClient.EventAttestationReady, Data: data})
	require.Equal(t, primitives.Slot(0), v.highestSlot())
}

func TestProcessEvent_AttestationReadyUnblocksAttestationWait(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{AttestTimely: true})
	defer resetCfg()

	currentSlot := primitives.Slot(4)
	genesisTime := time.Now().Add(-1 * time.Duration(currentSlot.Mul(params.BeaconConfig().SecondsPerSlot)) * time.Second)
	v := &validator{genesisTime: genesisTime, slotFeed: new(event.Feed)}

	start := time.Now()
	go func() {
		time.Sleep(100 * time.Millisecond)
		data, err := json.Marshal(&structs.AttestationReadyEvent{Slot: "4", BeaconBlockRoot: "0xabc"})
		require.NoError(t, err)
		v.ProcessEvent(t.Context(), &eventClient.Event{EventType: eventClient.EventAttestationReady, Data: data})
	}()
	v.waitUntilAttestationDueOrValidBlock(t.Context(), currentSlot)
	require.Equal(t, true, time.Since(start) < 2*time.Second, "wait did not unblock on attestation_ready event")
}
