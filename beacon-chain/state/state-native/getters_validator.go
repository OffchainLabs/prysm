package state_native

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
)

// Validators participating in consensus on the beacon chain.
func (b *BeaconState) Validators() []*ethpb.Validator {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.validatorsVal()
}

// ValidatorsReadOnly participating in consensus on the beacon chain.
func (b *BeaconState) ValidatorsReadOnly() []state.ReadOnlyValidator {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.validatorsReadOnlyVal()
}

func (b *BeaconState) validatorsVal() []*ethpb.Validator {
	var v []*ethpb.Validator
	if b.validatorsMultiValue == nil {
		return nil
	}
	v = b.validatorsMultiValue.Value(b)

	res := make([]*ethpb.Validator, len(v))
	for i := 0; i < len(res); i++ {
		val := v[i]
		if val == nil {
			continue
		}
		res[i] = ethpb.CopyValidator(val)
	}
	return res
}

func (b *BeaconState) validatorsReadOnlyVal() []state.ReadOnlyValidator {
	if b.validatorsMultiValue == nil {
		return nil
	}
	v := b.validatorsMultiValue.Value(b)

	res := make([]state.ReadOnlyValidator, len(v))
	var err error
	for i := 0; i < len(res); i++ {
		val := v[i]
		if val == nil {
			continue
		}
		res[i], err = NewValidator(val)
		if err != nil {
			continue
		}
	}
	return res
}

func (b *BeaconState) validatorsLen() int {
	if b.validatorsMultiValue == nil {
		return 0
	}
	return b.validatorsMultiValue.Len(b)
}

// ValidatorAtIndex is the validator at the provided index.
func (b *BeaconState) ValidatorAtIndex(idx primitives.ValidatorIndex) (*ethpb.Validator, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.validatorAtIndex(idx)
}

func (b *BeaconState) validatorAtIndex(idx primitives.ValidatorIndex) (*ethpb.Validator, error) {
	if b.validatorsMultiValue == nil {
		return &ethpb.Validator{}, nil
	}
	v, err := b.validatorsMultiValue.At(b, uint64(idx))
	if err != nil {
		return nil, err
	}
	return ethpb.CopyValidator(v), nil
}

// ValidatorAtIndexReadOnly is the validator at the provided index. This method
// doesn't clone the validator.
func (b *BeaconState) ValidatorAtIndexReadOnly(idx primitives.ValidatorIndex) (state.ReadOnlyValidator, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.validatorAtIndexReadOnly(idx)
}

func (b *BeaconState) validatorAtIndexReadOnly(idx primitives.ValidatorIndex) (state.ReadOnlyValidator, error) {
	if b.validatorsMultiValue == nil {
		return nil, state.ErrNilValidatorsInState
	}
	v, err := b.validatorsMultiValue.At(b, uint64(idx))
	if err != nil {
		return nil, err
	}
	return NewValidator(v)
}

// ValidatorIndexByPubkey returns a given validator by its 48-byte public key.
func (b *BeaconState) ValidatorIndexByPubkey(key [fieldparams.BLSPubkeyLength]byte) (primitives.ValidatorIndex, bool) {
	if b == nil || b.valMapHandler == nil || b.valMapHandler.IsNil() {
		return 0, false
	}
	b.lock.RLock()
	defer b.lock.RUnlock()

	numOfVals := b.validatorsMultiValue.Len(b)

	idx, ok := b.valMapHandler.Get(key)
	if ok && primitives.ValidatorIndex(numOfVals) <= idx {
		return primitives.ValidatorIndex(0), false
	}
	return idx, ok
}

// PubkeyAtIndex returns the pubkey at the given
// validator index.
func (b *BeaconState) PubkeyAtIndex(idx primitives.ValidatorIndex) [fieldparams.BLSPubkeyLength]byte {
	b.lock.RLock()
	defer b.lock.RUnlock()

	v, err := b.validatorsMultiValue.At(b, uint64(idx))
	if err != nil {
		return [fieldparams.BLSPubkeyLength]byte{}
	}

	if v == nil {
		return [fieldparams.BLSPubkeyLength]byte{}
	}
	return bytesutil.ToBytes48(v.PublicKey)
}

// AggregateKeyFromIndices builds an aggregated public key from the provided
// validator indices.
func (b *BeaconState) AggregateKeyFromIndices(idxs []uint64) (bls.PublicKey, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	pubKeys := make([][]byte, len(idxs))
	for i, idx := range idxs {
		v, err := b.validatorsMultiValue.At(b, idx)
		if err != nil {
			return nil, err
		}
		if v == nil {
			return nil, ErrNilWrappedValidator
		}
		pubKeys[i] = v.PublicKey
	}
	return bls.AggregatePublicKeys(pubKeys)
}

