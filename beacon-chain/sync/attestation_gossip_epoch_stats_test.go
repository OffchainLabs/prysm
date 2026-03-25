package sync

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/stretchr/testify/require"
)

func TestCommitteeAttGossipEpochStats_epochRolloverOnObserve(t *testing.T) {
	genesis := time.Unix(1_700_000_000, 0)
	var slot primitives.Slot
	cl := startup.NewClock(genesis, [32]byte{}, startup.WithNower(func() time.Time {
		tt, err := slots.StartTime(genesis, slot)
		require.NoError(t, err)
		return tt
	}))

	var st committeeAttGossipEpochStats
	st.observe(cl, pubsub.ValidationAccept, "")

	st.mu.Lock()
	require.Equal(t, slots.ToEpoch(0), st.epoch)
	require.Equal(t, uint64(1), st.success)
	require.Empty(t, st.nonSuccess)
	st.mu.Unlock()

	slot = params.BeaconConfig().SlotsPerEpoch
	st.observe(cl, pubsub.ValidationReject, "reject_test")

	st.mu.Lock()
	require.Equal(t, slots.ToEpoch(slot), st.epoch)
	require.Equal(t, uint64(0), st.success)
	require.Equal(t, uint64(1), st.nonSuccess["reject_test"])
	st.mu.Unlock()
}

func TestCommitteeAttGossipEpochStats_rotateOnlyFlushesGap(t *testing.T) {
	genesis := time.Unix(1_700_000_000, 0)
	var slot primitives.Slot
	cl := startup.NewClock(genesis, [32]byte{}, startup.WithNower(func() time.Time {
		tt, err := slots.StartTime(genesis, slot)
		require.NoError(t, err)
		return tt
	}))

	var st committeeAttGossipEpochStats
	st.observe(cl, pubsub.ValidationAccept, "")
	st.mu.Lock()
	require.Equal(t, primitives.Epoch(0), st.epoch)
	st.mu.Unlock()

	slot = 2 * params.BeaconConfig().SlotsPerEpoch
	st.rotateOnly(cl)

	st.mu.Lock()
	require.Equal(t, slots.ToEpoch(slot), st.epoch)
	require.Zero(t, st.success)
	require.Empty(t, st.nonSuccess)
	st.mu.Unlock()
}
