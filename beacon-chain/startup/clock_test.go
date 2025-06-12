package startup

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestClock(t *testing.T) {
	vr := [32]byte{}
	cases := []struct {
		name   string
		nSlots primitives.Slot
	}{
		{
			name:   "3 slots",
			nSlots: 3,
		},
		{
			name:   "0 slots",
			nSlots: 0,
		},
		{
			name:   "1 epoch",
			nSlots: params.BeaconConfig().SlotsPerEpoch,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Skip("TODO(preston): Consider adding support for alternative 'now' in SlotTimeSchedule")
			genesis, now := testInterval(t, c.nSlots)
			nower := func() time.Time { return now }
			cl := NewClock(genesis, vr, WithNower(nower))
			require.Equal(t, genesis, cl.GenesisTime())
			require.Equal(t, now, cl.Now())
			require.Equal(t, c.nSlots, cl.CurrentSlot())
		})
	}
}

func testInterval(t *testing.T, nSlots primitives.Slot) (time.Time, time.Time) {
	sg, err := params.BeaconConfig().SlotSchedule.SinceGenesis(nSlots)
	require.NoError(t, err)
	startTime := time.Now()
	return startTime, startTime.Add(sg)
}
