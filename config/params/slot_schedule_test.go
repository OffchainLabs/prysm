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

// 12s slots until Heze at epoch 10, 6s from there.
func hezeAtEpoch10(t *testing.T) *BeaconChainConfig {
	cfg := MainnetConfig().Copy()
	cfg.FuluForkEpoch = 5
	cfg.GloasForkEpoch = 8
	cfg.HezeForkEpoch = 10
	cfg.SlotDurationMillisHeze = 6000
	cfg.InitializeForkSchedule()
	require.NoError(t, cfg.validateSlotDurations())
	return cfg
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
		require.Equal(t, tc.want, testSchedule.durationAt(tc.slot, spe))
	}
}

func TestSlotScheduleSinceGenesis(t *testing.T) {
	t.Run("piecewise", func(t *testing.T) {
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
	})

	t.Run("overflow", func(t *testing.T) {
		// Overflows uint64 milliseconds.
		_, err := testSchedule.SinceGenesis(primitives.Slot(1<<63), spe)
		require.NotNil(t, err)
		// Fits uint64 milliseconds but overflows int64 nanoseconds.
		_, err = testSchedule.SinceGenesis(primitives.Slot(1e13), spe)
		require.NotNil(t, err)
		// A giant span in an interior entry must error rather than wrap.
		giantSpan := SlotSchedule{{Epoch: 0, SlotDurationMillis: 12000}, {Epoch: 1 << 40, SlotDurationMillis: 6000}}
		_, err = giantSpan.SinceGenesis(primitives.Slot(1<<46), spe)
		require.NotNil(t, err)
	})
}

func TestSlotScheduleSlotAt(t *testing.T) {
	genesis := time.Unix(1600000000, 0)

	t.Run("piecewise", func(t *testing.T) {
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
	})

	t.Run("giant entry span", func(t *testing.T) {
		giantSpan := SlotSchedule{{Epoch: 0, SlotDurationMillis: 12000}, {Epoch: 1 << 40, SlotDurationMillis: 6000}}
		require.Equal(t, primitives.Slot(2), giantSpan.SlotAt(genesis, genesis.Add(24*time.Second), spe))
	})

	t.Run("round trip", func(t *testing.T) {
		for _, slot := range []primitives.Slot{0, 1, 319, 320, 321, 639, 640, 641, 10000} {
			since, err := testSchedule.SinceGenesis(slot, spe)
			require.NoError(t, err)
			require.Equal(t, slot, testSchedule.SlotAt(genesis, genesis.Add(since), spe))
		}
	})
}

func TestSlotDurations(t *testing.T) {
	t.Run("heze unscheduled", func(t *testing.T) {
		require.DeepEqual(t, SlotSchedule{{Epoch: 0, SlotDurationMillis: 12000}}, MainnetConfig().SlotDurations())
		require.DeepEqual(t, SlotSchedule{{Epoch: 0, SlotDurationMillis: 6000}}, MinimalSpecConfig().SlotDurations())
	})

	t.Run("heze scheduled", func(t *testing.T) {
		require.DeepEqual(t, SlotSchedule{{Epoch: 0, SlotDurationMillis: 12000}, {Epoch: 10, SlotDurationMillis: 6000}}, hezeAtEpoch10(t).SlotDurations())
	})
}

func TestHasSlotDurationChange(t *testing.T) {
	require.Equal(t, false, MainnetConfig().HasSlotDurationChange())
	require.Equal(t, true, hezeAtEpoch10(t).HasSlotDurationChange())
}

func TestSlotDurationMillisAtEpoch(t *testing.T) {
	t.Run("heze at epoch 10", func(t *testing.T) {
		cfg := hezeAtEpoch10(t)
		require.Equal(t, uint64(12000), cfg.SlotDurationMillisAtEpoch(0))
		require.Equal(t, uint64(12000), cfg.SlotDurationMillisAtEpoch(9))
		require.Equal(t, uint64(6000), cfg.SlotDurationMillisAtEpoch(10))
		require.Equal(t, uint64(6000), cfg.SlotDurationMillisAtEpoch(1<<40))
	})

	t.Run("heze unscheduled", func(t *testing.T) {
		require.Equal(t, uint64(12000), MainnetConfig().SlotDurationMillisAtEpoch(1<<40))
	})

	t.Run("heze at genesis", func(t *testing.T) {
		// The Heze entry shadows the genesis one, so the genesis baseline is the Heze duration too.
		cfg := MainnetConfig().Copy()
		cfg.HezeForkEpoch = 0
		require.Equal(t, uint64(10000), cfg.SlotDurationMillisAtEpoch(cfg.GenesisEpoch))

		since, err := cfg.SinceGenesis(10)
		require.NoError(t, err)
		require.Equal(t, 100*time.Second, since)
	})
}

