package state_native_test

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	statenative "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/golang/snappy"
)

func TestNew(t *testing.T) {
	t.Run("Phase0", func(t *testing.T) {
		testNewFork(t, &ethpb.BeaconState{Slot: 4}, &ethpb.BeaconState{})
	})
	t.Run("Altair", func(t *testing.T) {
		testNewFork(t, &ethpb.BeaconStateAltair{Slot: 4}, &ethpb.BeaconStateAltair{})
	})
	t.Run("Bellatrix", func(t *testing.T) {
		testNewFork(t, &ethpb.BeaconStateBellatrix{Slot: 4}, &ethpb.BeaconStateBellatrix{})
	})
	t.Run("Capella", func(t *testing.T) {
		testNewFork(t, &ethpb.BeaconStateCapella{Slot: 4}, &ethpb.BeaconStateCapella{})
	})
	t.Run("Deneb", func(t *testing.T) {
		testNewFork(t, &ethpb.BeaconStateDeneb{Slot: 4}, &ethpb.BeaconStateDeneb{})
	})
	t.Run("Electra", func(t *testing.T) {
		testNewFork(t, &ethpb.BeaconStateElectra{Slot: 4}, &ethpb.BeaconStateElectra{})
	})
	t.Run("Fulu", func(t *testing.T) {
		testNewFork(t, &ethpb.BeaconStateFulu{Slot: 4}, &ethpb.BeaconStateFulu{})
	})
	t.Run("Gloas", func(t *testing.T) {
		testNewFork(t, &ethpb.BeaconStateGloas{Slot: 4}, &ethpb.BeaconStateGloas{})
	})
	t.Run("Phase0 full state", func(t *testing.T) {
		if fieldparams.Preset == "minimal" {
			// DeterministicGenesisState sizes the state from the beacon config.
			params.SetupTestConfigCleanup(t)
			params.OverrideBeaconConfig(params.MinimalSpecConfig())
		}
		testState, _ := util.DeterministicGenesisState(t, 64)
		full, err := statenative.ContainerFrom[*ethpb.BeaconState](testState.ToContainerUnsafe())
		require.NoError(t, err)
		_, err = statenative.New(full)
		require.NoError(t, err)
		_, err = statenative.NewUnsafe(full)
		require.NoError(t, err)
	})
}

// testNewFork checks New and NewUnsafe on a nil container, on the empty
// container empty, and on st, which has nil validators.
func testNewFork[T statenative.Container](t *testing.T, st, empty T) {
	ctors := map[string]func(T) (state.BeaconState, error){
		"New":       statenative.New[T],
		"NewUnsafe": statenative.NewUnsafe[T],
	}
	for name, ctor := range ctors {
		t.Run(name, func(t *testing.T) {
			var nilState T
			_, err := ctor(nilState)
			require.ErrorContains(t, "received nil state", err)
			_, err = ctor(empty)
			require.NoError(t, err)
			got, err := ctor(st)
			require.NoError(t, err)
			require.Equal(t, primitives.Slot(4), got.Slot())
		})
	}
}

