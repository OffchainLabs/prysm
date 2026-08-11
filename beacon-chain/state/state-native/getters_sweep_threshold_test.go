package state_native_test

import (
	"testing"

	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// compoundingValidator returns a maxed-out compounding validator, i.e. one that is eligible
// for sweep withdrawals down to whatever threshold it is configured with.
func compoundingValidator() *ethpb.Validator {
	creds := make([]byte, 32)
	creds[0] = params.BeaconConfig().CompoundingWithdrawalPrefixByte
	for i := 12; i < 32; i++ {
		creds[i] = 0x99
	}
	return &ethpb.Validator{
		PublicKey:             make([]byte, 48),
		WithdrawalCredentials: creds,
		EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalanceElectra,
		ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
		WithdrawableEpoch:     params.BeaconConfig().FarFutureEpoch,
	}
}

// TestExpectedWithdrawalsGloas_CustomSweepThreshold checks that the validator sweep leaves
// the custom threshold behind rather than the validator's max effective balance.
func TestExpectedWithdrawalsGloas_CustomSweepThreshold(t *testing.T) {
	cfg := params.BeaconConfig()
	increment := cfg.EffectiveBalanceIncrement

	tests := []struct {
		name       string
		threshold  uint64
		balance    uint64
		wantAmount uint64
		wantSweep  bool
	}{
		{
			// No custom threshold: falls back to get_max_effective_balance (2048 ETH).
			name:       "zero threshold falls back to max effective balance",
			threshold:  0,
			balance:    cfg.MaxEffectiveBalanceElectra + 5*increment,
			wantAmount: 5 * increment,
			wantSweep:  true,
		},
		{
			name:       "custom 64 ETH threshold sweeps down to 64 ETH",
			threshold:  64 * increment,
			balance:    100 * increment,
			wantAmount: 36 * increment,
			wantSweep:  true,
		},
		{
			name:      "balance exactly at the custom threshold is not swept",
			threshold: 64 * increment,
			balance:   64 * increment,
			wantSweep: false,
		},
		{
			name:       "balance one gwei over the custom threshold is swept",
			threshold:  64 * increment,
			balance:    64*increment + 1,
			wantAmount: 1,
			wantSweep:  true,
		},
		{
			// Without a custom threshold this validator would not be swept at all,
			// since its balance is far below 2048 ETH.
			name:       "custom threshold enables sweeps well below max effective balance",
			threshold:  cfg.MinActivationBalance + increment,
			balance:    cfg.MinActivationBalance + 10*increment,
			wantAmount: 9 * increment,
			wantSweep:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
				Validators:                   []*ethpb.Validator{compoundingValidator()},
				Balances:                     []uint64{tt.balance},
				ValidatorSweepThresholds:     []uint64{tt.threshold},
				BuilderPendingWithdrawals:    []*ethpb.BuilderPendingWithdrawal{},
				PendingPartialWithdrawals:    []*ethpb.PendingPartialWithdrawal{},
				Builders:                     []*ethpb.Builder{},
				ExecutionPayloadAvailability: make([]byte, (params.BeaconConfig().SlotsPerHistoricalRoot+7)/8),
				FinalizedCheckpoint:          &ethpb.Checkpoint{Root: make([]byte, 32)},
			})
			require.NoError(t, err)

			result, err := st.ExpectedWithdrawalsGloas()
			require.NoError(t, err)

			if !tt.wantSweep {
				require.Equal(t, 0, len(result.Withdrawals))
				return
			}
			require.Equal(t, 1, len(result.Withdrawals))
			require.Equal(t, tt.wantAmount, result.Withdrawals[0].Amount)
		})
	}
}

