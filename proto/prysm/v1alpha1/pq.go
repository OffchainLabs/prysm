package eth

import (
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
)

func (v *ValidatorPQ) Copy() *ValidatorPQ {
	if v == nil {
		return nil
	}
	return &ValidatorPQ{
		PublicKey:                  bytesutil.SafeCopyBytes(v.PublicKey),
		VrfRoot:                    bytesutil.SafeCopyBytes(v.VrfRoot),
		WithdrawalCredentials:      bytesutil.SafeCopyBytes(v.WithdrawalCredentials),
		EffectiveBalance:           v.EffectiveBalance,
		Slashed:                    v.Slashed,
		ActivationEligibilityEpoch: v.ActivationEligibilityEpoch,
		ActivationEpoch:            v.ActivationEpoch,
		ExitEpoch:                  v.ExitEpoch,
		WithdrawableEpoch:          v.WithdrawableEpoch,
	}
}

func (b *BuilderPQ) Copy() *BuilderPQ {
	if b == nil {
		return nil
	}
	return &BuilderPQ{
		Pubkey:            bytesutil.SafeCopyBytes(b.Pubkey),
		Version:           bytesutil.SafeCopyBytes(b.Version),
		ExecutionAddress:  bytesutil.SafeCopyBytes(b.ExecutionAddress),
		Balance:           b.Balance,
		DepositEpoch:      b.DepositEpoch,
		WithdrawableEpoch: b.WithdrawableEpoch,
	}
}

func (p *ValidatorPreregistration) Copy() *ValidatorPreregistration {
	if p == nil {
		return nil
	}
	return &ValidatorPreregistration{
		Pubkey:                bytesutil.SafeCopyBytes(p.Pubkey),
		WithdrawalCredentials: bytesutil.SafeCopyBytes(p.WithdrawalCredentials),
		VrfRoot:               bytesutil.SafeCopyBytes(p.VrfRoot),
		ExpirySlot:            p.ExpirySlot,
	}
}

func (p *BuilderPreregistration) Copy() *BuilderPreregistration {
	if p == nil {
		return nil
	}
	return &BuilderPreregistration{
		Pubkey:                bytesutil.SafeCopyBytes(p.Pubkey),
		WithdrawalCredentials: bytesutil.SafeCopyBytes(p.WithdrawalCredentials),
		ExpirySlot:            p.ExpirySlot,
	}
}
