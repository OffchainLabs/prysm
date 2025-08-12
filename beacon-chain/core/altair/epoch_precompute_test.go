package altair

import (
	"math"
	"math/big"
	"math/bits"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/epoch/precompute"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestInitializeEpochValidators_Ok(t *testing.T) {
	ffe := params.BeaconConfig().FarFutureEpoch
	s, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Slot: params.BeaconConfig().SlotsPerEpoch,
		// Validator 0 is slashed
		// Validator 1 is withdrawable
		// Validator 2 is active prev epoch and current epoch
		// Validator 3 is active prev epoch
		Validators: []*ethpb.Validator{
			{Slashed: true, WithdrawableEpoch: ffe, EffectiveBalance: 100},
			{EffectiveBalance: 100},
			{WithdrawableEpoch: ffe, ExitEpoch: ffe, EffectiveBalance: 100},
			{WithdrawableEpoch: ffe, ExitEpoch: 1, EffectiveBalance: 100},
		},
		InactivityScores: []uint64{0, 1, 2, 3},
	})
	require.NoError(t, err)
	v, b, err := InitializePrecomputeValidators(t.Context(), s)
	require.NoError(t, err)
	assert.DeepEqual(t, &precompute.Validator{
		IsSlashed:                    true,
		CurrentEpochEffectiveBalance: 100,
		InactivityScore:              0,
	}, v[0], "Incorrect validator 0 status")
	assert.DeepEqual(t, &precompute.Validator{
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: 100,
		InactivityScore:              1,
	}, v[1], "Incorrect validator 1 status")
	assert.DeepEqual(t, &precompute.Validator{
		IsActivePrevEpoch:            true,
		IsActiveCurrentEpoch:         true,
		CurrentEpochEffectiveBalance: 100,
		InactivityScore:              2,
	}, v[2], "Incorrect validator 2 status")
	assert.DeepEqual(t, &precompute.Validator{
		IsActivePrevEpoch:            true,
		CurrentEpochEffectiveBalance: 100,
		InactivityScore:              3,
	}, v[3], "Incorrect validator 3 status")

	wantedBalances := &precompute.Balance{
		ActiveCurrentEpoch: 100,
		ActivePrevEpoch:    200,
	}
	assert.DeepEqual(t, wantedBalances, b, "Incorrect wanted balance")
}

func TestInitializeEpochValidators_Overflow(t *testing.T) {
	ffe := params.BeaconConfig().FarFutureEpoch
	s, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Slot: params.BeaconConfig().SlotsPerEpoch,
		Validators: []*ethpb.Validator{
			{WithdrawableEpoch: ffe, ExitEpoch: ffe, EffectiveBalance: math.MaxUint64},
			{WithdrawableEpoch: ffe, ExitEpoch: ffe, EffectiveBalance: math.MaxUint64},
		},
		InactivityScores: []uint64{0, 1},
	})
	require.NoError(t, err)
	_, _, err = InitializePrecomputeValidators(t.Context(), s)
	require.ErrorContains(t, "could not read every validator: addition overflows", err)
}

func TestInitializeEpochValidators_BadState(t *testing.T) {
	s, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Validators:       []*ethpb.Validator{{}},
		InactivityScores: []uint64{},
	})
	require.NoError(t, err)
	_, _, err = InitializePrecomputeValidators(t.Context(), s)
	require.ErrorContains(t, "num of validators is different than num of inactivity scores", err)
}

