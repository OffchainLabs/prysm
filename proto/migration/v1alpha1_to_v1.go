package migration

import (
	ethpbv1 "github.com/prysmaticlabs/prysm/v5/proto/eth/v1"
	ethpbalpha "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// V1Alpha1SignedHeaderToV1 converts a v1alpha1 signed beacon block header to v1.
func V1Alpha1SignedHeaderToV1(v1alpha1Hdr *ethpbalpha.SignedBeaconBlockHeader) *ethpbv1.SignedBeaconBlockHeader {
	if v1alpha1Hdr == nil || v1alpha1Hdr.Header == nil {
		return &ethpbv1.SignedBeaconBlockHeader{}
	}
	return &ethpbv1.SignedBeaconBlockHeader{
		Message:   V1Alpha1HeaderToV1(v1alpha1Hdr.Header),
		Signature: v1alpha1Hdr.Signature,
	}
}

// V1Alpha1HeaderToV1 converts a v1alpha1 beacon block header to v1.
func V1Alpha1HeaderToV1(v1alpha1Hdr *ethpbalpha.BeaconBlockHeader) *ethpbv1.BeaconBlockHeader {
	if v1alpha1Hdr == nil {
		return &ethpbv1.BeaconBlockHeader{}
	}

	return &ethpbv1.BeaconBlockHeader{
		Slot:          v1alpha1Hdr.Slot,
		ProposerIndex: v1alpha1Hdr.ProposerIndex,
		ParentRoot:    v1alpha1Hdr.ParentRoot,
		StateRoot:     v1alpha1Hdr.StateRoot,
		BodyRoot:      v1alpha1Hdr.BodyRoot,
	}
}

// V1HeaderToV1Alpha1 converts a v1 beacon block header to v1alpha1.
func V1HeaderToV1Alpha1(v1Header *ethpbv1.BeaconBlockHeader) *ethpbalpha.BeaconBlockHeader {
	if v1Header == nil {
		return &ethpbalpha.BeaconBlockHeader{}
	}
	return &ethpbalpha.BeaconBlockHeader{
		Slot:          v1Header.Slot,
		ProposerIndex: v1Header.ProposerIndex,
		ParentRoot:    v1Header.ParentRoot,
		StateRoot:     v1Header.StateRoot,
		BodyRoot:      v1Header.BodyRoot,
	}
}

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
