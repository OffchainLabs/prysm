package issuance_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/issuance"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestBaseRewardFactorAtEpoch(t *testing.T) {
	require.Equal(t, params.BeaconConfig().BaseRewardFactor, issuance.BaseRewardFactorAtEpoch(0))

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.TransitionStartEpoch = 100
	cfg.TransitionDurationEpochs = 100
	params.OverrideBeaconConfig(cfg)

	require.Equal(t, cfg.BaseRewardFactor, issuance.BaseRewardFactorAtEpoch(99))
	require.Equal(t, cfg.TransitionBaseRewardFactor, issuance.BaseRewardFactorAtEpoch(100))
	require.Equal(t, uint64(96), issuance.BaseRewardFactorAtEpoch(150))
	require.Equal(t, cfg.BaseRewardFactor, issuance.BaseRewardFactorAtEpoch(200))
	require.Equal(t, cfg.BaseRewardFactor, issuance.BaseRewardFactorAtEpoch(1000))
}

func TestProcessBurn_Inactive(t *testing.T) {
	helpers.ClearCache()
	st, _ := util.DeterministicGenesisStateElectra(t, 64)
	before := st.Balances()
	require.NoError(t, issuance.ProcessBurn(t.Context(), st))
	require.DeepEqual(t, before, st.Balances())
}

func TestProcessBurn_Saturated(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.TransitionStartEpoch = 0
	// Far below total active balance so the burn fraction clamps to 1 and deductions equal ideal rewards.
	cfg.SaturationBalance = cfg.EffectiveBalanceIncrement
	params.OverrideBeaconConfig(cfg)
	helpers.ClearCache()

	st, _ := util.DeterministicGenesisStateElectra(t, 64)
	before := st.Balances()
	require.NoError(t, issuance.ProcessBurn(t.Context(), st))
	after := st.Balances()

	active, err := helpers.TotalActiveBalance(t.Context(), st)
	require.NoError(t, err)
	base, err := altair.BaseRewardPerIncrement(active, 0)
	require.NoError(t, err)
	attWeight := cfg.TimelySourceWeight + cfg.TimelyTargetWeight + cfg.TimelyHeadWeight
	attBurn := 32 * base * attWeight / cfg.WeightDenominator
	proposerBurn := active / cfg.EffectiveBalanceIncrement * base * cfg.ProposerWeight / cfg.WeightDenominator / uint64(cfg.SlotsPerEpoch)
	_, participantReward, err := altair.SyncRewards(active, 0)
	require.NoError(t, err)
	syncBurn := participantReward * uint64(cfg.SlotsPerEpoch)

	var burned uint64
	for i := range before {
		require.Equal(t, true, after[i] < before[i])
		burned += before[i] - after[i]
	}
	expected := 64*attBurn + uint64(cfg.SlotsPerEpoch)*proposerBurn + cfg.SyncCommitteeSize*syncBurn
	require.Equal(t, expected, burned)
}

func TestProcessBurn_BelowSaturation(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.TransitionStartEpoch = 0
	params.OverrideBeaconConfig(cfg)
	helpers.ClearCache()

	st, _ := util.DeterministicGenesisStateElectra(t, 64)
	before := st.Balances()
	require.NoError(t, issuance.ProcessBurn(t.Context(), st))
	after := st.Balances()

	// 2048 ETH active against a 60.25M ETH saturation balance burns less than one gwei per duty.
	require.DeepEqual(t, before, after)
}
