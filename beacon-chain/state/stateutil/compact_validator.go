package stateutil

import (
	"encoding/binary"
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// CompactValidator is a fixed-size, pointer-free representation of a validator.
// It stores the same data as *ethpb.Validator but in a flat 128-byte struct
// with zero heap pointers.
// (The used size is 121 bytes, but the struct is padded to 128 bytes for alignment.)
type CompactValidator struct {
	PublicKey                  [fieldparams.BLSPubkeyLength]byte // 48 bytes
	WithdrawalCredentials      [32]byte                          // 32 bytes
	EffectiveBalance           uint64                            // 8 bytes
	Slashed                    bool                              // 1 byte
	ActivationEligibilityEpoch primitives.Epoch                  // 8 bytes
	ActivationEpoch            primitives.Epoch                  // 8 bytes
	ExitEpoch                  primitives.Epoch                  // 8 bytes
	WithdrawableEpoch          primitives.Epoch                  // 8 bytes
}

// CompactValidatorsFromProto converts a slice of protobuf Validators to CompactValidators.
func CompactValidatorsFromProto(validators []*ethpb.Validator) []CompactValidator {
	compactValidators := make([]CompactValidator, len(validators))
	for i, validator := range validators {
		if validator != nil {
			compactValidators[i] = CompactValidatorFromProto(validator)
		}
	}

	return compactValidators
}

// CompactValidatorsToProto converts a slice of CompactValidators to protobuf Validators.
func CompactValidatorsToProto(compactValidators []CompactValidator) []*ethpb.Validator {
	validators := make([]*ethpb.Validator, len(compactValidators))
	for i := range compactValidators {
		validators[i] = compactValidators[i].ToProto()
	}

	return validators
}

// CompactValidatorFromProto converts a protobuf Validator to a CompactValidator.
func CompactValidatorFromProto(v *ethpb.Validator) CompactValidator {
	var publicKey [fieldparams.BLSPubkeyLength]byte
	copy(publicKey[:], v.PublicKey)

	var withdrawalCredentials [32]byte
	copy(withdrawalCredentials[:], v.WithdrawalCredentials)

	return CompactValidator{
		PublicKey:                  publicKey,
		WithdrawalCredentials:      withdrawalCredentials,
		EffectiveBalance:           v.EffectiveBalance,
		ActivationEligibilityEpoch: v.ActivationEligibilityEpoch,
		ActivationEpoch:            v.ActivationEpoch,
		ExitEpoch:                  v.ExitEpoch,
		WithdrawableEpoch:          v.WithdrawableEpoch,
		Slashed:                    v.Slashed,
	}
}

// ToProto converts a CompactValidator back to a protobuf Validator.
func (cv *CompactValidator) ToProto() *ethpb.Validator {
	publicKey := make([]byte, fieldparams.BLSPubkeyLength)
	copy(publicKey, cv.PublicKey[:])

	withdrawalCredentials := make([]byte, 32)
	copy(withdrawalCredentials, cv.WithdrawalCredentials[:])

	return &ethpb.Validator{
		PublicKey:                  publicKey,
		WithdrawalCredentials:      withdrawalCredentials,
		EffectiveBalance:           cv.EffectiveBalance,
		ActivationEligibilityEpoch: cv.ActivationEligibilityEpoch,
		ActivationEpoch:            cv.ActivationEpoch,
		ExitEpoch:                  cv.ExitEpoch,
		WithdrawableEpoch:          cv.WithdrawableEpoch,
		Slashed:                    cv.Slashed,
	}
}

// HasETH1WithdrawalCredentials returns true if the validator has ETH1 withdrawal credentials.
func (cv *CompactValidator) HasETH1WithdrawalCredentials() bool {
	return cv.WithdrawalCredentials[0] == params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
}

// HasCompoundingWithdrawalCredentials returns true if the validator has compounding withdrawal credentials.
func (cv *CompactValidator) HasCompoundingWithdrawalCredentials() bool {
	return cv.WithdrawalCredentials[0] == params.BeaconConfig().CompoundingWithdrawalPrefixByte
}

// HasExecutionWithdrawalCredentials returns true if the validator has execution withdrawal credentials.
func (cv *CompactValidator) HasExecutionWithdrawalCredentials() bool {
	return cv.HasETH1WithdrawalCredentials() || cv.HasCompoundingWithdrawalCredentials()
}

// Root computes the hash tree root of a CompactValidator.
func (cv *CompactValidator) Root() ([32]byte, error) {
	fieldRoots, err := cv.fieldRoots()
	if err != nil {
		return [32]byte{}, fmt.Errorf("fieldRoots: %w", err)
	}

	return ssz.BitwiseMerkleize(fieldRoots, uint64(len(fieldRoots)), uint64(len(fieldRoots)))
}

// fieldRoots computes the field roots of a CompactValidator
// for hash tree root computation.
func (cv *CompactValidator) fieldRoots() ([][32]byte, error) {
	// Public key root (merkleize 48-byte pubkey).
	pubKeyRoot, err := merkleizePubkey(cv.PublicKey[:])
	if err != nil {
		return nil, err
	}

	// Withdrawal credentials (already 32 bytes).
	withdrawCreds := cv.WithdrawalCredentials

	// Effective balance.
	var effectiveBalanceBuf [32]byte
	binary.LittleEndian.PutUint64(effectiveBalanceBuf[:8], cv.EffectiveBalance)

	// Slashed.
	var slashBuf [32]byte
	if cv.Slashed {
		slashBuf[0] = 1
	}

	// Activation eligibility epoch.
	var activationEligibilityBuf [32]byte
	binary.LittleEndian.PutUint64(activationEligibilityBuf[:8], uint64(cv.ActivationEligibilityEpoch))

	// Activation epoch.
	var activationBuf [32]byte
	binary.LittleEndian.PutUint64(activationBuf[:8], uint64(cv.ActivationEpoch))

	// Exit epoch.
	var exitBuf [32]byte
	binary.LittleEndian.PutUint64(exitBuf[:8], uint64(cv.ExitEpoch))

	// Withdrawable epoch.
	var withdrawalBuf [32]byte
	binary.LittleEndian.PutUint64(withdrawalBuf[:8], uint64(cv.WithdrawableEpoch))

	return [][32]byte{
		pubKeyRoot, withdrawCreds, effectiveBalanceBuf, slashBuf,
		activationEligibilityBuf, activationBuf, exitBuf, withdrawalBuf,
	}, nil
}