func TestBeaconState_HashTreeRoot(t *testing.T) {
	testState, _ := util.DeterministicGenesisState(t, 64)

	type test struct {
		name        string
		stateModify func(beaconState state.BeaconState) (state.BeaconState, error)
		error       string
	}
	initTests := []test{
		{
			name: "unchanged state",
			stateModify: func(beaconState state.BeaconState) (state.BeaconState, error) {
				return beaconState, nil
			},
			error: "",
		},
		{
			name: "different slot",
			stateModify: func(beaconState state.BeaconState) (state.BeaconState, error) {
				if err := beaconState.SetSlot(5); err != nil {
					return nil, err
				}
				return beaconState, nil
			},
			error: "",
		},
		{
			name: "different validator balance",
			stateModify: func(beaconState state.BeaconState) (state.BeaconState, error) {
				val, err := beaconState.ValidatorAtIndex(5)
				if err != nil {
					return nil, err
				}
				val.EffectiveBalance = params.BeaconConfig().MaxEffectiveBalance - params.BeaconConfig().EffectiveBalanceIncrement
				if err := beaconState.UpdateValidatorAtIndex(5, val); err != nil {
					return nil, err
				}
				return beaconState, nil
			},
			error: "",
		},
	}

	var err error
	var oldHTR []byte
	for _, tt := range initTests {
		t.Run(tt.name, func(t *testing.T) {
			testState, err = tt.stateModify(testState)
			assert.NoError(t, err)
			root, err := testState.HashTreeRoot(t.Context())
			if err == nil && tt.error != "" {
				t.Errorf("Expected error, expected %v, received %v", tt.error, err)
			}
			pbState, err := statenative.ContainerFrom[*ethpb.BeaconState](testState.ToContainerUnsafe())
			require.NoError(t, err)
			genericHTR, err := pbState.HashTreeRoot()
			if err == nil && tt.error != "" {
				t.Errorf("Expected error, expected %v, received %v", tt.error, err)
			}
			assert.DeepNotEqual(t, []byte{}, root[:], "Received empty hash tree root")
			assert.DeepEqual(t, genericHTR[:], root[:], "Expected hash tree root to match generic")
			if len(oldHTR) != 0 && bytes.Equal(root[:], oldHTR) {
				t.Errorf("Expected HTR to change, received %#x == old %#x", root, oldHTR)
			}
			oldHTR = root[:]
		})
	}
}

func BenchmarkBeaconState(b *testing.B) {
	testState, _ := util.DeterministicGenesisState(b, 16000)
	pbState, err := statenative.ContainerFrom[*ethpb.BeaconState](testState.ToContainerUnsafe())
	require.NoError(b, err)

	b.Run("Vectorized SHA256", func(b *testing.B) {
		st, err := statenative.NewUnsafe(pbState)
		require.NoError(b, err)
		_, err = st.HashTreeRoot(b.Context())
		assert.NoError(b, err)
	})

	b.Run("Current SHA256", func(b *testing.B) {
		_, err := pbState.HashTreeRoot()
		require.NoError(b, err)
	})
}

func TestBeaconState_HashTreeRoot_FieldTrie(t *testing.T) {
	testState, _ := util.DeterministicGenesisState(t, 64)

	type test struct {
		name        string
		stateModify func(state.BeaconState) (state.BeaconState, error)
		error       string
	}
	initTests := []test{
		{
			name: "unchanged state",
			stateModify: func(beaconState state.BeaconState) (state.BeaconState, error) {
				return beaconState, nil
			},
			error: "",
		},
		{
			name: "different slot",
			stateModify: func(beaconState state.BeaconState) (state.BeaconState, error) {
				if err := beaconState.SetSlot(5); err != nil {
					return nil, err
				}
				return beaconState, nil
			},
			error: "",
		},
		{
			name: "different validator balance",
			stateModify: func(beaconState state.BeaconState) (state.BeaconState, error) {
				val, err := beaconState.ValidatorAtIndex(5)
				if err != nil {
					return nil, err
				}
				val.EffectiveBalance = params.BeaconConfig().MaxEffectiveBalance - params.BeaconConfig().EffectiveBalanceIncrement
				if err := beaconState.UpdateValidatorAtIndex(5, val); err != nil {
					return nil, err
				}
				return beaconState, nil
			},
			error: "",
		},
	}

	var err error
	var oldHTR []byte
	for _, tt := range initTests {
		t.Run(tt.name, func(t *testing.T) {
			testState, err = tt.stateModify(testState)
			assert.NoError(t, err)
			root, err := testState.HashTreeRoot(t.Context())
			if err == nil && tt.error != "" {
				t.Errorf("Expected error, expected %v, received %v", tt.error, err)
			}
			pbState, err := statenative.ContainerFrom[*ethpb.BeaconState](testState.ToContainerUnsafe())
			require.NoError(t, err)
			genericHTR, err := pbState.HashTreeRoot()
			if err == nil && tt.error != "" {
				t.Errorf("Expected error, expected %v, received %v", tt.error, err)
			}
			assert.DeepNotEqual(t, []byte{}, root[:], "Received empty hash tree root")
			assert.DeepEqual(t, genericHTR[:], root[:], "Expected hash tree root to match generic")
			if len(oldHTR) != 0 && bytes.Equal(root[:], oldHTR) {
				t.Errorf("Expected HTR to change, received %#x == old %#x", root, oldHTR)
			}
			oldHTR = root[:]
		})
	}
}