func TestSlotComponentDurationAt(t *testing.T) {
	t.Run("heze at epoch 10", func(t *testing.T) {
		cfg := hezeAtEpoch10(t)
		// Before Gloas the pre-Gloas fractions of 12s apply.
		require.Equal(t, 3999*time.Millisecond, cfg.SlotComponentDurationAt(AttestationDue, 0))
		require.Equal(t, 2000*time.Millisecond, cfg.SlotComponentDurationAt(ProposerReorgCutoff, 319))
		// Between Gloas and Heze the Gloas fractions of 12s apply.
		require.Equal(t, 3000*time.Millisecond, cfg.SlotComponentDurationAt(AttestationDue, 256))
		// From Heze the Gloas fractions of SLOT_DURATION_MS_HEZE apply.
		require.Equal(t, 1500*time.Millisecond, cfg.SlotComponentDurationAt(AttestationDue, 320))
		require.Equal(t, 3000*time.Millisecond, cfg.SlotComponentDurationAt(AggregateDue, 320))
		require.Equal(t, 4500*time.Millisecond, cfg.SlotComponentDurationAt(PayloadAttestationDue, 639))
	})

	t.Run("heze unscheduled", func(t *testing.T) {
		plain := MainnetConfig().Copy()
		plain.GloasForkEpoch = 1
		require.Equal(t, 3999*time.Millisecond, plain.SlotComponentDurationAt(AttestationDue, 31))
		require.Equal(t, 3000*time.Millisecond, plain.SlotComponentDurationAt(AttestationDue, 32))
		require.Equal(t, 6000*time.Millisecond, plain.SlotComponentDurationAt(PayloadDue, 32))
		require.Equal(t, 9000*time.Millisecond, plain.SlotComponentDurationAt(PayloadAttestationDue, 32))
	})
}

func TestBlobDataRetentionStartEpoch(t *testing.T) {
	t.Run("heze unscheduled", func(t *testing.T) {
		plain := MainnetConfig().Copy()
		require.Equal(t, primitives.Epoch(0), plain.BlobDataRetentionStartEpoch(3, 5))
		require.Equal(t, primitives.Epoch(95), plain.BlobDataRetentionStartEpoch(100, 5))
	})

	t.Run("heze at epoch 10", func(t *testing.T) {
		cfg := hezeAtEpoch10(t)
		// 5 epochs of 12s.
		cfg.MinBlobDataRetentionMillis = 5 * 32 * 12000

		// Before Heze the window is the plain epoch count.
		require.Equal(t, primitives.Epoch(4), cfg.BlobDataRetentionStartEpoch(9, 5))
		require.Equal(t, primitives.Epoch(0), cfg.BlobDataRetentionStartEpoch(2, 5))
		// Epoch 12 starts 2 epochs of 6s after the change, 1 epoch of 12s wall clock, so 4 more 12s epochs reach back to epoch 6.
		require.Equal(t, primitives.Epoch(6), cfg.BlobDataRetentionStartEpoch(12, 5))
		// Fully inside the 6s era 5 epochs of 12s wall clock span 10 epochs of 6s.
		require.Equal(t, primitives.Epoch(10), cfg.BlobDataRetentionStartEpoch(20, 5))
		// Epoch 22 starts 12 epochs of 6s after the change, the 10 epoch window ends 2 epochs of 6s in.
		require.Equal(t, primitives.Epoch(12), cfg.BlobDataRetentionStartEpoch(22, 5))
	})

	t.Run("longest window wins", func(t *testing.T) {
		cfg := hezeAtEpoch10(t)
		cfg.MinBlobDataRetentionMillis = 5 * 32 * 12000

		// MIN_BLOB_DATA_RETENTION_MS is longer than 1 epoch.
		require.Equal(t, primitives.Epoch(10), cfg.BlobDataRetentionStartEpoch(20, 1))
		// 10 epochs of 12s are longer than MIN_BLOB_DATA_RETENTION_MS, epoch 20 starts 15 epochs of 12s after genesis.
		require.Equal(t, primitives.Epoch(5), cfg.BlobDataRetentionStartEpoch(20, 10))
	})
}

func TestValidateSlotDurations(t *testing.T) {
	cases := []struct {
		name       string
		hezeEpoch  primitives.Epoch
		durationMs uint64
		wantErr    string
	}{
		{name: "heze unscheduled", hezeEpoch: 1<<64 - 1, durationMs: 0},
		{name: "valid", hezeEpoch: 10, durationMs: 6000},
		{name: "zero duration", hezeEpoch: 10, durationMs: 0, wantErr: "positive multiple of 1000"},
		{name: "sub-second duration", hezeEpoch: 10, durationMs: 6500, wantErr: "positive multiple of 1000"},
		{name: "epoch overflows slot arithmetic", hezeEpoch: 1 << 60, durationMs: 6000, wantErr: "overflows slot arithmetic"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := MainnetConfig().Copy()
			cfg.HezeForkEpoch = tc.hezeEpoch
			cfg.SlotDurationMillisHeze = tc.durationMs
			err := cfg.validateSlotDurations()
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, tc.wantErr, err)
			}
		})
	}
}

func TestSlotDurationsConfigYamlRoundTrip(t *testing.T) {
	cfg := hezeAtEpoch10(t)
	cfg.MinBlobDataRetentionMillis = 123_000
	out, err := UnmarshalConfig(ConfigToYaml(cfg), MainnetConfig().Copy())
	require.NoError(t, err)
	require.Equal(t, cfg.SlotDurationMillisHeze, out.SlotDurationMillisHeze)
	require.Equal(t, cfg.MinBlobDataRetentionMillis, out.MinBlobDataRetentionMillis)
	require.DeepEqual(t, cfg.SlotDurations(), out.SlotDurations())

	cfg.SlotDurationMillisHeze = 6500
	_, err = UnmarshalConfig(ConfigToYaml(cfg), MainnetConfig().Copy())
	require.ErrorContains(t, "positive multiple of 1000", err)
}
