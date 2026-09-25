package state_native_test

import (
	"runtime"
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	statenative "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	testtmpl "github.com/OffchainLabs/prysm/v7/beacon-chain/state/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestBeaconState_ValidatorAtIndexReadOnly_HandlesNilSlice_Phase0(t *testing.T) {
	testtmpl.VerifyBeaconStateValidatorAtIndexReadOnlyHandlesNilSlice(t, func() (state.BeaconState, error) {
		return statenative.InitializeFromProtoUnsafePhase0(&ethpb.BeaconState{
			Validators: nil,
		})
	})
}

func TestBeaconState_ValidatorAtIndexReadOnly_HandlesNilSlice_Altair(t *testing.T) {
	testtmpl.VerifyBeaconStateValidatorAtIndexReadOnlyHandlesNilSlice(t, func() (state.BeaconState, error) {
		return statenative.InitializeFromProtoUnsafeAltair(&ethpb.BeaconStateAltair{
			Validators: nil,
		})
	})
}

func TestBeaconState_ValidatorAtIndexReadOnly_HandlesNilSlice_Bellatrix(t *testing.T) {
	testtmpl.VerifyBeaconStateValidatorAtIndexReadOnlyHandlesNilSlice(t, func() (state.BeaconState, error) {
		return statenative.InitializeFromProtoUnsafeBellatrix(&ethpb.BeaconStateBellatrix{
			Validators: nil,
		})
	})
}

func TestBeaconState_ValidatorAtIndexReadOnly_HandlesNilSlice_Capella(t *testing.T) {
	testtmpl.VerifyBeaconStateValidatorAtIndexReadOnlyHandlesNilSlice(t, func() (state.BeaconState, error) {
		return statenative.InitializeFromProtoUnsafeCapella(&ethpb.BeaconStateCapella{
			Validators: nil,
		})
	})
}

func TestBeaconState_ValidatorAtIndexReadOnly_HandlesNilSlice_Deneb(t *testing.T) {
	testtmpl.VerifyBeaconStateValidatorAtIndexReadOnlyHandlesNilSlice(t, func() (state.BeaconState, error) {
		return statenative.InitializeFromProtoUnsafeDeneb(&ethpb.BeaconStateDeneb{
			Validators: nil,
		})
	})
}

func TestValidatorIndexes(t *testing.T) {
	dState, _ := util.DeterministicGenesisState(t, 10)
	byteValue := dState.PubkeyAtIndex(1)
	t.Run("ValidatorIndexByPubkey", func(t *testing.T) {
		require.Equal(t, hexutil.Encode(byteValue[:]), "0xb89bebc699769726a318c8e9971bd3171297c61aea4a6578a7a4f94b547dcba5bac16a89108b6b6a1fe3695d1a874a0b")
	})
	t.Run("ValidatorAtIndexReadOnly", func(t *testing.T) {
		readOnlyState, err := dState.ValidatorAtIndexReadOnly(1)
		require.NoError(t, err)
		readOnlyBytes := readOnlyState.PublicKey()
		require.NotEmpty(t, readOnlyBytes)
		require.Equal(t, hexutil.Encode(readOnlyBytes[:]), hexutil.Encode(byteValue[:]))
	})
}

func TestEffectiveBalanceAtIndex(t *testing.T) {
	dState, _ := util.DeterministicGenesisState(t, 10)
	for i := range uint64(10) {
		want, err := dState.ValidatorAtIndexReadOnly(primitives.ValidatorIndex(i))
		require.NoError(t, err)
		got, err := dState.EffectiveBalanceAtIndex(primitives.ValidatorIndex(i))
		require.NoError(t, err)
		require.Equal(t, want.EffectiveBalance(), got)
	}

	_, err := dState.EffectiveBalanceAtIndex(primitives.ValidatorIndex(10))
	require.NotNil(t, err)
}

func TestPendingBalanceToWithdraw(t *testing.T) {
	pb := &ethpb.BeaconStateElectra{
		PendingPartialWithdrawals: []*ethpb.PendingPartialWithdrawal{
			{
				Amount: 100,
			},
			{
				Amount: 200,
			},
			{
				Amount: 300,
			},
		},
	}
	state, err := statenative.InitializeFromProtoUnsafeElectra(pb)
	require.NoError(t, err)

	ab, err := state.PendingBalanceToWithdraw(0)
	require.NoError(t, err)
	require.Equal(t, uint64(600), ab)
}

func TestAggregateKeyFromIndices(t *testing.T) {
	dState, _ := util.DeterministicGenesisState(t, 10)
	pKey1 := dState.PubkeyAtIndex(3)
	pKey2 := dState.PubkeyAtIndex(7)
	pKey3 := dState.PubkeyAtIndex(9)

	aggKey, err := bls.AggregatePublicKeys([][]byte{pKey1[:], pKey2[:], pKey3[:]})
	require.NoError(t, err)

	retKey, err := dState.AggregateKeyFromIndices([]uint64{3, 7, 9})
	require.NoError(t, err)

	assert.Equal(t, true, aggKey.Equals(retKey), "unequal aggregated keys")
}

func TestHasPendingBalanceToWithdraw(t *testing.T) {
	pb := &ethpb.BeaconStateElectra{
		PendingPartialWithdrawals: []*ethpb.PendingPartialWithdrawal{
			{
				Amount: 100,
				Index:  1,
			},
			{
				Amount: 200,
				Index:  2,
			},
			{
				Amount: 300,
				Index:  3,
			},
			{
				Amount: 0,
				Index:  4,
			},
		},
	}
	state, err := statenative.InitializeFromProtoUnsafeElectra(pb)
	require.NoError(t, err)

	ok, err := state.HasPendingBalanceToWithdraw(1)
	require.NoError(t, err)
	require.Equal(t, true, ok)

	ok, err = state.HasPendingBalanceToWithdraw(5)
	require.NoError(t, err)
	require.Equal(t, false, ok)

	// Handle 0 amount case.
	ok, err = state.HasPendingBalanceToWithdraw(4)
	require.NoError(t, err)
	require.Equal(t, false, ok)
}

const benchRegistrySize = 2_300_000 // ~ number of validators on mainnet at the time of writing

var (
	benchRegistryOnce     sync.Once
	benchRegistryShared   state.BeaconState
	benchRegistryDiverged state.BeaconState
	benchRegistryErr      error
)

// benchRegistryStates returns two cached states holding benchRegistrySize validators each: one
// whose registry consists purely of shared multi-value slice items, and a copy of it in which
// every 10th validator has been updated, so that 10% of the registry resolves to individual
// values. The latter approximates a head state that has diverged from its ancestors.
func benchRegistryStates(b *testing.B) (sharedOnly, tenPctIndividual state.BeaconState) {
	benchRegistryOnce.Do(func() {
		vals := make([]*ethpb.Validator, benchRegistrySize)
		for i := range vals {
			pk := make([]byte, 48)
			wc := make([]byte, 32)
			pk[0] = byte(i)
			vals[i] = &ethpb.Validator{
				PublicKey:             pk,
				WithdrawalCredentials: wc,
				EffectiveBalance:      32_000_000_000,
				ExitEpoch:             100,
				ActivationEpoch:       1,
			}
		}
		benchRegistryShared, benchRegistryErr = statenative.InitializeFromProtoUnsafeDeneb(&ethpb.BeaconStateDeneb{Validators: vals})
		if benchRegistryErr != nil {
			return
		}
		diverged := benchRegistryShared.Copy()
		for i := 0; i < benchRegistrySize; i += 10 {
			idx := primitives.ValidatorIndex(i)
			var v *ethpb.Validator
			v, benchRegistryErr = diverged.ValidatorAtIndex(idx)
			if benchRegistryErr != nil {
				return
			}
			v.EffectiveBalance = 31_000_000_000
			if benchRegistryErr = diverged.UpdateValidatorAtIndex(idx, v); benchRegistryErr != nil {
				return
			}
		}
		benchRegistryDiverged = diverged
		// Collect the sizeable setup garbage now so that it is not attributed to the first
		// measured iterations.
		runtime.GC()
	})
	require.NoError(b, benchRegistryErr)
	return benchRegistryShared, benchRegistryDiverged
}

// BenchmarkValidatorsReadOnlySeq measures the per-validator cost of iterating the
// registry through the ReadOnlyValidator wrapper.
//
// Results on an Apple M4 Pro, comparing the previous implementation (one lock acquisition
// and At call per index) against the bulk All iterator
// (go test -benchtime 20x -count 12, benchstat medians):
//
//	shared values only       per-index At 91.0 ms/op, bulk All 75.4 ms/op (-17%)
//	10% individual values    per-index At 93.7 ms/op, bulk All 78.7 ms/op (-16%)
func BenchmarkValidatorsReadOnlySeq(b *testing.B) {
	sharedOnly, tenPctIndividual := benchRegistryStates(b)
	for _, tc := range []struct {
		name string
		st   state.BeaconState
	}{
		{name: "shared values only", st: sharedOnly},
		{name: "10% individual values", st: tenPctIndividual},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				for _, v := range tc.st.ValidatorsReadOnlySeq() {
					_ = v.EffectiveBalance()
				}
			}
		})
	}
}