func TestProcessEpochParticipation(t *testing.T) {
	s, err := testState()
	require.NoError(t, err)
	validators, balance, err := InitializePrecomputeValidators(t.Context(), s)
	require.NoError(t, err)
	validators, balance, err = ProcessEpochParticipation(t.Context(), s, balance, validators)
	require.NoError(t, err)
	require.DeepEqual(t, &precompute.Validator{
		IsActiveCurrentEpoch:         true,
		IsActivePrevEpoch:            true,
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
	}, validators[0])
	require.DeepEqual(t, &precompute.Validator{
		IsActiveCurrentEpoch:         true,
		IsActivePrevEpoch:            true,
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		IsCurrentEpochAttester:       true,
		IsPrevEpochAttester:          true,
		IsPrevEpochSourceAttester:    true,
	}, validators[1])
	require.DeepEqual(t, &precompute.Validator{
		IsActiveCurrentEpoch:         true,
		IsActivePrevEpoch:            true,
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		IsCurrentEpochAttester:       true,
		IsPrevEpochAttester:          true,
		IsPrevEpochSourceAttester:    true,
		IsCurrentEpochTargetAttester: true,
		IsPrevEpochTargetAttester:    true,
	}, validators[2])
	require.DeepEqual(t, &precompute.Validator{
		IsActiveCurrentEpoch:         true,
		IsActivePrevEpoch:            true,
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		IsCurrentEpochAttester:       true,
		IsPrevEpochAttester:          true,
		IsPrevEpochSourceAttester:    true,
		IsCurrentEpochTargetAttester: true,
		IsPrevEpochTargetAttester:    true,
		IsPrevEpochHeadAttester:      true,
	}, validators[3])
	require.Equal(t, params.BeaconConfig().MaxEffectiveBalance*3, balance.PrevEpochAttested)
	require.Equal(t, balance.CurrentEpochTargetAttested, params.BeaconConfig().MaxEffectiveBalance*2)
	require.Equal(t, balance.PrevEpochTargetAttested, params.BeaconConfig().MaxEffectiveBalance*2)
	require.Equal(t, balance.PrevEpochHeadAttested, params.BeaconConfig().MaxEffectiveBalance*1)
}

func TestProcessEpochParticipation_InactiveValidator(t *testing.T) {
	generateParticipation := func(flags ...uint8) byte {
		b := byte(0)
		var err error
		for _, flag := range flags {
			b, err = AddValidatorFlag(b, flag)
			require.NoError(t, err)
		}
		return b
	}
	st, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Slot: 2 * params.BeaconConfig().SlotsPerEpoch,
		Validators: []*ethpb.Validator{
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance},                                                  // Inactive
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance, ExitEpoch: 2},                                    // Inactive current epoch, active previous epoch
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance, ExitEpoch: params.BeaconConfig().FarFutureEpoch}, // Active
		},
		CurrentEpochParticipation: []byte{
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex),
		},
		PreviousEpochParticipation: []byte{
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex),
		},
		InactivityScores: []uint64{0, 0, 0},
	})
	require.NoError(t, err)
	validators, balance, err := InitializePrecomputeValidators(t.Context(), st)
	require.NoError(t, err)
	validators, balance, err = ProcessEpochParticipation(t.Context(), st, balance, validators)
	require.NoError(t, err)
	require.DeepEqual(t, &precompute.Validator{
		IsActiveCurrentEpoch:         false,
		IsActivePrevEpoch:            false,
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
	}, validators[0])
	require.DeepEqual(t, &precompute.Validator{
		IsActiveCurrentEpoch:         false,
		IsActivePrevEpoch:            true,
		IsPrevEpochAttester:          true,
		IsPrevEpochSourceAttester:    true,
		IsPrevEpochTargetAttester:    true,
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
	}, validators[1])
	require.DeepEqual(t, &precompute.Validator{
		IsActiveCurrentEpoch:         true,
		IsActivePrevEpoch:            true,
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		IsCurrentEpochAttester:       true,
		IsPrevEpochAttester:          true,
		IsPrevEpochSourceAttester:    true,
		IsCurrentEpochTargetAttester: true,
		IsPrevEpochTargetAttester:    true,
		IsPrevEpochHeadAttester:      true,
	}, validators[2])
	require.Equal(t, balance.PrevEpochAttested, 2*params.BeaconConfig().MaxEffectiveBalance)
	require.Equal(t, balance.CurrentEpochTargetAttested, params.BeaconConfig().MaxEffectiveBalance)
	require.Equal(t, balance.PrevEpochTargetAttested, 2*params.BeaconConfig().MaxEffectiveBalance)
	require.Equal(t, balance.PrevEpochHeadAttested, params.BeaconConfig().MaxEffectiveBalance)
}

