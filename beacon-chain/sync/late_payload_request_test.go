package sync

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/async/abool"
	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/stretchr/testify/require"
)

func TestRequestLatePayload_Guards(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	var headRoot [32]byte
	headRoot[0] = 0x42

	tests := []struct {
		name    string
		started bool
		syncing bool
		slot    primitives.Slot
	}{
		{name: "chain not started", started: false, syncing: false, slot: 1},
		{name: "still syncing", started: true, syncing: true, slot: 1},
		{name: "slot is not head slot", started: true, syncing: false, slot: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headSlot := primitives.Slot(1)
			// p2p is nil — if a guard fails to short-circuit and we reach
			// requestPayloadEnvelope's peer lookup, the test would panic.
			s := &Service{
				cfg: &config{
					chain: &mock.ChainService{
						Root:         headRoot[:],
						MockHeadSlot: &headSlot,
					},
					initialSync: &mockSync.Sync{IsSyncing: tt.syncing},
				},
				chainStarted:    abool.New(),
				badPayloadCache: lruwrpr.New(10),
			}
			if tt.started {
				s.chainStarted.Set()
			}
			require.NotPanics(t, func() { s.requestLatePayload(tt.slot) })
		})
	}
}
