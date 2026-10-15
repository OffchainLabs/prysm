package startup

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
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
			genesis, now := testInterval(c.nSlots)
			nower := func() time.Time { return now }
			cl := NewClock(genesis, vr, WithNower(nower))
			require.Equal(t, genesis, cl.GenesisTime())
			require.Equal(t, now, cl.Now())
			require.Equal(t, c.nSlots, cl.CurrentSlot())
		})
	}
}

func testInterval(nSlots primitives.Slot) (time.Time, time.Time) {
	oneSlot := time.Second * time.Duration(params.BeaconConfig().SecondsPerSlot)
	var start uint64 = 23
	endOffset := oneSlot * time.Duration(nSlots)
	startTime := time.Unix(int64(start), 0)
	return startTime, startTime.Add(endOffset)
}

func TestClock_WithinSlotRange(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	disparity := params.BeaconConfig().MaximumGossipClockDisparityDuration()
	slotDuration := params.BeaconConfig().SlotDuration()
	genesis := time.Unix(1_000_000, 0)
	slot5 := genesis.Add(5 * slotDuration)
	tests := []struct {
		name        string
		now         time.Time
		first, last primitives.Slot
		want        bool
	}{
		{name: "inside the slot", now: slot5.Add(time.Second), first: 5, last: 5, want: true},
		{name: "early within the disparity", now: slot5.Add(-disparity), first: 5, last: 5, want: true},
		{name: "early past the disparity", now: slot5.Add(-disparity - time.Millisecond), first: 5, last: 5, want: false},
		{name: "late within the disparity", now: slot5.Add(slotDuration + disparity), first: 5, last: 5, want: true},
		{name: "late past the disparity", now: slot5.Add(slotDuration + disparity + time.Millisecond), first: 5, last: 5, want: false},
		{name: "inside a two-slot range", now: slot5.Add(slotDuration + time.Second), first: 5, last: 6, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clock := NewClock(genesis, [32]byte{}, WithTimeAsNow(tc.now))
			got, err := clock.WithinSlotRange(tc.first, tc.last)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
