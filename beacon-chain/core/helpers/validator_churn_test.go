package helpers_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
)

func TestBalanceChurnLimit(t *testing.T) {
	tests := []struct {
		name          string
		activeBalance primitives.Gwei
		expected      primitives.Gwei
	}{
		{
			name:          "less than MIN_PER_EPOCH_CHURN_LIMIT_ELECTRA",
			activeBalance: 111,
			expected:      primitives.Gwei(params.BeaconConfig().MinPerEpochChurnLimitElectra),
		},
		{
			name:          "modulo EFFECTIVE_BALANCE_INCREMENT",
			activeBalance: primitives.Gwei(111 + params.BeaconConfig().MinPerEpochChurnLimitElectra*params.BeaconConfig().ChurnLimitQuotient),
			expected:      primitives.Gwei(params.BeaconConfig().MinPerEpochChurnLimitElectra),
		},
		{
			name:          "more than MIN_PER_EPOCH_CHURN_LIMIT_ELECTRA",
			activeBalance: primitives.Gwei(2000 * params.BeaconConfig().EffectiveBalanceIncrement * params.BeaconConfig().ChurnLimitQuotient),
			expected:      primitives.Gwei(2000 * params.BeaconConfig().EffectiveBalanceIncrement),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, helpers.BalanceChurnLimit(tt.activeBalance))
		})
	}
}

func TestActivationExitChurnLimit(t *testing.T) {
	tests := []struct {
		name          string
		activeBalance primitives.Gwei
		expected      primitives.Gwei
	}{
		{
			name:          "less than MAX_PER_EPOCH_ACTIVATION_EXIT_CHURN_LIMIT",
			activeBalance: 1,
			expected:      primitives.Gwei(params.BeaconConfig().MinPerEpochChurnLimitElectra),
		},
		{
			name:          "more than MAX_PER_EPOCH_ACTIVATION_EXIT_CHURN_LIMIT",
			activeBalance: primitives.Gwei(2000 * params.BeaconConfig().EffectiveBalanceIncrement * params.BeaconConfig().ChurnLimitQuotient),
			expected:      primitives.Gwei(params.BeaconConfig().MaxPerEpochActivationExitChurnLimit),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, helpers.ActivationExitChurnLimit(tt.activeBalance))
		})
	}
}

// FuzzConsolidationChurnLimit exercises BalanceChurnLimit and ActivationExitChurnLimit
func FuzzConsolidationChurnLimit(f *testing.F) {
	f.Fuzz(func(t *testing.T, activeBalance uint64) {
		helpers.ConsolidationChurnLimit(primitives.Gwei(activeBalance))
	})
}

func TestGloasChurnLimits_SlotDurationSchedule(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotDurationSchedule = params.SlotSchedule{
		params.SlotScheduleEntryForTest(0, 12000),
		params.SlotScheduleEntryForTest(10, 6000),
	}
	params.OverrideBeaconConfig(cfg)

	activeBalance := primitives.Gwei(cfg.MaxEffectiveBalance * 1e6)
	increment := primitives.Gwei(cfg.EffectiveBalanceIncrement)
	for _, tc := range []struct {
		name  string
		limit func(int, primitives.Gwei, primitives.Epoch) primitives.Gwei
	}{
		{"activation", helpers.ActivationChurnLimitForVersion},
		{"exit", helpers.ExitChurnLimitForVersion},
		{"consolidation", helpers.ConsolidationChurnLimitForVersion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.limit(version.Gloas, activeBalance, 9)
			after := tc.limit(version.Gloas, activeBalance, 10)
			assert.Equal(t, primitives.Gwei(0), before%increment)
			assert.Equal(t, primitives.Gwei(0), after%increment)
			assert.Equal(t, before/2-before/2%increment, after)
			// Pre-Gloas limits are not scaled.
			assert.Equal(t, tc.limit(version.Fulu, activeBalance, 9), tc.limit(version.Fulu, activeBalance, 10))
		})
	}
}