func TestBeaconState_AppendValidator_DoesntMutateCopy(t *testing.T) {
	st0, err := util.NewBeaconState()
	require.NoError(t, err)
	st1 := st0.Copy()
	originalCount := st1.NumValidators()

	val := &ethpb.Validator{Slashed: true}
	assert.NoError(t, st0.AppendValidator(val))
	assert.Equal(t, originalCount, st1.NumValidators(), "st1 NumValidators mutated")
	_, ok := st1.ValidatorIndexByPubkey(bytesutil.ToBytes48(val.PublicKey))
	assert.Equal(t, false, ok, "Expected no validator index to be present in st1 for the newly inserted pubkey")
}

func TestBeaconState_ValidatorMutation_Phase0(t *testing.T) {
	testState, _ := util.DeterministicGenesisState(t, 400)
	pbState, err := statenative.ContainerFrom[*ethpb.BeaconState](testState.ToContainerUnsafe())
	require.NoError(t, err)
	testState, err = statenative.New(pbState)
	require.NoError(t, err)

	_, err = testState.HashTreeRoot(t.Context())
	require.NoError(t, err)

	// Reset tries
	require.NoError(t, testState.UpdateValidatorAtIndex(200, new(ethpb.Validator)))
	_, err = testState.HashTreeRoot(t.Context())
	require.NoError(t, err)

	newState1 := testState.Copy()
	_ = testState.Copy()

	require.NoError(t, testState.UpdateValidatorAtIndex(15, &ethpb.Validator{
		PublicKey:                  make([]byte, 48),
		WithdrawalCredentials:      make([]byte, 32),
		EffectiveBalance:           1111,
		Slashed:                    false,
		ActivationEligibilityEpoch: 1112,
		ActivationEpoch:            1114,
		ExitEpoch:                  1116,
		WithdrawableEpoch:          1117,
	}))

	rt, err := testState.HashTreeRoot(t.Context())
	require.NoError(t, err)
	pbState, err = statenative.ContainerFrom[*ethpb.BeaconState](testState.ToContainerUnsafe())
	require.NoError(t, err)

	copiedTestState, err := statenative.New(pbState)
	require.NoError(t, err)

	rt2, err := copiedTestState.HashTreeRoot(t.Context())
	require.NoError(t, err)

	assert.Equal(t, rt, rt2)

	require.NoError(t, newState1.UpdateValidatorAtIndex(150, &ethpb.Validator{
		PublicKey:                  make([]byte, 48),
		WithdrawalCredentials:      make([]byte, 32),
		EffectiveBalance:           2111,
		Slashed:                    false,
		ActivationEligibilityEpoch: 2112,
		ActivationEpoch:            2114,
		ExitEpoch:                  2116,
		WithdrawableEpoch:          2117,
	}))

	rt, err = newState1.HashTreeRoot(t.Context())
	require.NoError(t, err)
	pbState, err = statenative.ContainerFrom[*ethpb.BeaconState](newState1.ToContainerUnsafe())
	require.NoError(t, err)

	copiedTestState, err = statenative.New(pbState)
	require.NoError(t, err)

	rt2, err = copiedTestState.HashTreeRoot(t.Context())
	require.NoError(t, err)

	assert.Equal(t, rt, rt2)
}