func TestAttestationsDelta(t *testing.T) {
	s, err := testState()
	require.NoError(t, err)
	validators, balance, err := InitializePrecomputeValidators(t.Context(), s)
	require.NoError(t, err)
	validators, balance, err = ProcessEpochParticipation(t.Context(), s, balance, validators)
	require.NoError(t, err)
	deltas, err := AttestationsDelta(s, balance, validators)
	require.NoError(t, err)

	rewards := make([]uint64, len(deltas))
	penalties := make([]uint64, len(deltas))
	for i, d := range deltas {
		rewards[i] = d.HeadReward + d.SourceReward + d.TargetReward
		penalties[i] = d.SourcePenalty + d.TargetPenalty + d.InactivityPenalty
	}

	// Reward amount should increase as validator index increases due to setup.
	for i := 1; i < len(rewards); i++ {
		require.Equal(t, true, rewards[i] > rewards[i-1])
	}

	// Penalty amount should decrease as validator index increases due to setup.
	for i := 1; i < len(penalties); i++ {
		require.Equal(t, true, penalties[i] <= penalties[i-1])
	}

	// First index should have 0 reward.
	require.Equal(t, uint64(0), rewards[0])
	// Last index should have 0 penalty.
	require.Equal(t, uint64(0), penalties[len(penalties)-1])

	want := []uint64{0, 939146, 2101898, 2414946}
	require.DeepEqual(t, want, rewards)
	want = []uint64{3577700, 2325505, 0, 0}
	require.DeepEqual(t, want, penalties)
}

func TestAttestationsDeltaBellatrix(t *testing.T) {
	s, err := testStateBellatrix()
	require.NoError(t, err)
	validators, balance, err := InitializePrecomputeValidators(t.Context(), s)
	require.NoError(t, err)
	validators, balance, err = ProcessEpochParticipation(t.Context(), s, balance, validators)
	require.NoError(t, err)
	deltas, err := AttestationsDelta(s, balance, validators)
	require.NoError(t, err)

	rewards := make([]uint64, len(deltas))
	penalties := make([]uint64, len(deltas))
	for i, d := range deltas {
		rewards[i] = d.HeadReward + d.SourceReward + d.TargetReward
		penalties[i] = d.SourcePenalty + d.TargetPenalty + d.InactivityPenalty
	}

	// Reward amount should increase as validator index increases due to setup.
	for i := 1; i < len(rewards); i++ {
		require.Equal(t, true, rewards[i] > rewards[i-1])
	}

	// Penalty amount should decrease as validator index increases due to setup.
	for i := 1; i < len(penalties); i++ {
		require.Equal(t, true, penalties[i] <= penalties[i-1])
	}

	// First index should have 0 reward.
	require.Equal(t, uint64(0), rewards[0])
	// Last index should have 0 penalty.
	require.Equal(t, uint64(0), penalties[len(penalties)-1])

	want := []uint64{0, 939146, 2101898, 2414946}
	require.DeepEqual(t, want, rewards)
	want = []uint64{3577700, 2325505, 0, 0}
	require.DeepEqual(t, want, penalties)
}

func TestProcessRewardsAndPenaltiesPrecompute_Ok(t *testing.T) {
	s, err := testState()
	require.NoError(t, err)
	validators, balance, err := InitializePrecomputeValidators(t.Context(), s)
	require.NoError(t, err)
	validators, balance, err = ProcessEpochParticipation(t.Context(), s, balance, validators)
	require.NoError(t, err)
	s, err = ProcessRewardsAndPenaltiesPrecompute(s, balance, validators)
	require.NoError(t, err)

	balances := s.Balances()
	// Reward amount should increase as validator index increases due to setup.
	for i := 1; i < len(balances); i++ {
		require.Equal(t, true, balances[i] >= balances[i-1])
	}

	wanted := make([]uint64, s.NumValidators())
	deltas, err := AttestationsDelta(s, balance, validators)
	require.NoError(t, err)

	rewards := make([]uint64, len(deltas))
	penalties := make([]uint64, len(deltas))
	for i, d := range deltas {
		rewards[i] = d.HeadReward + d.SourceReward + d.TargetReward
		penalties[i] = d.SourcePenalty + d.TargetPenalty + d.InactivityPenalty
	}
	for i := range rewards {
		wanted[i] += rewards[i]
	}
	for i := range penalties {
		if wanted[i] > penalties[i] {
			wanted[i] -= penalties[i]
		} else {
			wanted[i] = 0
		}
	}
	require.DeepEqual(t, wanted, balances)
}