// PublicKeys builds a list of all validator public keys, with each key's index aligned to its validator index.
func (b *BeaconState) PublicKeys() ([][fieldparams.BLSPubkeyLength]byte, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	l := b.validatorsLen()
	res := make([][fieldparams.BLSPubkeyLength]byte, l)
	for i := 0; i < l; i++ {
		val, err := b.validatorsMultiValue.At(b, uint64(i))
		if err != nil {
			return nil, err
		}
		copy(res[i][:], val.PublicKey)
	}
	return res, nil
}

// NumValidators returns the size of the validator registry.
func (b *BeaconState) NumValidators() int {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.validatorsLen()
}

// ReadFromEveryValidator reads values from every validator and applies it to the provided function.
//
// WARNING: This method is potentially unsafe, as it exposes the actual validator registry.
func (b *BeaconState) ReadFromEveryValidator(f func(idx int, val state.ReadOnlyValidator) error) error {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.validatorsMultiValue == nil {
		return state.ErrNilValidatorsInState
	}
	l := b.validatorsMultiValue.Len(b)
	for i := 0; i < l; i++ {
		v, err := b.validatorsMultiValue.At(b, uint64(i))
		if err != nil {
			return err
		}
		rov, err := NewValidator(v)
		if err != nil {
			return err
		}
		if err = f(i, rov); err != nil {
			return err
		}
	}
	return nil
}

// Balances of validators participating in consensus on the beacon chain.
func (b *BeaconState) Balances() []uint64 {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.balancesVal()
}

func (b *BeaconState) balancesVal() []uint64 {
	if b.balancesMultiValue == nil {
		return nil
	}
	return b.balancesMultiValue.Value(b)
}

// BalanceAtIndex of validator with the provided index.
func (b *BeaconState) BalanceAtIndex(idx primitives.ValidatorIndex) (uint64, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.balanceAtIndex(idx)
}

func (b *BeaconState) balanceAtIndex(idx primitives.ValidatorIndex) (uint64, error) {
	if b.balancesMultiValue == nil {
		return 0, nil
	}
	return b.balancesMultiValue.At(b, uint64(idx))
}

// BalancesLength returns the length of the balances slice.
func (b *BeaconState) BalancesLength() int {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.balancesMultiValue == nil {
		return 0
	}
	return b.balancesMultiValue.Len(b)
}

// Slashings of validators on the beacon chain.
func (b *BeaconState) Slashings() []uint64 {
	if b.slashings == nil {
		return nil
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.slashingsVal()
}

// slashingsVal of validators on the beacon chain.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) slashingsVal() []uint64 {
	if b.slashings == nil {
		return nil
	}

	res := make([]uint64, len(b.slashings))
	copy(res, b.slashings)
	return res
}

// InactivityScores of validators participating in consensus on the beacon chain.
func (b *BeaconState) InactivityScores() ([]uint64, error) {
	if b.version == version.Phase0 {
		return nil, errNotSupported("InactivityScores", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.inactivityScoresVal(), nil
}

func (b *BeaconState) inactivityScoresVal() []uint64 {
	if b.inactivityScoresMultiValue == nil {
		return nil
	}
	return b.inactivityScoresMultiValue.Value(b)
}

// PendingBalanceToWithdraw returns the sum of all pending withdrawals for the given validator.
//
// Spec definition:
//
//	def get_pending_balance_to_withdraw(state: BeaconState, validator_index: ValidatorIndex) -> Gwei:
//	    return sum(
//	        withdrawal.amount for withdrawal in state.pending_partial_withdrawals if withdrawal.index == validator_index)
func (b *BeaconState) PendingBalanceToWithdraw(idx primitives.ValidatorIndex) (uint64, error) {
	if b.version < version.Electra {
		return 0, errNotSupported("PendingBalanceToWithdraw", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	// TODO: Consider maintaining this value in the state, if it's a potential bottleneck.
	// This is n*m complexity, but this method can only be called
	// MAX_WITHDRAWAL_REQUESTS_PER_PAYLOAD per slot. A more optimized storage indexing such as a
	// lookup map could be used to reduce the complexity marginally.
	var sum uint64
	for _, w := range b.pendingPartialWithdrawals {
		if w.Index == idx {
			sum += w.Amount
		}
	}
	return sum, nil
}

func (b *BeaconState) HasPendingBalanceToWithdraw(idx primitives.ValidatorIndex) (bool, error) {
	if b.version < version.Electra {
		return false, errNotSupported("HasPendingBalanceToWithdraw", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	// TODO: Consider maintaining this value in the state, if it's a potential bottleneck.
	// This is n*m complexity, but this method can only be called
	// MAX_WITHDRAWAL_REQUESTS_PER_PAYLOAD per slot. A more optimized storage indexing such as a
	// lookup map could be used to reduce the complexity marginally.
	for _, w := range b.pendingPartialWithdrawals {
		if w.Index == idx && w.Amount > 0 {
			return true, nil
		}
	}

	return false, nil
}