func TestBeaconState_ValidatorMutation_Altair(t *testing.T) {
	testState, _ := util.DeterministicGenesisStateAltair(t, 400)
	pbState, err := statenative.ContainerFrom[*ethpb.BeaconStateAltair](testState.ToContainerUnsafe())
	require.NoError(t, err)
	testState, err = statenative.New(pbState)
	require.NoError(t, err)

	_, err = testState.HashTreeRoot(t.Context())
	require.NoError(t, err)

	// Reset tries
	require.NoError(t, testState.UpdateValidatorAtIndex(200, new(ethpb.Validator)))
	_, err = testState.HashTreeRoot(t.Context())
	require.NoError(t, err)

	newState1 := testState.Copy()
	_ = testState.Copy()

	require.NoError(t, testState.UpdateValidatorAtIndex(15, &ethpb.Validator{
		PublicKey:                  make([]byte, 48),
		WithdrawalCredentials:      make([]byte, 32),
		EffectiveBalance:           1111,
		Slashed:                    false,
		ActivationEligibilityEpoch: 1112,
		ActivationEpoch:            1114,
		ExitEpoch:                  1116,
		WithdrawableEpoch:          1117,
	}))

	rt, err := testState.HashTreeRoot(t.Context())
	require.NoError(t, err)
	pbState, err = statenative.ContainerFrom[*ethpb.BeaconStateAltair](testState.ToContainerUnsafe())
	require.NoError(t, err)

	copiedTestState, err := statenative.New(pbState)
	require.NoError(t, err)

	rt2, err := copiedTestState.HashTreeRoot(t.Context())
	require.NoError(t, err)

	assert.Equal(t, rt, rt2)

	require.NoError(t, newState1.UpdateValidatorAtIndex(150, &ethpb.Validator{
		PublicKey:                  make([]byte, 48),
		WithdrawalCredentials:      make([]byte, 32),
		EffectiveBalance:           2111,
		Slashed:                    false,
		ActivationEligibilityEpoch: 2112,
		ActivationEpoch:            2114,
		ExitEpoch:                  2116,
		WithdrawableEpoch:          2117,
	}))

	rt, err = newState1.HashTreeRoot(t.Context())
	require.NoError(t, err)
	pbState, err = statenative.ContainerFrom[*ethpb.BeaconStateAltair](newState1.ToContainerUnsafe())
	require.NoError(t, err)

	copiedTestState, err = statenative.New(pbState)
	require.NoError(t, err)

	rt2, err = copiedTestState.HashTreeRoot(t.Context())
	require.NoError(t, err)

	assert.Equal(t, rt, rt2)
}

func TestBeaconState_ValidatorMutation_Bellatrix(t *testing.T) {
	testState, _ := util.DeterministicGenesisStateBellatrix(t, 400)
	pbState, err := statenative.ContainerFrom[*ethpb.BeaconStateBellatrix](testState.ToContainerUnsafe())
	require.NoError(t, err)
	testState, err = statenative.New(pbState)
	require.NoError(t, err)

	_, err = testState.HashTreeRoot(t.Context())
	require.NoError(t, err)

	// Reset tries
	require.NoError(t, testState.UpdateValidatorAtIndex(200, new(ethpb.Validator)))
	_, err = testState.HashTreeRoot(t.Context())
	require.NoError(t, err)

	newState1 := testState.Copy()
	_ = testState.Copy()

	require.NoError(t, testState.UpdateValidatorAtIndex(15, &ethpb.Validator{
		PublicKey:                  make([]byte, 48),
		WithdrawalCredentials:      make([]byte, 32),
		EffectiveBalance:           1111,
		Slashed:                    false,
		ActivationEligibilityEpoch: 1112,
		ActivationEpoch:            1114,
		ExitEpoch:                  1116,
		WithdrawableEpoch:          1117,
	}))

	rt, err := testState.HashTreeRoot(t.Context())
	require.NoError(t, err)
	pbState, err = statenative.ContainerFrom[*ethpb.BeaconStateBellatrix](testState.ToContainerUnsafe())
	require.NoError(t, err)

	copiedTestState, err := statenative.New(pbState)
	require.NoError(t, err)

	rt2, err := copiedTestState.HashTreeRoot(t.Context())
	require.NoError(t, err)

	assert.Equal(t, rt, rt2)

	require.NoError(t, newState1.UpdateValidatorAtIndex(150, &ethpb.Validator{
		PublicKey:                  make([]byte, 48),
		WithdrawalCredentials:      make([]byte, 32),
		EffectiveBalance:           2111,
		Slashed:                    false,
		ActivationEligibilityEpoch: 2112,
		ActivationEpoch:            2114,
		ExitEpoch:                  2116,
		WithdrawableEpoch:          2117,
	}))

	rt, err = newState1.HashTreeRoot(t.Context())
	require.NoError(t, err)
	pbState, err = statenative.ContainerFrom[*ethpb.BeaconStateBellatrix](newState1.ToContainerUnsafe())
	require.NoError(t, err)

	copiedTestState, err = statenative.New(pbState)
	require.NoError(t, err)

	rt2, err = copiedTestState.HashTreeRoot(t.Context())
	require.NoError(t, err)

	assert.Equal(t, rt, rt2)
}

