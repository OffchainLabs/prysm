package helpers_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
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
