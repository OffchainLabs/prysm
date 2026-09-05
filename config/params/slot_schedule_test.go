package params

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

const spe = primitives.Slot(32)

// 12s slots for epochs 0-9, 6s for epochs 10-19, 4s from epoch 20.
var testSchedule = SlotSchedule{
	{Epoch: 0, SlotDurationMillis: 12000},
	{Epoch: 10, SlotDurationMillis: 6000},
	{Epoch: 20, SlotDurationMillis: 4000},
}

func TestSlotScheduleValidate(t *testing.T) {
	cases := []struct {
		name     string
		schedule SlotSchedule
		wantErr  string
	}{
		{name: "valid single entry", schedule: SlotSchedule{{Epoch: 0, SlotDurationMillis: 12000}}},
		{name: "valid multi entry", schedule: testSchedule},
		{name: "empty", schedule: SlotSchedule{}, wantErr: "empty slot schedule"},
		{name: "missing epoch 0", schedule: SlotSchedule{{Epoch: 1, SlotDurationMillis: 12000}}, wantErr: "must start at epoch 0"},
		{name: "zero duration", schedule: SlotSchedule{{Epoch: 0, SlotDurationMillis: 0}}, wantErr: "zero duration"},
		{
			name:     "duplicate epoch",
			schedule: SlotSchedule{{Epoch: 0, SlotDurationMillis: 12000}, {Epoch: 10, SlotDurationMillis: 6000}, {Epoch: 10, SlotDurationMillis: 4000}},
			wantErr:  "strictly increasing",
		},
		{
			name:     "unsorted",
			schedule: SlotSchedule{{Epoch: 0, SlotDurationMillis: 12000}, {Epoch: 20, SlotDurationMillis: 6000}, {Epoch: 10, SlotDurationMillis: 4000}},
			wantErr:  "strictly increasing",
		},
		{
			name:     "epoch overflows slot arithmetic",
			schedule: SlotSchedule{{Epoch: 0, SlotDurationMillis: 12000}, {Epoch: primitives.Epoch(1 << 60), SlotDurationMillis: 6000}},
			wantErr:  "overflows slot arithmetic",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.schedule.Validate(spe)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, tc.wantErr, err)
			}
		})
	}
}

func TestSlotScheduleDurationAt(t *testing.T) {
	cases := []struct {
		slot primitives.Slot
		want time.Duration
	}{
		{slot: 0, want: 12 * time.Second},
		{slot: 319, want: 12 * time.Second},
		{slot: 320, want: 6 * time.Second},
		{slot: 639, want: 6 * time.Second},
		{slot: 640, want: 4 * time.Second},
		{slot: 1 << 40, want: 4 * time.Second},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, testSchedule.DurationAt(tc.slot, spe))
	}
}

func TestSlotScheduleSinceGenesis(t *testing.T) {
	firstEntrySpan := 320 * 12 * time.Second
	secondEntrySpan := 320 * 6 * time.Second
	cases := []struct {
		slot primitives.Slot
		want time.Duration
	}{
		{slot: 0, want: 0},
		{slot: 1, want: 12 * time.Second},
		{slot: 320, want: firstEntrySpan},
		{slot: 321, want: firstEntrySpan + 6*time.Second},
		{slot: 640, want: firstEntrySpan + secondEntrySpan},
		{slot: 645, want: firstEntrySpan + secondEntrySpan + 5*4*time.Second},
	}
	for _, tc := range cases {
		got, err := testSchedule.SinceGenesis(tc.slot, spe)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
}

func TestSlotScheduleSinceGenesisOverflow(t *testing.T) {
	// Overflows uint64 milliseconds.
	_, err := testSchedule.SinceGenesis(primitives.Slot(1<<63), spe)
	require.NotNil(t, err)
	// Fits uint64 milliseconds but overflows int64 nanoseconds.
	_, err = testSchedule.SinceGenesis(primitives.Slot(1e13), spe)
	require.NotNil(t, err)
	// A giant span in an interior entry must error rather than wrap.
	giantSpan := SlotSchedule{{Epoch: 0, SlotDurationMillis: 12000}, {Epoch: 1 << 40, SlotDurationMillis: 6000}}
	require.NoError(t, giantSpan.Validate(spe))
	_, err = giantSpan.SinceGenesis(primitives.Slot(1<<46), spe)
	require.NotNil(t, err)
}

func TestSlotScheduleSlotAtGiantEntrySpan(t *testing.T) {
	giantSpan := SlotSchedule{{Epoch: 0, SlotDurationMillis: 12000}, {Epoch: 1 << 40, SlotDurationMillis: 6000}}
	require.NoError(t, giantSpan.Validate(spe))
	genesis := time.Unix(1600000000, 0)
	require.Equal(t, primitives.Slot(2), giantSpan.SlotAt(genesis, genesis.Add(24*time.Second), spe))
}

func TestSlotScheduleSlotAt(t *testing.T) {
	genesis := time.Unix(1600000000, 0)
	firstEntrySpan := 320 * 12 * time.Second
	secondEntrySpan := 320 * 6 * time.Second
	cases := []struct {
		offset time.Duration
		want   primitives.Slot
	}{
		{offset: -time.Hour, want: 0},
		{offset: 0, want: 0},
		{offset: 11 * time.Second, want: 0},
		{offset: 12 * time.Second, want: 1},
		{offset: firstEntrySpan - time.Millisecond, want: 319},
		{offset: firstEntrySpan, want: 320},
		{offset: firstEntrySpan + 6*time.Second, want: 321},
		{offset: firstEntrySpan + secondEntrySpan - time.Millisecond, want: 639},
		{offset: firstEntrySpan + secondEntrySpan, want: 640},
		{offset: firstEntrySpan + secondEntrySpan + 9*time.Second, want: 642},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, testSchedule.SlotAt(genesis, genesis.Add(tc.offset), spe))
	}
}

func TestSlotScheduleConfigYamlRoundTrip(t *testing.T) {
	cfg := MainnetConfig().Copy()
	cfg.SlotSchedule = testSchedule
	out, err := UnmarshalConfig(ConfigToYaml(cfg), MainnetConfig().Copy())
	require.NoError(t, err)
	require.DeepEqual(t, cfg.SlotSchedule, out.SlotSchedule)
}

func TestSlotScheduleConfigMismatch(t *testing.T) {
	cfg := MainnetConfig().Copy()
	cfg.SlotSchedule = SlotSchedule{{Epoch: 0, SlotDurationMillis: 6000}}
	_, err := UnmarshalConfig(ConfigToYaml(cfg), MainnetConfig().Copy())
	require.ErrorContains(t, "does not match the configured slot duration", err)
}

func TestSlotScheduleRoundTrip(t *testing.T) {
	genesis := time.Unix(1600000000, 0)
	for _, slot := range []primitives.Slot{0, 1, 319, 320, 321, 639, 640, 641, 10000} {
		since, err := testSchedule.SinceGenesis(slot, spe)
		require.NoError(t, err)
		require.Equal(t, slot, testSchedule.SlotAt(genesis, genesis.Add(since), spe))
	}
}
