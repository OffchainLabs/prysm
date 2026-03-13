package state_native

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

var (
	// ErrNilWrappedValidator returns when caller attempts to wrap a nil pointer validator.
	ErrNilWrappedValidator = errors.New("nil validator cannot be wrapped as readonly")
)

// readOnlyValidator returns a wrapper that only allows fields from a validator
// to be read, and prevents any modification of internal validator fields.
type readOnlyValidator struct {
	validator stateutil.CompactValidator
}

var _ = state.ReadOnlyValidator(readOnlyValidator{})

// NewValidator initializes the read only wrapper for validator from a proto.
func NewValidator(v *ethpb.Validator) (state.ReadOnlyValidator, error) {
	if v == nil {
		return nil, ErrNilWrappedValidator
	}
	rov := readOnlyValidator{
		validator: stateutil.CompactValidatorFromProto(v),
	}
	return rov, nil
}

// NewValidatorFromCompact initializes the read only wrapper from a CompactValidator.
func NewValidatorFromCompact(cv stateutil.CompactValidator) state.ReadOnlyValidator {
	return readOnlyValidator{
		validator: cv,
	}
}

// EffectiveBalance returns the effective balance of the
// read only validator.
func (v readOnlyValidator) EffectiveBalance() uint64 {
	return v.validator.EffectiveBalance
}

// ActivationEligibilityEpoch returns the activation eligibility epoch of the
// read only validator.
func (v readOnlyValidator) ActivationEligibilityEpoch() primitives.Epoch {
	return v.validator.ActivationEligibilityEpoch
}

// ActivationEpoch returns the activation epoch of the
// read only validator.
func (v readOnlyValidator) ActivationEpoch() primitives.Epoch {
	return v.validator.ActivationEpoch
}

// WithdrawableEpoch returns the withdrawable epoch of the
// read only validator.
func (v readOnlyValidator) WithdrawableEpoch() primitives.Epoch {
	return v.validator.WithdrawableEpoch
}

// ExitEpoch returns the exit epoch of the
// read only validator.
func (v readOnlyValidator) ExitEpoch() primitives.Epoch {
	return v.validator.ExitEpoch
}

// PublicKey returns the public key of the
// read only validator.
func (v readOnlyValidator) PublicKey() [fieldparams.BLSPubkeyLength]byte {
	return v.validator.PublicKey
}

// WithdrawalCredentials returns the withdrawal credentials of the
// read only validator.
func (v readOnlyValidator) GetWithdrawalCredentials() []byte {
	creds := make([]byte, 32)
	copy(creds, v.validator.WithdrawalCredentials[:])
	return creds
}

// Slashed returns the read only validator is slashed.
func (v readOnlyValidator) Slashed() bool {
	return v.validator.Slashed
}

// HasETH1WithdrawalCredentials returns true if the validator has an ETH1 withdrawal credentials.
func (v readOnlyValidator) HasETH1WithdrawalCredentials() bool {
	return v.validator.WithdrawalCredentials[0] == params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
}

// HasCompoundingWithdrawalCredentials returns true if the validator has a compounding withdrawal credentials.
func (v readOnlyValidator) HasCompoundingWithdrawalCredentials() bool {
	return v.validator.WithdrawalCredentials[0] == params.BeaconConfig().CompoundingWithdrawalPrefixByte
}

// HasExecutionWithdrawalCredentials returns true if the validator has an execution withdrawal credentials.
func (v readOnlyValidator) HasExecutionWithdrawalCredentials() bool {
	return v.HasETH1WithdrawalCredentials() || v.HasCompoundingWithdrawalCredentials()
}

// Copy returns a new validator from the read only validator
func (v readOnlyValidator) Copy() *ethpb.Validator {
	return v.validator.ToProto()
}