func TestProcessRewardsAndPenaltiesPrecompute_InactivityLeak(t *testing.T) {
	s, err := testState()
	require.NoError(t, err)
	validators, balance, err := InitializePrecomputeValidators(t.Context(), s)
	require.NoError(t, err)
	validators, balance, err = ProcessEpochParticipation(t.Context(), s, balance, validators)
	require.NoError(t, err)
	sCopy := s.Copy()
	s, err = ProcessRewardsAndPenaltiesPrecompute(s, balance, validators)
	require.NoError(t, err)

	// Copied state where finality happened long ago
	require.NoError(t, sCopy.SetSlot(params.BeaconConfig().SlotsPerEpoch*1000))
	sCopy, err = ProcessRewardsAndPenaltiesPrecompute(sCopy, balance, validators)
	require.NoError(t, err)

	balances := s.Balances()
	inactivityBalances := sCopy.Balances()
	// Balances decreased to 0 due to inactivity
	require.Equal(t, uint64(2101898), balances[2])
	require.Equal(t, uint64(2414946), balances[3])
	require.Equal(t, uint64(0), inactivityBalances[2])
	require.Equal(t, uint64(0), inactivityBalances[3])
}

func TestProcessInactivityScores_CanProcessInactivityLeak(t *testing.T) {
	s, err := testState()
	require.NoError(t, err)
	defaultScore := uint64(5)
	require.NoError(t, s.SetInactivityScores([]uint64{defaultScore, defaultScore, defaultScore, defaultScore}))
	require.NoError(t, s.SetSlot(params.BeaconConfig().SlotsPerEpoch*primitives.Slot(params.BeaconConfig().MinEpochsToInactivityPenalty+2)))
	validators, balance, err := InitializePrecomputeValidators(t.Context(), s)
	require.NoError(t, err)
	validators, _, err = ProcessEpochParticipation(t.Context(), s, balance, validators)
	require.NoError(t, err)
	s, _, err = ProcessInactivityScores(t.Context(), s, validators)
	require.NoError(t, err)
	inactivityScores, err := s.InactivityScores()
	require.NoError(t, err)
	// V0 and V1 didn't vote head. V2 and V3 did.
	require.Equal(t, defaultScore+params.BeaconConfig().InactivityScoreBias, inactivityScores[0])
	require.Equal(t, defaultScore+params.BeaconConfig().InactivityScoreBias, inactivityScores[1])
	require.Equal(t, defaultScore-1, inactivityScores[2])
	require.Equal(t, defaultScore-1, inactivityScores[3])
}

func TestProcessInactivityScores_GenesisEpoch(t *testing.T) {
	s, err := testState()
	require.NoError(t, err)
	defaultScore := uint64(10)
	require.NoError(t, s.SetInactivityScores([]uint64{defaultScore, defaultScore, defaultScore, defaultScore}))
	require.NoError(t, s.SetSlot(params.BeaconConfig().GenesisSlot))
	validators, balance, err := InitializePrecomputeValidators(t.Context(), s)
	require.NoError(t, err)
	validators, _, err = ProcessEpochParticipation(t.Context(), s, balance, validators)
	require.NoError(t, err)
	s, _, err = ProcessInactivityScores(t.Context(), s, validators)
	require.NoError(t, err)
	inactivityScores, err := s.InactivityScores()
	require.NoError(t, err)
	require.Equal(t, defaultScore, inactivityScores[0])
	require.Equal(t, defaultScore, inactivityScores[1])
	require.Equal(t, defaultScore, inactivityScores[2])
	require.Equal(t, defaultScore, inactivityScores[3])
}

