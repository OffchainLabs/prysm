package slots

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	prysmTime "github.com/OffchainLabs/prysm/v6/time"
)

func TestAbsoluteValueSlotDifference(t *testing.T) {
	type args struct {
		x primitives.Slot
		y primitives.Slot
	}
	tests := []struct {
		name string
		args args
		want uint64
	}{
		{
			name: "x_<_y",
			args: args{
				x: primitives.Slot(3),
				y: primitives.Slot(4),
			},
			want: 1,
		},
		{
			name: "x_>_y",
			args: args{
				x: primitives.Slot(100),
				y: primitives.Slot(4),
			},
			want: 96,
		},
		{
			name: "x_==_y",
			args: args{
				x: primitives.Slot(100),
				y: primitives.Slot(100),
			},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AbsoluteValueSlotDifference(tt.args.x, tt.args.y); got != tt.want {
				t.Errorf("AbsoluteValueSlotDifference() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMultiplySlotBy(t *testing.T) {
	type args struct {
		times int64
	}
	tests := []struct {
		name string
		args args
		want time.Duration
	}{
		{
			name: "multiply by 1",
			args: args{
				times: 1,
			},
			want: time.Duration(12) * time.Second,
		},
		{
			name: "multiply by 2",
			args: args{
				times: 2,
			},
			want: time.Duration(24) * time.Second,
		},
		{
			name: "multiply by 10",
			args: args{
				times: 10,
			},
			want: time.Duration(120) * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MultiplySlotBy(tt.args.times); got != tt.want {
				t.Errorf("MultiplySlotBy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEpochStartSlot_OK(t *testing.T) {
	tests := []struct {
		epoch     primitives.Epoch
		startSlot primitives.Slot
		error     bool
	}{
		{epoch: 0, startSlot: 0 * params.BeaconConfig().SlotsPerEpoch, error: false},
		{epoch: 1, startSlot: 1 * params.BeaconConfig().SlotsPerEpoch, error: false},
		{epoch: 10, startSlot: 10 * params.BeaconConfig().SlotsPerEpoch, error: false},
		{epoch: 1 << 58, startSlot: 1 << 63, error: false},
		{epoch: 1 << 59, startSlot: 1 << 63, error: true},
		{epoch: 1 << 60, startSlot: 1 << 63, error: true},
	}
	for _, tt := range tests {
		ss, err := EpochStart(tt.epoch)
		if !tt.error {
			require.NoError(t, err)
			assert.Equal(t, tt.startSlot, ss, "EpochStart(%d)", tt.epoch)
		} else {
			require.ErrorContains(t, "start slot calculation overflow", err)
		}
	}
}

func TestEpochEndSlot_OK(t *testing.T) {
	tests := []struct {
		epoch     primitives.Epoch
		startSlot primitives.Slot
		error     bool
	}{
		{epoch: 0, startSlot: 1*params.BeaconConfig().SlotsPerEpoch - 1, error: false},
		{epoch: 1, startSlot: 2*params.BeaconConfig().SlotsPerEpoch - 1, error: false},
		{epoch: 10, startSlot: 11*params.BeaconConfig().SlotsPerEpoch - 1, error: false},
		{epoch: 1 << 59, startSlot: 1 << 63, error: true},
		{epoch: 1 << 60, startSlot: 1 << 63, error: true},
		{epoch: math.MaxUint64, startSlot: 0, error: true},
	}
	for _, tt := range tests {
		ss, err := EpochEnd(tt.epoch)
		if !tt.error {
			require.NoError(t, err)
			assert.Equal(t, tt.startSlot, ss, "EpochStart(%d)", tt.epoch)
		} else {
			require.ErrorContains(t, "start slot calculation overflow", err)
		}
	}
}

func TestIsEpochStart(t *testing.T) {
	epochLength := params.BeaconConfig().SlotsPerEpoch

	tests := []struct {
		slot   primitives.Slot
		result bool
	}{
		{
			slot:   epochLength + 1,
			result: false,
		},
		{
			slot:   epochLength - 1,
			result: false,
		},
		{
			slot:   epochLength,
			result: true,
		},
		{
			slot:   epochLength * 2,
			result: true,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.result, IsEpochStart(tt.slot), "IsEpochStart(%d)", tt.slot)
	}
}

func TestIsEpochEnd(t *testing.T) {
	epochLength := params.BeaconConfig().SlotsPerEpoch

	tests := []struct {
		slot   primitives.Slot
		result bool
	}{
		{
			slot:   epochLength + 1,
			result: false,
		},
		{
			slot:   epochLength,
			result: false,
		},
		{
			slot:   epochLength - 1,
			result: true,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.result, IsEpochEnd(tt.slot), "IsEpochEnd(%d)", tt.slot)
	}
}

func TestSlotsSinceEpochStarts(t *testing.T) {
	tests := []struct {
		slots       primitives.Slot
		wantedSlots primitives.Slot
	}{
		{slots: 0, wantedSlots: 0},
		{slots: 1, wantedSlots: 1},
		{slots: params.BeaconConfig().SlotsPerEpoch - 1, wantedSlots: params.BeaconConfig().SlotsPerEpoch - 1},
		{slots: params.BeaconConfig().SlotsPerEpoch + 1, wantedSlots: 1},
		{slots: 10*params.BeaconConfig().SlotsPerEpoch + 2, wantedSlots: 2},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.wantedSlots, SinceEpochStarts(tt.slots))
	}
}

func TestRoundUpToNearestEpoch_OK(t *testing.T) {
	tests := []struct {
		startSlot     primitives.Slot
		roundedUpSlot primitives.Slot
	}{
		{startSlot: 0 * params.BeaconConfig().SlotsPerEpoch, roundedUpSlot: 0},
		{startSlot: 1*params.BeaconConfig().SlotsPerEpoch - 10, roundedUpSlot: 1 * params.BeaconConfig().SlotsPerEpoch},
		{startSlot: 10*params.BeaconConfig().SlotsPerEpoch - (params.BeaconConfig().SlotsPerEpoch - 1), roundedUpSlot: 10 * params.BeaconConfig().SlotsPerEpoch},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.roundedUpSlot, RoundUpToNearestEpoch(tt.startSlot), "RoundUpToNearestEpoch(%d)", tt.startSlot)
	}
}

func TestSlotToTime(t *testing.T) {
	type args struct {
		genesis time.Time
		slot    primitives.Slot
	}
	tests := []struct {
		name      string
		args      args
		want      time.Time
		wantedErr string
	}{
		{
			name: "slot_0",
			args: args{
				genesis: time.Unix(0, 0),
				slot:    0,
			},
			want: time.Unix(0, 0),
		},
		{
			name: "slot_1",
			args: args{
				genesis: time.Unix(0, 0),
				slot:    1,
			},
			want: time.Unix(int64(1*params.BeaconConfig().SecondsPerSlot), 0),
		},
		{
			name: "slot_12",
			args: args{
				genesis: time.Unix(500, 0),
				slot:    12,
			},
			want: time.Unix(500+int64(12*params.BeaconConfig().SecondsPerSlot), 0),
		},
		{
			name: "overflow",
			args: args{
				genesis: time.Unix(500, 0),
				slot:    math.MaxUint64,
			},
			wantedErr: "is in the far distant future",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := StartTime(tt.args.genesis, tt.args.slot)
			if tt.wantedErr != "" {
				assert.ErrorContains(t, tt.wantedErr, err)
			} else {
				assert.NoError(t, err)
				assert.DeepEqual(t, tt.want, got)
			}
		})
	}
}

func TestVerifySlotTime(t *testing.T) {
	type args struct {
		genesisTime   time.Time
		slot          primitives.Slot
		timeTolerance time.Duration
	}
	tests := []struct {
		name      string
		args      args
		wantedErr string
	}{
		{
			name: "Past slot",
			args: args{
				genesisTime: prysmTime.Now().Add(-1 * 5 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second),
				slot:        3,
			},
		},
		{
			name: "within tolerance",
			args: args{
				genesisTime:   prysmTime.Now().Add(-1 * 5 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second).Add(20 * time.Millisecond),
				slot:          5,
				timeTolerance: 20 * time.Millisecond,
			},
		},
		{
			name: "future slot",
			args: args{
				genesisTime: prysmTime.Now().Add(-1 * 5 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second),
				slot:        6,
			},
			wantedErr: "could not process slot from the future",
		},
		{
			name: "future slot but ok given 2s tolerance",
			args: args{
				genesisTime:   prysmTime.Now().Add(-1*time.Duration(params.BeaconConfig().SecondsPerSlot) - 10*time.Second),
				slot:          1,
				timeTolerance: 2 * time.Second,
			},
		},
		{
			name: "max future slot",
			args: args{
				genesisTime: prysmTime.Now().Add(-1 * 5 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second),
				slot:        primitives.Slot(MaxSlotBuffer + 6),
			},
			wantedErr: "exceeds max allowed value relative to the local clock",
		},
		{
			name: "evil future slot",
			args: args{
				genesisTime: prysmTime.Now().Add(-1 * 24 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second), // 24 slots in the past
				// Gets multiplied with slot duration, and results in an overflow. Wraps around to a valid time.
				// Lower than max signed int. And chosen specifically to wrap to a valid slot 24
				slot: primitives.Slot((^uint64(0))/params.BeaconConfig().SecondsPerSlot) + 24,
			},
			wantedErr: "is in the far distant future",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyTime(tt.args.genesisTime, tt.args.slot, tt.args.timeTolerance)
			if tt.wantedErr != "" {
				assert.ErrorContains(t, tt.wantedErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSlotClock_HandlesBadSlot(t *testing.T) {
	genTime := prysmTime.Now().Add(-1 * time.Duration(MaxSlotBuffer) * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)

	assert.NoError(t, ValidateClock(primitives.Slot(MaxSlotBuffer), genTime), "unexpected error validating slot")
	assert.NoError(t, ValidateClock(primitives.Slot(2*MaxSlotBuffer), genTime), "unexpected error validating slot")
	assert.ErrorContains(t, "which exceeds max allowed value relative to the local clock", ValidateClock(primitives.Slot(2*MaxSlotBuffer+1), genTime), "no error from bad slot")
	assert.ErrorContains(t, "which exceeds max allowed value relative to the local clock", ValidateClock(1<<63, genTime), "no error from bad slot")
}

func TestPrevSlot(t *testing.T) {
	tests := []struct {
		name string
		slot primitives.Slot
		want primitives.Slot
	}{
		{
			name: "no underflow",
			slot: 0,
			want: 0,
		},
		{
			name: "slot 1",
			slot: 1,
			want: 0,
		},
		{
			name: "slot 2",
			slot: 2,
			want: 1,
		},
		{
			name: "max",
			slot: math.MaxUint64,
			want: math.MaxUint64 - 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PrevSlot(tt.slot); got != tt.want {
				t.Errorf("PrevSlot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSyncCommitteePeriod(t *testing.T) {
	tests := []struct {
		epoch  primitives.Epoch
		wanted uint64
	}{
		{epoch: 0, wanted: 0},
		{epoch: 0, wanted: 0 / uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod)},
		{epoch: 1, wanted: 1 / uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod)},
		{epoch: 1000, wanted: 1000 / uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod)},
	}
	for _, test := range tests {
		require.Equal(t, test.wanted, SyncCommitteePeriod(test.epoch))
	}
}

func TestSyncCommitteePeriodStartEpoch(t *testing.T) {
	tests := []struct {
		epoch  primitives.Epoch
		wanted primitives.Epoch
	}{
		{epoch: 0, wanted: 0},
		{epoch: params.BeaconConfig().EpochsPerSyncCommitteePeriod + 1, wanted: params.BeaconConfig().EpochsPerSyncCommitteePeriod},
		{epoch: params.BeaconConfig().EpochsPerSyncCommitteePeriod*2 + 100, wanted: params.BeaconConfig().EpochsPerSyncCommitteePeriod * 2},
		{epoch: params.BeaconConfig().EpochsPerSyncCommitteePeriod*params.BeaconConfig().EpochsPerSyncCommitteePeriod + 1, wanted: params.BeaconConfig().EpochsPerSyncCommitteePeriod * params.BeaconConfig().EpochsPerSyncCommitteePeriod},
	}
	for _, test := range tests {
		e, err := SyncCommitteePeriodStartEpoch(test.epoch)
		require.NoError(t, err)
		require.Equal(t, test.wanted, e)
	}
}

func TestSinceSlotStart(t *testing.T) {
	now := time.Now()
	tests := []struct {
		slot      primitives.Slot
		timeStamp time.Time
		wanted    time.Duration
		wantedErr bool
	}{
		{slot: 1, timeStamp: now.Add(-1 * time.Hour), wantedErr: true},
		{slot: 1, timeStamp: now.Add(2 * time.Second).Add(time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second), wanted: 2 * time.Second},
	}
	for i, test := range tests {
		t.Logf("testing scenario %d", i)
		w, err := SinceSlotStart(test.slot, now, test.timeStamp)
		if err != nil {
			require.Equal(t, true, test.wantedErr, "wanted no error, but got %v", err)
		} else {
			require.Equal(t, false, test.wantedErr, "wanted an error, but got %v", err)
			require.Equal(t, w, test.wanted)
		}
	}
}

func TestDuration(t *testing.T) {
	oneSlot := time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	cases := []struct {
		name     string
		start    time.Time
		endDelta time.Duration
		expected primitives.Slot
	}{
		{
			name:     "end before start",
			start:    time.Now(),
			endDelta: -64 * time.Second,
			expected: 0,
		},
		{
			name:     "end equals start",
			start:    time.Now(),
			endDelta: 0,
			expected: 0,
		},
		{
			name:     "one slot apart",
			start:    time.Now(),
			endDelta: oneSlot,
			expected: 1,
		},
		{
			name:     "same slot",
			start:    time.Now(),
			endDelta: time.Second,
			expected: 0,
		},
		{
			name:     "don't round up",
			start:    time.Now(),
			endDelta: oneSlot - time.Second,
			expected: 0,
		},
		{
			name:     "don't round up pt 2",
			start:    time.Now(),
			endDelta: 2*oneSlot - time.Second,
			expected: 1,
		},
		{
			name:     "2 slots",
			start:    time.Now(),
			endDelta: 2 * oneSlot,
			expected: 2,
		},
		{
			name:     "1 epoch",
			start:    time.Now(),
			endDelta: time.Duration(params.BeaconConfig().SlotsPerEpoch) * oneSlot,
			expected: params.BeaconConfig().SlotsPerEpoch,
		},
		{
			name:     "1 epoch and change",
			start:    time.Now(),
			endDelta: oneSlot + time.Second + time.Duration(params.BeaconConfig().SlotsPerEpoch)*oneSlot,
			expected: params.BeaconConfig().SlotsPerEpoch + 1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			end := c.start.Add(c.endDelta)
			a := Duration(c.start, end)
			require.Equal(t, c.expected, a)
		})
	}
}

func TestWithinVotingWindow(t *testing.T) {
	genesisTime := time.Now().Add(-37 * time.Second)
	require.Equal(t, true, WithinVotingWindow(genesisTime, 3))
	genesisTime = time.Now().Add(-40 * time.Second)
	require.Equal(t, false, WithinVotingWindow(genesisTime, 3))
}

func TestSecondsUntilNextEpochStart(t *testing.T) {
	secondsInEpoch := uint64(params.BeaconConfig().SlotsPerEpoch) * params.BeaconConfig().SecondsPerSlot
	// try slot 3
	genesisTime := time.Now().Add(-39 * time.Second)
	waitTime, err := SecondsUntilNextEpochStart(genesisTime)
	require.NoError(t, err)
	require.Equal(t, secondsInEpoch-(params.BeaconConfig().SecondsPerSlot*3)-3, waitTime)
	// try slot 34
	genesisTime = time.Now().Add(time.Duration(-1*int(secondsInEpoch)-int(params.BeaconConfig().SecondsPerSlot*2)-5) * time.Second)
	waitTime, err = SecondsUntilNextEpochStart(genesisTime)
	require.NoError(t, err)
	require.Equal(t, secondsInEpoch-(params.BeaconConfig().SecondsPerSlot*2)-5, waitTime)

	// check if waitTime is correctly EpochStart
	n := time.Now().Add(-39 * time.Second)
	genesisTime = n
	waitTime, err = SecondsUntilNextEpochStart(genesisTime)
	require.NoError(t, err)
	require.Equal(t, secondsInEpoch-39, waitTime)
	newGenesisTime := n.Add(time.Duration(-1*int(waitTime)) * time.Second)
	currentSlot := CurrentSlot(newGenesisTime)
	require.Equal(t, true, IsEpochStart(currentSlot))
}

func TestCurrentSlot(t *testing.T) {
	type args struct {
		genesis time.Time
	}
	tests := []struct {
		name string
		args args
		want primitives.Slot
	}{
		{
			name: "pre-genesis",
			args: args{
				genesis: prysmTime.Now().Add(1 * time.Hour), // 1 hour in the future
			},
			want: 0,
		},
		{
			name: "post-genesis",
			args: args{
				genesis: prysmTime.Now().Add(-5 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second),
			},
			want: 5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CurrentSlot(tt.args.genesis); got != tt.want {
				t.Errorf("CurrentSlot() = %v, want %v", got, tt.want)
			}
		})
	}
}
func TestCurrentSlot_iterative(t *testing.T) {
	for i := primitives.Slot(0); i < 1<<20; i++ {
		testCurrentSlot(t, i)
	}
}

func testCurrentSlot(t testing.TB, slot primitives.Slot) {
	genesis := time.Now().Add(-1 * time.Duration(slot) * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	cs := CurrentSlot(genesis)
	require.Equal(t, slot, cs)
}

func TestToForkVersion(t *testing.T) {
	t.Run("Fulu fork version", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		config := params.BeaconConfig()
		config.FuluForkEpoch = 100
		params.OverrideBeaconConfig(config)

		slot, err := EpochStart(params.BeaconConfig().FuluForkEpoch)
		require.NoError(t, err)

		result := ToForkVersion(slot)
		require.Equal(t, version.Fulu, result)
	})

	t.Run("Electra fork version", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		config := params.BeaconConfig()
		config.ElectraForkEpoch = 100
		params.OverrideBeaconConfig(config)

		slot, err := EpochStart(params.BeaconConfig().ElectraForkEpoch)
		require.NoError(t, err)

		result := ToForkVersion(slot)
		require.Equal(t, version.Electra, result)
	})

	t.Run("Deneb fork version", func(t *testing.T) {
		slot, err := EpochStart(params.BeaconConfig().DenebForkEpoch)
		require.NoError(t, err)

		result := ToForkVersion(slot)
		require.Equal(t, version.Deneb, result)
	})

	t.Run("Capella fork version", func(t *testing.T) {
		slot, err := EpochStart(params.BeaconConfig().CapellaForkEpoch)
		require.NoError(t, err)

		result := ToForkVersion(slot)
		require.Equal(t, version.Capella, result)
	})

	t.Run("Bellatrix fork version", func(t *testing.T) {
		slot, err := EpochStart(params.BeaconConfig().BellatrixForkEpoch)
		require.NoError(t, err)

		result := ToForkVersion(slot)
		require.Equal(t, version.Bellatrix, result)
	})

	t.Run("Altair fork version", func(t *testing.T) {
		slot, err := EpochStart(params.BeaconConfig().AltairForkEpoch)
		require.NoError(t, err)

		result := ToForkVersion(slot)
		require.Equal(t, version.Altair, result)
	})

	t.Run("Phase0 fork version", func(t *testing.T) {
		slot, err := EpochStart(params.BeaconConfig().AltairForkEpoch)
		require.NoError(t, err)

		result := ToForkVersion(slot - 1)
		require.Equal(t, version.Phase0, result)
	})
}

func TestSlotTickerReplayBehaviour(t *testing.T) {
	secondsPerslot := uint64(1)
	st := NewSlotTicker(time.Unix(time.Now().Unix(), 0), secondsPerslot) // 1-second period
	const ticks = 5

	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second) // make the timeout very close
	defer cancel()
	time.Sleep(time.Duration(ticks) * time.Second) // simulate slow consumer by delaying tick consumption
	counter := 0
	prevTime := time.Now()
	for counter < ticks {
		select {
		case <-st.C(): // simulate ticks faster than supposed iteration due to replaying old ticks
			assert.Equal(t, true, time.Now().Sub(prevTime) < time.Duration(secondsPerslot)*time.Second)
			counter++
			prevTime = time.Now()
		case <-ctx.Done(): // timed out before enough ticks arrived
			t.Fatalf("expected %d ticks, got %d", ticks, counter)
		}
	}

	require.Equal(t, ticks, counter)
}
