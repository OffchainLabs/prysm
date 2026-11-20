package stateutil

import (
	"math"
	"math/rand"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestCompactValidator(t *testing.T) {
	vals := randomCompactValidators(rand.New(rand.NewSource(2)), 256)

	// Maximum value for the public key.
	var ones [48]byte
	for i := range ones {
		ones[i] = 0xff
	}

	vals = append(vals,
		// All fields zero.
		CompactValidator{},
		// All fields at their maximum value.
		CompactValidator{PublicKey: ones, Slashed: true, EffectiveBalance: math.MaxUint64,
			ActivationEligibilityEpoch: math.MaxUint64, ActivationEpoch: math.MaxUint64,
			ExitEpoch: math.MaxUint64, WithdrawableEpoch: math.MaxUint64},
	)

	t.Run("proto round trip", func(t *testing.T) {
		for i, v := range vals {
			require.Equal(t, v, CompactValidatorFromProto(v.ToProto()), "validator %d", i)
		}
	})

	t.Run("field roots match proto", func(t *testing.T) {
		for i, v := range vals {
			want, err := ValidatorFieldRoots(v.ToProto())
			require.NoError(t, err)

			// Prefill dst to check that every byte is written.
			var dst [validatorFieldRoots][32]byte
			for k := range dst {
				for b := range dst[k] {
					dst[k][b] = 0xee
				}
			}

			v.fieldRootsInto(&dst)
			require.DeepEqual(t, want, dst[:], "validator %d", i)
		}
	})

	t.Run("root matches proto", func(t *testing.T) {
		for i, v := range vals {
			want, err := ValidatorRootWithHasher(v.ToProto())
			require.NoError(t, err)
			got, err := v.Root()
			require.NoError(t, err)
			require.Equal(t, want, got, "validator %d", i)
		}
	})
}

func randomCompactValidators(r *rand.Rand, n int) []CompactValidator {
	vals := make([]CompactValidator, n)
	for i := range vals {
		v := &vals[i]
		r.Read(v.PublicKey[:])
		r.Read(v.WithdrawalCredentials[:])
		v.EffectiveBalance = r.Uint64()
		v.Slashed = r.Intn(2) == 0
		v.ActivationEligibilityEpoch = primitives.Epoch(r.Uint64())
		v.ActivationEpoch = primitives.Epoch(r.Uint64())
		v.ExitEpoch = primitives.Epoch(r.Uint64())
		v.WithdrawableEpoch = primitives.Epoch(r.Uint64())
	}
	return vals
}