// BenchmarkPublicKeys measures the cost of building the index-aligned list of every
// validator public key.
//
// Results on an Apple M4 Pro, comparing the previous implementation (one lock acquisition
// and At call per index) against the bulk All iterator
// (go test -benchtime 20x -count 12, benchstat medians):
//
//	shared values only       per-index At 84.6 ms/op, bulk All 62.5 ms/op (-26%)
//	10% individual values    per-index At 83.0 ms/op, bulk All 58.9 ms/op (-29%)
func BenchmarkPublicKeys(b *testing.B) {
	sharedOnly, tenPctIndividual := benchRegistryStates(b)
	for _, tc := range []struct {
		name string
		st   state.BeaconState
	}{
		{name: "shared values only", st: sharedOnly},
		{name: "10% individual values", st: tenPctIndividual},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				keys, err := tc.st.PublicKeys()
				require.NoError(b, err)
				require.Equal(b, benchRegistrySize, len(keys))
			}
		})
	}
}

// BenchmarkValidatorsReadOnly measures the cost of building the full slice of read-only
// validator wrappers returned by ValidatorsReadOnly.
func BenchmarkValidatorsReadOnly(b *testing.B) {
	st, _ := benchRegistryStates(b)

	b.ReportAllocs()
	for b.Loop() {
		ros := st.ValidatorsReadOnly()
		require.Equal(b, benchRegistrySize, len(ros))
	}
}

// BenchmarkAggregateKeyFromIndices measures the cost of aggregating validator public
// keys.
func BenchmarkAggregateKeyFromIndices(b *testing.B) {
	n := params.BeaconConfig().MaxValidatorsPerCommittee

	st, _ := util.DeterministicGenesisState(b, n)
	idxs := make([]uint64, n)
	for i := range idxs {
		idxs[i] = uint64(i)
	}

	b.ReportAllocs()
	for b.Loop() {
		_, err := st.AggregateKeyFromIndices(idxs)
		require.NoError(b, err)
	}
}