func TestProcessInactivityScores_CanProcessNonInactivityLeak(t *testing.T) {
	s, err := testState()
	require.NoError(t, err)
	defaultScore := uint64(5)
	require.NoError(t, s.SetInactivityScores([]uint64{defaultScore, defaultScore, defaultScore, defaultScore}))
	validators, balance, err := InitializePrecomputeValidators(t.Context(), s)
	require.NoError(t, err)
	validators, _, err = ProcessEpochParticipation(t.Context(), s, balance, validators)
	require.NoError(t, err)
	s, _, err = ProcessInactivityScores(t.Context(), s, validators)
	require.NoError(t, err)
	inactivityScores, err := s.InactivityScores()
	require.NoError(t, err)

	require.Equal(t, uint64(0), inactivityScores[0])
	require.Equal(t, uint64(0), inactivityScores[1])
	require.Equal(t, uint64(0), inactivityScores[2])
	require.Equal(t, uint64(0), inactivityScores[3])
}

func TestProcessRewardsAndPenaltiesPrecompute_GenesisEpoch(t *testing.T) {
	s, err := testState()
	require.NoError(t, err)
	validators, balance, err := InitializePrecomputeValidators(t.Context(), s)
	require.NoError(t, err)
	validators, balance, err = ProcessEpochParticipation(t.Context(), s, balance, validators)
	require.NoError(t, err)
	require.NoError(t, s.SetSlot(0))
	s, err = ProcessRewardsAndPenaltiesPrecompute(s, balance, validators)
	require.NoError(t, err)

	balances := s.Balances()
	// Nothing should happen at genesis epoch
	require.Equal(t, uint64(0), balances[0])
	for i := 1; i < len(balances); i++ {
		require.Equal(t, true, balances[i] == balances[i-1])
	}
}

func TestProcessRewardsAndPenaltiesPrecompute_BadState(t *testing.T) {
	s, err := testState()
	require.NoError(t, err)
	validators, balance, err := InitializePrecomputeValidators(t.Context(), s)
	require.NoError(t, err)
	_, balance, err = ProcessEpochParticipation(t.Context(), s, balance, validators)
	require.NoError(t, err)
	_, err = ProcessRewardsAndPenaltiesPrecompute(s, balance, []*precompute.Validator{})
	require.ErrorContains(t, "validator registries not the same length as state's validator registries", err)
}

func TestProcessInactivityScores_NonEligibleValidator(t *testing.T) {
	s, err := testState()
	require.NoError(t, err)
	defaultScore := uint64(5)
	require.NoError(t, s.SetInactivityScores([]uint64{defaultScore, defaultScore, defaultScore, defaultScore}))
	validators, balance, err := InitializePrecomputeValidators(t.Context(), s)
	require.NoError(t, err)

	// v0 is eligible (not active previous epoch, slashed and not withdrawable)
	validators[0].IsActivePrevEpoch = false
	validators[0].IsSlashed = true
	validators[0].IsWithdrawableCurrentEpoch = false

	// v1 is not eligible (not active previous epoch, not slashed and not withdrawable)
	validators[1].IsActivePrevEpoch = false
	validators[1].IsSlashed = false
	validators[1].IsWithdrawableCurrentEpoch = false

	// v2 is not eligible (not active previous epoch, slashed and withdrawable)
	validators[2].IsActivePrevEpoch = false
	validators[2].IsSlashed = true
	validators[2].IsWithdrawableCurrentEpoch = true

	// v3 is eligible (active previous epoch)
	validators[3].IsActivePrevEpoch = true

	validators, _, err = ProcessEpochParticipation(t.Context(), s, balance, validators)
	require.NoError(t, err)
	s, _, err = ProcessInactivityScores(t.Context(), s, validators)
	require.NoError(t, err)
	inactivityScores, err := s.InactivityScores()
	require.NoError(t, err)

	require.Equal(t, uint64(0), inactivityScores[0])
	require.Equal(t, defaultScore, inactivityScores[1]) // Should remain unchanged
	require.Equal(t, defaultScore, inactivityScores[2]) // Should remain unchanged
	require.Equal(t, uint64(0), inactivityScores[3])
}

