package query

import (
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestOptimizedContainerRootsMatchesValidatorRoots(t *testing.T) {
	validators := make([]*ethpb.Validator, 16)
	for i := range validators {
		validators[i] = makeTestValidator(i)
	}

	info, err := AnalyzeObject(validators[0])
	require.NoError(t, err)

	pc := newProofCollector()
	roots, err := OptimizedContainerRoots(info, reflect.ValueOf(validators), pc)
	require.NoError(t, err)

	expected, err := stateutil.OptimizedValidatorRoots(validators)
	require.NoError(t, err)

	require.Equal(t, len(expected), len(roots))
	for i := range expected {
		require.Equal(t, expected[i], roots[i])
	}
}

func BenchmarkOptimizedContainerRoots(b *testing.B) {
	validators := make([]*ethpb.Validator, 1000)
	for i := range validators {
		validators[i] = makeTestValidator(i)
	}

	info, err := AnalyzeObject(validators[0])
	require.NoError(b, err)

	pc := newProofCollector()
	v := reflect.ValueOf(validators)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := OptimizedContainerRoots(info, v, pc)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOptimizedValidatorRoots(b *testing.B) {
	validators := make([]*ethpb.Validator, 1000)
	for i := range validators {
		validators[i] = makeTestValidator(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := stateutil.OptimizedValidatorRoots(validators)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProofCollectorMerkleize(b *testing.B) {
	validators := make([]*ethpb.Validator, 1000)
	for i := range validators {
		validators[i] = makeTestValidator(i)
	}

	info, err := AnalyzeObject(validators[0])
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, val := range validators {
			pc := newProofCollector()
			v := reflect.ValueOf(val)
			_, err := pc.merkleize(info, v, 1)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func makeTestValidator(i int) *ethpb.Validator {
	pubkey := make([]byte, 48)
	for j := range pubkey {
		pubkey[j] = byte(i + j)
	}

	withdrawalCredentials := make([]byte, 32)
	for j := range withdrawalCredentials {
		withdrawalCredentials[j] = byte(255 - ((i + j) % 256))
	}

	return &ethpb.Validator{
		PublicKey:                  pubkey,
		WithdrawalCredentials:      withdrawalCredentials,
		EffectiveBalance:           uint64(32000000000 + i),
		Slashed:                    i%2 == 0,
		ActivationEligibilityEpoch: primitives.Epoch(i),
		ActivationEpoch:            primitives.Epoch(i + 1),
		ExitEpoch:                  primitives.Epoch(i + 2),
		WithdrawableEpoch:          primitives.Epoch(i + 3),
	}
}
