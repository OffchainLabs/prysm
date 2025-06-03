package migration

import (
	ethpbv1 "github.com/OffchainLabs/prysm/v6/proto/eth/v1"
	ethpbalpha "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

// V1ValidatorToV1Alpha1 converts a v1 validator to v1alpha1.
func V1ValidatorToV1Alpha1(v1Validator *ethpbv1.Validator) *ethpbalpha.Validator {
	if v1Validator == nil {
		return &ethpbalpha.Validator{}
	}
	return &ethpbalpha.Validator{
		PublicKey:                  v1Validator.Pubkey,
		WithdrawalCredentials:      v1Validator.WithdrawalCredentials,
		EffectiveBalance:           v1Validator.EffectiveBalance,
		Slashed:                    v1Validator.Slashed,
		ActivationEligibilityEpoch: v1Validator.ActivationEligibilityEpoch,
		ActivationEpoch:            v1Validator.ActivationEpoch,
		ExitEpoch:                  v1Validator.ExitEpoch,
		WithdrawableEpoch:          v1Validator.WithdrawableEpoch,
	}
}