func testState() (state.BeaconState, error) {
	generateParticipation := func(flags ...uint8) byte {
		b := byte(0)
		var err error
		for _, flag := range flags {
			b, err = AddValidatorFlag(b, flag)
			if err != nil {
				return 0
			}
		}
		return b
	}
	return state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Slot: 2 * params.BeaconConfig().SlotsPerEpoch,
		Validators: []*ethpb.Validator{
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance, ExitEpoch: params.BeaconConfig().FarFutureEpoch},
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance, ExitEpoch: params.BeaconConfig().FarFutureEpoch},
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance, ExitEpoch: params.BeaconConfig().FarFutureEpoch},
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance, ExitEpoch: params.BeaconConfig().FarFutureEpoch},
		},
		CurrentEpochParticipation: []byte{
			0,
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex),
		},
		PreviousEpochParticipation: []byte{
			0,
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex),
		},
		InactivityScores: []uint64{0, 0, 0, 0},
		Balances:         []uint64{0, 0, 0, 0},
	})
}

func testStateBellatrix() (state.BeaconState, error) {
	generateParticipation := func(flags ...uint8) byte {
		b := byte(0)
		var err error
		for _, flag := range flags {
			b, err = AddValidatorFlag(b, flag)
			if err != nil {
				return 0
			}
		}
		return b
	}
	return state_native.InitializeFromProtoBellatrix(&ethpb.BeaconStateBellatrix{
		Slot: 2 * params.BeaconConfig().SlotsPerEpoch,
		Validators: []*ethpb.Validator{
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance, ExitEpoch: params.BeaconConfig().FarFutureEpoch},
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance, ExitEpoch: params.BeaconConfig().FarFutureEpoch},
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance, ExitEpoch: params.BeaconConfig().FarFutureEpoch},
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance, ExitEpoch: params.BeaconConfig().FarFutureEpoch},
		},
		CurrentEpochParticipation: []byte{
			0,
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex),
		},
		PreviousEpochParticipation: []byte{
			0,
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex),
		},
		InactivityScores: []uint64{0, 0, 0, 0},
		Balances:         []uint64{0, 0, 0, 0},
	})
}

// helper: compute the mathematically correct head reward using big.Int
func expectedHeadRewardBig(
	effectiveBalance, activeCurrentEpoch, prevEpochHeadAttested uint64,
	baseRewardMultiplier, headWeight, weightDenominator, increment uint64,
) uint64 {
	// baseReward := (effectiveBalance / increment) * baseRewardMultiplier
	effDiv := new(big.Int).SetUint64(effectiveBalance / increment)
	brMul  := new(big.Int).SetUint64(baseRewardMultiplier)
	baseReward := new(big.Int).Mul(effDiv, brMul)

	attestedDiv := new(big.Int).SetUint64(prevEpochHeadAttested / increment)
	den := new(big.Int).Mul(
		new(big.Int).SetUint64(activeCurrentEpoch/increment),
		new(big.Int).SetUint64(weightDenominator),
	)

	num := new(big.Int).Mul(baseReward, new(big.Int).SetUint64(headWeight))
	num.Mul(num, attestedDiv)

	num.Quo(num, den) // floor division
	return num.Uint64()
}

