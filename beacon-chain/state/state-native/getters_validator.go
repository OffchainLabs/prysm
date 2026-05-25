package state_native

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
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
	if b.validatorsMultiValue == nil {
		return nil
	}
	v := b.validatorsMultiValue.Value(b)
	return stateutil.CompactValidatorsToProto(v)
}

func (b *BeaconState) validatorsReadOnlyVal() []state.ReadOnlyValidator {
	if b.validatorsMultiValue == nil {
		return nil
	}
	v := b.validatorsMultiValue.Value(b)

	res := make([]state.ReadOnlyValidator, len(v))
	for i := range res {
		res[i] = NewValidatorFromCompact(v[i])
	}
	return res
}

// validatorsCompactVal returns the raw compact validator slice for internal use (hashing).
func (b *BeaconState) validatorsCompactVal() []stateutil.CompactValidator {
	if b.validatorsMultiValue == nil {
		return nil
	}
	return b.validatorsMultiValue.Value(b)
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

// EffectiveBalances returns the sum of the effective balances of the given list of validator indices, the eb of each given validator, or an
// error if one of the indices is out of bounds, or the state wasn't correctly initialized.
func (b *BeaconState) EffectiveBalanceSum(idxs []primitives.ValidatorIndex) (uint64, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()
	var sum uint64
	for i := range idxs {
		if b.validatorsMultiValue == nil {
			return 0, errors.Wrap(state.ErrNilValidatorsInState, "nil validators multi-value slice")
		}
		v, err := b.validatorsMultiValue.At(b, uint64(idxs[i]))
		if err != nil {
			return 0, errors.Wrap(err, "validators multi value at index")
		}
		sum += v.EffectiveBalance
	}
	return sum, nil
}

// EffectiveBalanceAtIndex returns the effective balance of the validator at the given index
// without materializing a Validator struct.
func (b *BeaconState) EffectiveBalanceAtIndex(idx primitives.ValidatorIndex) (uint64, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()
	if b.validatorsMultiValue == nil {
		return 0, state.ErrNilValidatorsInState
	}
	v, err := b.validatorsMultiValue.At(b, uint64(idx))
	if err != nil {
		return 0, err
	}
	return v.EffectiveBalance, nil
}

func (b *BeaconState) validatorAtIndex(idx primitives.ValidatorIndex) (*ethpb.Validator, error) {
	if b.validatorsMultiValue == nil {
		return &ethpb.Validator{}, nil
	}
	v, err := b.validatorsMultiValue.At(b, uint64(idx))
	if err != nil {
		return nil, err
	}
	return v.ToProto(), nil
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
	return NewValidatorFromCompact(v), nil
}

// ValidatorIndexByPubkey returns a given validator by its 48-byte public key.
func (b *BeaconState) ValidatorIndexByPubkey(key [fieldparams.BLSPubkeyLength]byte) (primitives.ValidatorIndex, bool) {
	if b == nil || b.valMapHandler == nil || b.valMapHandler.IsNil() {
		return 0, false
	}
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.validatorIndexByPubkey(key)
}

// Lock free version of ValidatorIndexByPubkey. This assumes that a lock is already held on BeaconState.
func (b *BeaconState) validatorIndexByPubkey(key [fieldparams.BLSPubkeyLength]byte) (primitives.ValidatorIndex, bool) {
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

	return v.PublicKey
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
		pubKeys[i] = v.PublicKey[:]
	}
	return bls.AggregatePublicKeys(pubKeys)
}

// PublicKeys builds a list of all validator public keys, with each key's index aligned to its validator index.
func (b *BeaconState) PublicKeys() ([][fieldparams.BLSPubkeyLength]byte, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	l := b.validatorsLen()
	res := make([][fieldparams.BLSPubkeyLength]byte, l)
	for i := range l {
		val, err := b.validatorsMultiValue.At(b, uint64(i))
		if err != nil {
			return nil, err
		}
		res[i] = val.PublicKey
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
func (b *BeaconState) ReadFromEveryValidator(f func(idx primitives.ValidatorIndex, val state.ReadOnlyValidator) error) error {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.validatorsMultiValue == nil {
		return state.ErrNilValidatorsInState
	}
	l := b.validatorsMultiValue.Len(b)
	for i := range l {
		v, err := b.validatorsMultiValue.At(b, uint64(i))
		if err != nil {
			return err
		}
		rov := NewValidatorFromCompact(v)
		if err = f(primitives.ValidatorIndex(i), rov); err != nil {
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