func TestBeaconState_InitializeInactivityScoresCorrectly_Deneb(t *testing.T) {
	st, _ := util.DeterministicGenesisStateDeneb(t, 200)
	_, err := st.HashTreeRoot(t.Context())
	require.NoError(t, err)
	ic, err := st.InactivityScores()
	require.NoError(t, err)
	ic[10] = 10000
	ic[100] = 1000

	err = st.SetInactivityScores(ic)
	require.NoError(t, err)

	_, err = st.HashTreeRoot(t.Context())
	require.NoError(t, err)

	ic[150] = 2390239
	err = st.SetInactivityScores(ic)
	require.NoError(t, err)
	rt, err := st.HashTreeRoot(t.Context())
	require.NoError(t, err)

	copiedSt, ok := st.ToContainerUnsafe().(*ethpb.BeaconStateDeneb)
	if !ok {
		t.Error("not ok")
	}
	newSt, err := statenative.NewUnsafe(copiedSt)
	require.NoError(t, err)

	newRt, err := newSt.HashTreeRoot(t.Context())
	require.NoError(t, err)

	require.DeepSSZEqual(t, rt, newRt)
}

func TestBeaconChainCopy_Electra(t *testing.T) {
	// Load a serialized Electra state from disk.
	// This is a fully hydrated random test case from spectests.
	serializedBytes, err := util.BazelFileBytes("tests/mainnet/electra/ssz_static/BeaconState/ssz_random/case_0/serialized.ssz_snappy")
	require.NoError(t, err)
	serializedSSZ, err := snappy.Decode(nil /* dst */, serializedBytes)
	require.NoError(t, err)
	pb := &ethpb.BeaconStateElectra{}
	require.NoError(t, pb.UnmarshalSSZ(serializedSSZ))
	st, err := statenative.New(pb)
	require.NoError(t, err)

	// Sanity check that New and ToContainer round-trip.
	require.DeepSSZEqual(t, pb, st.ToContainer(), "New does not match input container")

	// Perform the copy and check that the copied state matches the original state.
	st2 := st.Copy()
	require.DeepSSZEqual(t, st.ToContainer(), st2.ToContainer(), "Copied state does not match original state")
}

func TestContainerFrom(t *testing.T) {
	t.Run("matching type", func(t *testing.T) {
		pb := &ethpb.BeaconStateAltair{Slot: 7}
		got, err := statenative.ContainerFrom[*ethpb.BeaconStateAltair](any(pb))
		require.NoError(t, err)
		require.Equal(t, pb, got)
	})
	t.Run("other fork", func(t *testing.T) {
		got, err := statenative.ContainerFrom[*ethpb.BeaconStateAltair](any(&ethpb.BeaconState{}))
		require.ErrorContains(t, "input is *eth.BeaconState, not *eth.BeaconStateAltair", err)
		require.Equal(t, (*ethpb.BeaconStateAltair)(nil), got)
	})
	t.Run("nil input", func(t *testing.T) {
		_, err := statenative.ContainerFrom[*ethpb.BeaconState](nil)
		require.ErrorContains(t, "input is <nil>", err)
	})
}

func TestNew_CopiesInput(t *testing.T) {
	newPB := func() *ethpb.BeaconStateDeneb {
		return &ethpb.BeaconStateDeneb{Fork: &ethpb.Fork{Epoch: 1}}
	}
	t.Run("New copies", func(t *testing.T) {
		pb := newPB()
		st, err := statenative.New(pb)
		require.NoError(t, err)
		pb.Fork.Epoch = 2
		require.Equal(t, primitives.Epoch(1), st.Fork().Epoch)
	})
	t.Run("NewUnsafe shares", func(t *testing.T) {
		pb := newPB()
		st, err := statenative.NewUnsafe(pb)
		require.NoError(t, err)
		pb.Fork.Epoch = 2
		require.Equal(t, primitives.Epoch(2), st.Fork().Epoch)
	})
}
