package stateutil

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func validator(slashed bool) *ethpb.Validator {
	return &ethpb.Validator{
		PublicKey:                  []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48},
		WithdrawalCredentials:      []byte{0xaa, 0xbb, 0xcc, 0xdd, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff},
		EffectiveBalance:           32000000000,
		Slashed:                    slashed,
		ActivationEligibilityEpoch: 10,
		ActivationEpoch:            20,
		ExitEpoch:                  primitives.Epoch(30),
		WithdrawableEpoch:          primitives.Epoch(40),
	}
}

func TestCompactValidator(t *testing.T) {
	t.Run("ProtoRoundTrip", func(t *testing.T) {
		for _, slashed := range []bool{true, false} {
			validatorStart := validator(slashed)
			compactValidator := CompactValidatorFromProto(validatorStart)
			validatorEnd := compactValidator.ToProto()

			assert.DeepEqual(t, validatorStart.PublicKey, validatorEnd.PublicKey)
			assert.DeepEqual(t, validatorStart.WithdrawalCredentials, validatorEnd.WithdrawalCredentials)
			assert.Equal(t, validatorStart.EffectiveBalance, validatorEnd.EffectiveBalance)
			assert.Equal(t, validatorStart.Slashed, validatorEnd.Slashed)
			assert.Equal(t, validatorStart.ActivationEligibilityEpoch, validatorEnd.ActivationEligibilityEpoch)
			assert.Equal(t, validatorStart.ActivationEpoch, validatorEnd.ActivationEpoch)
			assert.Equal(t, validatorStart.ExitEpoch, validatorEnd.ExitEpoch)
			assert.Equal(t, validatorStart.WithdrawableEpoch, validatorEnd.WithdrawableEpoch)
		}
	})

	t.Run("RootMatchesProto", func(t *testing.T) {
		validator := validator(true)

		protoRoot, err := ValidatorRootWithHasher(validator)
		require.NoError(t, err)

		compactValidator := CompactValidatorFromProto(validator)
		compactRoot, err := compactValidator.Root()
		require.NoError(t, err)

		assert.Equal(t, protoRoot, compactRoot)
	})

	t.Run("FieldRootsMatchProto", func(t *testing.T) {
		v := validator(true)

		protoFieldRoots, err := ValidatorFieldRoots(v)
		require.NoError(t, err)

		compactValidator := CompactValidatorFromProto(v)
		compactFieldRoots, err := compactValidator.fieldRoots()
		require.NoError(t, err)

		require.Equal(t, len(protoFieldRoots), len(compactFieldRoots))
		for i := range protoFieldRoots {
			assert.Equal(t, protoFieldRoots[i], compactFieldRoots[i], "field root mismatch at index %d", i)
		}
	})
}
