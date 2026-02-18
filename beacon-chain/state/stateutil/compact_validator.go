package stateutil

import (
	"encoding/binary"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// CompactValidator is a fixed-size, pointer-free representation of a validator.
// It stores the same data as *ethpb.Validator but in a flat 128-byte struct
// with zero heap pointers, making it GC-friendly and cache-line efficient.
type CompactValidator struct {
	PublicKey                  [fieldparams.BLSPubkeyLength]byte // 48 bytes
	WithdrawalCredentials      [32]byte                          // 32 bytes
	EffectiveBalance           uint64                            // 8 bytes
	ActivationEligibilityEpoch primitives.Epoch                  // 8 bytes
	ActivationEpoch            primitives.Epoch                  // 8 bytes
	ExitEpoch                  primitives.Epoch                  // 8 bytes
	WithdrawableEpoch          primitives.Epoch                  // 8 bytes
	Slashed                    bool                              // 1 byte
	_                          [7]byte                           // 7 bytes padding = 128 total
}

// CompactValidatorFromProto converts a protobuf Validator to a CompactValidator.
func CompactValidatorFromProto(v *ethpb.Validator) CompactValidator {
	cv := CompactValidator{
		EffectiveBalance:           v.EffectiveBalance,
		ActivationEligibilityEpoch: v.ActivationEligibilityEpoch,
		ActivationEpoch:            v.ActivationEpoch,
		ExitEpoch:                  v.ExitEpoch,
		WithdrawableEpoch:          v.WithdrawableEpoch,
		Slashed:                    v.Slashed,
	}
	copy(cv.PublicKey[:], v.PublicKey)
	copy(cv.WithdrawalCredentials[:], v.WithdrawalCredentials)
	return cv
}

// ToProto converts a CompactValidator back to a protobuf Validator.
func (cv *CompactValidator) ToProto() *ethpb.Validator {
	pubKey := make([]byte, fieldparams.BLSPubkeyLength)
	copy(pubKey, cv.PublicKey[:])
	wc := make([]byte, 32)
	copy(wc, cv.WithdrawalCredentials[:])
	return &ethpb.Validator{
		PublicKey:                  pubKey,
		WithdrawalCredentials:      wc,
		EffectiveBalance:           cv.EffectiveBalance,
		ActivationEligibilityEpoch: cv.ActivationEligibilityEpoch,
		ActivationEpoch:            cv.ActivationEpoch,
		ExitEpoch:                  cv.ExitEpoch,
		WithdrawableEpoch:          cv.WithdrawableEpoch,
		Slashed:                    cv.Slashed,
	}
}

// CompactValidatorsFromProto converts a slice of protobuf Validators to CompactValidators.
func CompactValidatorsFromProto(vals []*ethpb.Validator) []CompactValidator {
	res := make([]CompactValidator, len(vals))
	for i, v := range vals {
		if v != nil {
			res[i] = CompactValidatorFromProto(v)
		}
	}
	return res
}

// CompactValidatorsToProto converts a slice of CompactValidators to protobuf Validators.
func CompactValidatorsToProto(cvs []CompactValidator) []*ethpb.Validator {
	res := make([]*ethpb.Validator, len(cvs))
	for i := range cvs {
		res[i] = cvs[i].ToProto()
	}
	return res
}

// CompactValidatorFieldRoots computes the field roots of a CompactValidator
// for hash tree root computation. This avoids pointer chasing and bounds checking
// present in the protobuf-based ValidatorFieldRoots.
func CompactValidatorFieldRoots(cv *CompactValidator) ([][32]byte, error) {
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

// CompactValidatorRootWithHasher computes the hash tree root of a CompactValidator.
func CompactValidatorRootWithHasher(cv *CompactValidator) ([32]byte, error) {
	fieldRoots, err := CompactValidatorFieldRoots(cv)
	if err != nil {
		return [32]byte{}, err
	}
	return ssz.BitwiseMerkleize(fieldRoots, uint64(len(fieldRoots)), uint64(len(fieldRoots)))
}
