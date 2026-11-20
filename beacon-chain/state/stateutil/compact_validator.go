package stateutil

import (
	"encoding/binary"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/hash/htr"
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

// Root computes the hash tree root of a CompactValidator.
func (cv *CompactValidator) Root() ([32]byte, error) {
	// Allocate a 2D fixed byte array per validator.
	var dst [validatorFieldRoots][32]byte
	cv.fieldRootsInto(&dst)
	return ssz.BitwiseMerkleize(dst[:], validatorFieldRoots, validatorFieldRoots)
}

// fieldRootsInto writes the field roots of cv into dst.
// Receive a pointer of array to avoid extra allocations, and check the length of the array as well.
func (cv *CompactValidator) fieldRootsInto(dst *[validatorFieldRoots][32]byte) {
	// Pubkey root, using dst[0:3] as scratch:
	//
	//   dst[0]        dst[1]               dst[2]
	//   [pk[0:32]]    [pk[32:48] | 0..0]   [ ]             1. copy pubkey
	//   [pk[0:32]]    [pk[32:48] | 0..0]   [H(dst[0:2])]   2. hash
	//   [pkRoot  ]    [withdrawal creds]   [eff. balance]  3. final

	// 1. Copy pubkey.
	copy(dst[0][:], cv.PublicKey[:32])
	dst[1] = [32]byte{}
	copy(dst[1][:], cv.PublicKey[32:])

	// 2. Hash the pubkey chunks into dst[2].
	htr.HashChunks(dst[2:3], dst[0:2])

	// 3. Copy the pubkey root to dst[0] for the final layout.
	dst[0] = dst[2]

	// From here on, fill in the remaining fields.
	dst[1] = cv.WithdrawalCredentials
	for i := 2; i < validatorFieldRoots; i++ {
		dst[i] = [32]byte{}
	}
	binary.LittleEndian.PutUint64(dst[2][:8], cv.EffectiveBalance)
	if cv.Slashed {
		dst[3][0] = 1
	}
	binary.LittleEndian.PutUint64(dst[4][:8], uint64(cv.ActivationEligibilityEpoch))
	binary.LittleEndian.PutUint64(dst[5][:8], uint64(cv.ActivationEpoch))
	binary.LittleEndian.PutUint64(dst[6][:8], uint64(cv.ExitEpoch))
	binary.LittleEndian.PutUint64(dst[7][:8], uint64(cv.WithdrawableEpoch))
}