func TestAttestationDelta_HeadRewardOverflow(t *testing.T) {
	cfg := params.BeaconConfig()

	// Mainnet-like constants (the test does not assume exact numbers; it reads them)
	increment := cfg.EffectiveBalanceIncrement       // typically 1e9 (gwei)
	weightDenominator := cfg.WeightDenominator       // typically 64
	headWeight := cfg.TimelyHeadWeight               // typically 14

	// Construct a pathological-but-valid config:
	// 1024 validators * 10,000,000 ETH effective balance
	effectiveBalance := uint64(10_000_000) * increment // 10,000,000 ETH in gwei
	activeCurrent := uint64(1024) * effectiveBalance
	prevHead := activeCurrent // assume full participation for simplicity

	// Validator flags to ensure we take the "reward" path (no penalties)
	val := &precompute.Validator{
		IsActivePrevEpoch:              true,
		IsSlashed:                      false,
		IsWithdrawableCurrentEpoch:     false,
		IsPrevEpochSourceAttester:      true,
		IsPrevEpochTargetAttester:      true,
		IsPrevEpochHeadAttester:        true,
		CurrentEpochEffectiveBalance:   effectiveBalance,
		InactivityScore:                0,
	}
	bal := &precompute.Balance{
		ActiveCurrentEpoch:      activeCurrent,
		PrevEpochAttested:       activeCurrent,
		PrevEpochTargetAttested: activeCurrent,
		PrevEpochHeadAttested:   prevHead,
	}

	// Use a big multiplier to trigger overflow (matches the report)
	const baseRewardMultiplier = uint64(200_000_000)
	const inactivityDenominator = uint64(1) // irrelevant since no penalty branch is taken
	const inactivityLeak = false

	// In the 'older' attestationDelta this would fail in the comparison later
	got, err := attestationDelta(
		bal, val,
		baseRewardMultiplier, inactivityDenominator,
		inactivityLeak,
	)
	if err != nil {
		t.Fatalf("attestationDelta error: %v", err)
	}

	// Compute mathematically correct head reward with big.Int for comparison
	want := expectedHeadRewardBig(
		effectiveBalance, activeCurrent, prevHead,
		baseRewardMultiplier, headWeight, weightDenominator, increment,
	)

	if got.HeadReward != want {
		t.Fatalf("HeadReward mismatch (possible overflow): got=%d want=%d", got.HeadReward, want)
	}
}

// Proves that simple reordering:
//   ((baseReward*headWeight)/weightDenominator) * (attested/increment) / activeIncrement
// still overflows the intermediate multiplication, and verifies that safeMul3Div2 handles it correctly.
func TestHeadReward_EarlyDivisionStillOverflows(t *testing.T) {
	cfg := params.BeaconConfig()
	increment := cfg.EffectiveBalanceIncrement   // typically 1e9
	weightDen := cfg.WeightDenominator           // typically 64
	headWeight := cfg.TimelyHeadWeight           // typically 14

	// Pathological but valid: 1024 validators @ 10,000,000 ETH effective balance
	effectiveBalance := uint64(10_000_000) * increment
	activeCurrent := uint64(1024) * effectiveBalance
	prevHead := activeCurrent 

	const baseRewardMultiplier = uint64(200_000_000)

	// Ground-truth (non-overflow) result using big.Int and the original single-flooring formula.
	want := expectedHeadRewardBig(
		effectiveBalance, activeCurrent, prevHead,
		baseRewardMultiplier, headWeight, weightDen, increment,
	)

	// Compute the pieces of the 'naive' formula.
	baseReward := (effectiveBalance / increment) * baseRewardMultiplier
	// Step 1: early divide by weightDenominator
	n1 := (baseReward * headWeight) / weightDen // this fits in uint64 for our numbers
	// Step 2: multiply by (prevHead/increment) — this is where overflow happens
	attDiv := prevHead / increment

	// Detect overflow explicitly.
	hi, lo := bits.Mul64(n1, attDiv)
	if hi == 0 {
		t.Fatalf("Expected overflow in n1*attDiv, but hi==0 (test ineffective). n1=%d attDiv=%d lo=%d", n1, attDiv, lo)
	}

	// Now actually do the naive formula in uint64 to show the wrong answer.
	nCorrected := (n1 * attDiv) / (activeCurrent / increment)

	if nCorrected == want {
		t.Fatalf("Early-division formula unexpectedly matched ground truth; adjust constants.")
	}
	
	// Verify that the naive approach fails
	t.Logf("Early-division overflows: got=%d want=%d (hi of n1*attDiv=%d)", nCorrected, want, hi)
	
	// Now verify that safeMul3Div2 handles it correctly
	att := prevHead / increment
	denA := activeCurrent / increment
	denB := weightDen
	got := safeMul3Div2(baseReward, headWeight, att, denA, denB)
	
	if got != want {
		t.Fatalf("safeMul3Div2 mismatch: got=%d want=%d", got, want)
	}
	t.Logf("safeMul3Div2 correctly computed: %d", got)
}