func TestValidatorSweepThreshold_Setters(t *testing.T) {
	cfg := params.BeaconConfig()
	st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
		Validators:               []*ethpb.Validator{compoundingValidator(), compoundingValidator()},
		Balances:                 []uint64{0, 0},
		ValidatorSweepThresholds: []uint64{0, 0},
		FinalizedCheckpoint:      &ethpb.Checkpoint{Root: make([]byte, 32)},
	})
	require.NoError(t, err)

	require.NoError(t, st.SetValidatorSweepThresholdAtIndex(1, cfg.MaxEffectiveBalanceElectra))
	got, err := st.ValidatorSweepThreshold(1)
	require.NoError(t, err)
	require.Equal(t, cfg.MaxEffectiveBalanceElectra, got)

	// Index 0 is untouched.
	got, err = st.ValidatorSweepThreshold(0)
	require.NoError(t, err)
	require.Equal(t, uint64(0), got)

	// Out of range indices are rejected rather than silently growing the list.
	require.ErrorContains(t, "out of range", st.SetValidatorSweepThresholdAtIndex(2, 1))

	all, err := st.ValidatorSweepThresholds()
	require.NoError(t, err)
	require.DeepEqual(t, []uint64{0, cfg.MaxEffectiveBalanceElectra}, all)
}

// TestValidatorSweepThreshold_AppendIsolation checks that appending a new validator's
// threshold in one state is not visible from a state copied beforehand. Multi-value slices
// track appended elements per state, so this is the path most likely to leak between them.
func TestValidatorSweepThreshold_AppendIsolation(t *testing.T) {
	st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
		Validators:               []*ethpb.Validator{compoundingValidator()},
		Balances:                 []uint64{0},
		ValidatorSweepThresholds: []uint64{0},
		FinalizedCheckpoint:      &ethpb.Checkpoint{Root: make([]byte, 32)},
	})
	require.NoError(t, err)

	cp := st.Copy()
	require.NoError(t, cp.AppendValidator(compoundingValidator()))
	require.NoError(t, cp.AppendValidatorSweepThreshold(params.BeaconConfig().MaxEffectiveBalanceElectra))

	// The original still sees a single-entry list.
	original, err := st.ValidatorSweepThresholds()
	require.NoError(t, err)
	require.DeepEqual(t, []uint64{0}, original)

	copied, err := cp.ValidatorSweepThresholds()
	require.NoError(t, err)
	require.DeepEqual(t, []uint64{0, params.BeaconConfig().MaxEffectiveBalanceElectra}, copied)

	// And the appended entry is readable by index from the copy only.
	got, err := cp.ValidatorSweepThreshold(1)
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxEffectiveBalanceElectra, got)
	_, err = st.ValidatorSweepThreshold(1)
	require.ErrorContains(t, "out of range", err)
}

// TestValidatorSweepThreshold_CopyIsolation checks that mutating a copied state does not
// leak back into the original.
func TestValidatorSweepThreshold_CopyIsolation(t *testing.T) {
	st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
		Validators:               []*ethpb.Validator{compoundingValidator()},
		Balances:                 []uint64{0},
		ValidatorSweepThresholds: []uint64{0},
		FinalizedCheckpoint:      &ethpb.Checkpoint{Root: make([]byte, 32)},
	})
	require.NoError(t, err)

	cp := st.Copy()
	require.NoError(t, cp.SetValidatorSweepThresholdAtIndex(0, 1234))

	original, err := st.ValidatorSweepThreshold(0)
	require.NoError(t, err)
	require.Equal(t, uint64(0), original)

	copied, err := cp.ValidatorSweepThreshold(0)
	require.NoError(t, err)
	require.Equal(t, uint64(1234), copied)
}

// TestValidatorSweepThreshold_HashTreeRootChanges guards against the new field being left
// out of the state's merkleization.
func TestValidatorSweepThreshold_HashTreeRootChanges(t *testing.T) {
	st, err := util.NewBeaconStateGloas(func(s *ethpb.BeaconStateGloas) error {
		s.Validators = []*ethpb.Validator{compoundingValidator()}
		s.Balances = []uint64{0}
		s.ValidatorSweepThresholds = []uint64{0}
		return nil
	})
	require.NoError(t, err)

	before, err := st.HashTreeRoot(t.Context())
	require.NoError(t, err)

	require.NoError(t, st.SetValidatorSweepThresholdAtIndex(0, params.BeaconConfig().MaxEffectiveBalanceElectra))
	after, err := st.HashTreeRoot(t.Context())
	require.NoError(t, err)

	require.NotEqual(t, before, after)
}
