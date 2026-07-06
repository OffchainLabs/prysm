package client

import (
	"encoding/json"
	"strings"
	"testing"

	eventClient "github.com/OffchainLabs/prysm/v7/api/client/event"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/async/event"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func headValidator() *validator {
	return &validator{
		head:                 newHeadTracker(),
		slotFeed:             &event.Feed{},
		disableDutiesPolling: true, // skip checkDependentRoots (needs duties/clients)
	}
}

func TestProcessEvent_HeadRecordsRoot(t *testing.T) {
	v := headValidator()
	root := "0x" + strings.Repeat("ab", 32)
	data, err := json.Marshal(&structs.HeadEvent{Slot: "42", Block: root})
	require.NoError(t, err)

	v.ProcessEvent(t.Context(), &eventClient.Event{Type: eventClient.EventHead, Data: data})

	gotRoot, gotSlot, ok := latestHead(v.head)
	require.Equal(t, true, ok)
	require.Equal(t, uint64(42), uint64(gotSlot))
	require.Equal(t, byte(0xab), gotRoot[0])
}

func TestProcessEvent_HeadMalformedRootStillUpdatesSlot(t *testing.T) {
	v := headValidator()
	data, err := json.Marshal(&structs.HeadEvent{Slot: "42", Block: "0xnothex"})
	require.NoError(t, err)

	v.ProcessEvent(t.Context(), &eventClient.Event{Type: eventClient.EventHead, Data: data})

	// Head root not recorded because the block root failed to decode...
	_, _, ok := latestHead(v.head)
	require.Equal(t, false, ok)
	// ...but the highest slot update must not have been dropped.
	require.Equal(t, uint64(42), uint64(v.highestSlot()))
}
