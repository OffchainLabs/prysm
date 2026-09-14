package params

import (
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

const spe = primitives.Slot(32)

// 12s slots for epochs 0-9, 6s for epochs 10-19, 4s from epoch 20.
var testSchedule = SlotSchedule{
	SlotScheduleEntryForTest(0, 12000),
	SlotScheduleEntryForTest(10, 6000),
	SlotScheduleEntryForTest(20, 4000),
}

func TestSlotScheduleValidate(t *testing.T) {
	withField := func(epoch primitives.Epoch, mutate func(*SlotScheduleEntry)) SlotSchedule {
		e := SlotScheduleEntryForTest(epoch, 12000)
		mutate(&e)
		if epoch == 0 {
			return SlotSchedule{e}
		}
		return SlotSchedule{SlotScheduleEntryForTest(0, 12000), e}
	}
	cases := []struct {
		name     string
		schedule SlotSchedule
		wantErr  string
	}{
		{name: "valid single entry", schedule: SlotSchedule{SlotScheduleEntryForTest(0, 12000)}},
		{name: "valid multi entry", schedule: testSchedule},
		{name: "empty", schedule: SlotSchedule{}, wantErr: "empty slot schedule"},
		{name: "missing epoch 0", schedule: SlotSchedule{SlotScheduleEntryForTest(1, 12000)}, wantErr: "must start at epoch 0"},
		{name: "zero duration", schedule: withField(0, func(e *SlotScheduleEntry) { e.SlotDurationMillis = 0 }), wantErr: "zero duration"},
		{name: "sub-second duration", schedule: withField(10, func(e *SlotScheduleEntry) { e.SlotDurationMillis = 6500 }), wantErr: "multiple of 1000"},
		{name: "zero deadline", schedule: withField(10, func(e *SlotScheduleEntry) { e.PayloadDueMillis = 0 }), wantErr: "PAYLOAD_DUE_MS=0"},
		{name: "deadline at slot end", schedule: withField(10, func(e *SlotScheduleEntry) { e.InclusionListDueMillis = 12000 }), wantErr: "INCLUSION_LIST_DUE_MS=12000"},
		{name: "reorg cutoff after attestation", schedule: withField(10, func(e *SlotScheduleEntry) { e.ProposerReorgCutoffMillis = 3000 }), wantErr: "proposer reorg cutoff < attestation due"},
		{name: "aggregate before attestation", schedule: withField(10, func(e *SlotScheduleEntry) { e.AggregateDueMillis = 2500 }), wantErr: "attestation due < aggregate due"},
		{name: "contribution before sync message", schedule: withField(10, func(e *SlotScheduleEntry) { e.ContributionDueMillis = 3000 }), wantErr: "sync message due < contribution due"},
		{name: "payload attestation before payload", schedule: withField(10, func(e *SlotScheduleEntry) { e.PayloadAttestationDueMillis = 6000 }), wantErr: "payload due < payload attestation due"},
		{
			name:     "duplicate epoch",
			schedule: SlotSchedule{SlotScheduleEntryForTest(0, 12000), SlotScheduleEntryForTest(10, 6000), SlotScheduleEntryForTest(10, 4000)},
			wantErr:  "strictly increasing",
		},
		{
			name:     "unsorted",
			schedule: SlotSchedule{SlotScheduleEntryForTest(0, 12000), SlotScheduleEntryForTest(20, 6000), SlotScheduleEntryForTest(10, 4000)},
			wantErr:  "strictly increasing",
		},
		{
			name:     "epoch overflows slot arithmetic",
			schedule: SlotSchedule{SlotScheduleEntryForTest(0, 12000), SlotScheduleEntryForTest(1<<60, 6000)},
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
	giantSpan := SlotSchedule{SlotScheduleEntryForTest(0, 12000), SlotScheduleEntryForTest(1<<40, 6000)}
	require.NoError(t, giantSpan.Validate(spe))
	_, err = giantSpan.SinceGenesis(primitives.Slot(1<<46), spe)
	require.NotNil(t, err)
}

func TestSlotScheduleSlotAtGiantEntrySpan(t *testing.T) {
	giantSpan := SlotSchedule{SlotScheduleEntryForTest(0, 12000), SlotScheduleEntryForTest(1<<40, 6000)}
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

// 12s slots until Gloas at epoch 10, 6s from there.
func scheduledMainnet(t *testing.T) *BeaconChainConfig {
	cfg := MainnetConfig().Copy()
	cfg.FuluForkEpoch = 5
	cfg.GloasForkEpoch = 10
	cfg.SlotDurationSchedule = SlotSchedule{SlotScheduleEntryForTest(0, 12000), SlotScheduleEntryForTest(10, 6000)}
	cfg.InitializeForkSchedule()
	require.NoError(t, validateSlotDurationSchedule(cfg))
	return cfg
}

func TestSlotScheduleConfigYamlRoundTrip(t *testing.T) {
	cfg := scheduledMainnet(t)
	out, err := UnmarshalConfig(ConfigToYaml(cfg), MainnetConfig().Copy())
	require.NoError(t, err)
	require.DeepEqual(t, cfg.SlotDurationSchedule, out.SlotDurationSchedule)
}

func TestSlotScheduleConfigMismatch(t *testing.T) {
	cfg := MainnetConfig().Copy()
	cfg.SlotDurationSchedule = SlotSchedule{SlotScheduleEntryForTest(0, 6000)}
	_, err := UnmarshalConfig(ConfigToYaml(cfg), MainnetConfig().Copy())
	require.ErrorContains(t, "does not match the configured slot duration", err)
}

func TestSlotScheduleConfigForkAlignment(t *testing.T) {
	cfg := MainnetConfig().Copy()
	cfg.SlotDurationSchedule = SlotSchedule{SlotScheduleEntryForTest(0, 12000), SlotScheduleEntryForTest(10, 6000)}
	_, err := UnmarshalConfig(ConfigToYaml(cfg), MainnetConfig().Copy())
	require.ErrorContains(t, "epoch 10 precedes GLOAS_FORK_EPOCH", err)

	cfg.GloasForkEpoch = 8
	_, err = UnmarshalConfig(ConfigToYaml(cfg), MainnetConfig().Copy())
	require.ErrorContains(t, "epoch 10 does not coincide with a fork epoch", err)
}

func TestSlotScheduleRoundTrip(t *testing.T) {
	genesis := time.Unix(1600000000, 0)
	for _, slot := range []primitives.Slot{0, 1, 319, 320, 321, 639, 640, 641, 10000} {
		since, err := testSchedule.SinceGenesis(slot, spe)
		require.NoError(t, err)
		require.Equal(t, slot, testSchedule.SlotAt(genesis, genesis.Add(since), spe))
	}
}

func TestSlotDurationMillisAtEpoch(t *testing.T) {
	cfg := scheduledMainnet(t)
	require.Equal(t, uint64(12000), cfg.SlotDurationMillisAtEpoch(0))
	require.Equal(t, uint64(12000), cfg.SlotDurationMillisAtEpoch(9))
	require.Equal(t, uint64(6000), cfg.SlotDurationMillisAtEpoch(10))
	require.Equal(t, uint64(6000), cfg.SlotDurationMillisAtEpoch(1<<40))
	require.Equal(t, uint64(12000), MainnetConfig().SlotDurationMillisAtEpoch(1<<40))
}

func TestSlotComponentDurationAt(t *testing.T) {
	cfg := scheduledMainnet(t)
	// Before the first change the *_DUE_BPS fractions apply, pre-Gloas values since Gloas is at epoch 10.
	require.Equal(t, 3999*time.Millisecond, cfg.SlotComponentDurationAt(AttestationDue, 0))
	require.Equal(t, 2000*time.Millisecond, cfg.SlotComponentDurationAt(ProposerReorgCutoff, 319))
	// From epoch 10 the explicit entry deadlines apply.
	require.Equal(t, 1500*time.Millisecond, cfg.SlotComponentDurationAt(AttestationDue, 320))
	require.Equal(t, 3000*time.Millisecond, cfg.SlotComponentDurationAt(AggregateDue, 320))
	require.Equal(t, 4500*time.Millisecond, cfg.SlotComponentDurationAt(PayloadAttestationDue, 639))
	require.Equal(t, 4000*time.Millisecond, cfg.SlotComponentDurationAt(InclusionListDue, 640))

	// Without a schedule the fractions follow the Gloas switch.
	plain := MainnetConfig().Copy()
	plain.GloasForkEpoch = 1
	require.Equal(t, 3999*time.Millisecond, plain.SlotComponentDurationAt(AttestationDue, 31))
	require.Equal(t, 3000*time.Millisecond, plain.SlotComponentDurationAt(AttestationDue, 32))
	require.Equal(t, 6000*time.Millisecond, plain.SlotComponentDurationAt(PayloadDue, 32))
	require.Equal(t, 9000*time.Millisecond, plain.SlotComponentDurationAt(PayloadAttestationDue, 32))
}

func TestRetentionStartEpoch(t *testing.T) {
	plain := MainnetConfig().Copy()
	require.Equal(t, primitives.Epoch(0), plain.RetentionStartEpoch(3, 5))
	require.Equal(t, primitives.Epoch(95), plain.RetentionStartEpoch(100, 5))

	cfg := scheduledMainnet(t)
	// Fully inside the 12s era the window is the plain epoch count.
	require.Equal(t, primitives.Epoch(4), cfg.RetentionStartEpoch(9, 5))
	// Epoch 12 starts 2 epochs of 6s after the change, 1 epoch of 12s wall clock, so 4 more 12s epochs reach back to epoch 6.
	require.Equal(t, primitives.Epoch(6), cfg.RetentionStartEpoch(12, 5))
	// Fully inside the 6s era 5 epochs of 12s wall clock span 10 epochs of 6s.
	require.Equal(t, primitives.Epoch(10), cfg.RetentionStartEpoch(20, 5))
	// Epoch 22 starts 12 epochs of 6s after the change, the 10 epoch window ends 2 epochs of 6s in.
	require.Equal(t, primitives.Epoch(12), cfg.RetentionStartEpoch(22, 5))
	require.Equal(t, primitives.Epoch(0), cfg.RetentionStartEpoch(2, 5))
}

func TestSlotScheduleConfigDerivesSlotDuration(t *testing.T) {
	cfg := MinimalSpecConfig().Copy()
	cfg.GloasForkEpoch = 10
	cfg.SlotDurationSchedule = SlotSchedule{SlotScheduleEntryForTest(0, 6000), SlotScheduleEntryForTest(10, 4000)}
	var lines []string
	for _, l := range strings.Split(string(ConfigToYaml(cfg)), "\n") {
		if strings.HasPrefix(l, "SLOT_DURATION_MS:") || strings.HasPrefix(l, "SECONDS_PER_SLOT:") {
			continue
		}
		lines = append(lines, l)
	}
	out, err := UnmarshalConfig([]byte(strings.Join(lines, "\n")), MainnetConfig().Copy())
	require.NoError(t, err)
	require.Equal(t, uint64(6000), out.SlotDurationMillis())
	require.Equal(t, uint64(6), out.SecondsPerSlot)
}

func TestGenesisSlotScheduleEntry(t *testing.T) {
	// Matches the epoch 0 entry of the spec's mainnet.yaml and minimal.yaml.
	require.Equal(t, SlotScheduleEntryForTest(0, 12000), MainnetConfig().GenesisSlotScheduleEntry())
	require.Equal(t, SlotScheduleEntryForTest(0, 6000), MinimalSpecConfig().GenesisSlotScheduleEntry())
	require.NoError(t, MainnetConfig().GenesisSlotScheduleEntry().validate())
}
