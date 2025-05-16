package state_native

import (
	"reflect"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/prysmaticlabs/go-bitfield"
)

func TestStateReferenceSharing_Finalizer_Phase0(t *testing.T) {
	// This test showcases the logic on the Slashings field with the GC finalizer.

	s, err := InitializeFromProtoUnsafePhase0(&ethpb.BeaconState{Slashings: []uint64{10, 30, 40}})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)
	assert.Equal(t, uint(1), a.sharedFieldReferences[types.Slashings].Refs(), "Expected a single reference for RANDAO mixes")

	func() {
		// Create object in a different scope for GC
		b := a.Copy()
		assert.Equal(t, uint(2), a.sharedFieldReferences[types.Slashings].Refs(), "Expected 2 references to RANDAO mixes")
		_ = b
	}()

	runtime.GC() // Should run finalizer on object b
	assert.Equal(t, uint(1), a.sharedFieldReferences[types.Slashings].Refs(), "Expected 1 shared reference to RANDAO mixes!")

	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)
	assert.Equal(t, uint(2), b.sharedFieldReferences[types.Slashings].Refs(), "Expected 2 shared references to RANDAO mixes")
	require.NoError(t, b.UpdateSlashingsAtIndex(0, 100))
	if b.sharedFieldReferences[types.Slashings].Refs() != 1 || a.sharedFieldReferences[types.Slashings].Refs() != 1 {
		t.Error("Expected 1 shared reference to Slashings for both a and b")
	}
}

func TestStateReferenceSharing_Finalizer_Altair(t *testing.T) {
	// This test showcases the logic on the Slashings field with the GC finalizer.

	s, err := InitializeFromProtoUnsafeAltair(&ethpb.BeaconStateAltair{Slashings: []uint64{10, 30, 40}})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)
	assert.Equal(t, uint(1), a.sharedFieldReferences[types.Slashings].Refs(), "Expected a single reference for RANDAO mixes")

	func() {
		// Create object in a different scope for GC
		b := a.Copy()
		assert.Equal(t, uint(2), a.sharedFieldReferences[types.Slashings].Refs(), "Expected 2 references to RANDAO mixes")
		_ = b
	}()

	runtime.GC() // Should run finalizer on object b
	assert.Equal(t, uint(1), a.sharedFieldReferences[types.Slashings].Refs(), "Expected 1 shared reference to RANDAO mixes!")

	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)
	assert.Equal(t, uint(2), b.sharedFieldReferences[types.Slashings].Refs(), "Expected 2 shared references to RANDAO mixes")
	require.NoError(t, b.UpdateSlashingsAtIndex(0, 100))
	if b.sharedFieldReferences[types.Slashings].Refs() != 1 || a.sharedFieldReferences[types.Slashings].Refs() != 1 {
		t.Error("Expected 1 shared reference to Slashings for both a and b")
	}
}

func TestStateReferenceSharing_Finalizer_Bellatrix(t *testing.T) {
	// This test showcases the logic on the Slashings field with the GC finalizer.

	s, err := InitializeFromProtoUnsafeBellatrix(&ethpb.BeaconStateBellatrix{Slashings: []uint64{10, 30, 40}})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)
	assert.Equal(t, uint(1), a.sharedFieldReferences[types.Slashings].Refs(), "Expected a single reference for RANDAO mixes")

	func() {
		// Create object in a different scope for GC
		b := a.Copy()
		assert.Equal(t, uint(2), a.sharedFieldReferences[types.Slashings].Refs(), "Expected 2 references to RANDAO mixes")
		_ = b
	}()

	runtime.GC() // Should run finalizer on object b
	assert.Equal(t, uint(1), a.sharedFieldReferences[types.Slashings].Refs(), "Expected 1 shared reference to RANDAO mixes!")

	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)
	assert.Equal(t, uint(2), b.sharedFieldReferences[types.Slashings].Refs(), "Expected 2 shared references to RANDAO mixes")
	require.NoError(t, b.UpdateSlashingsAtIndex(0, 100))
	if b.sharedFieldReferences[types.Slashings].Refs() != 1 || a.sharedFieldReferences[types.Slashings].Refs() != 1 {
		t.Error("Expected 1 shared reference to Slashings for both a and b")
	}
}

func TestStateReferenceSharing_Finalizer_Capella(t *testing.T) {
	// This test showcases the logic on the Slashings field with the GC finalizer.

	s, err := InitializeFromProtoUnsafeCapella(&ethpb.BeaconStateCapella{Slashings: []uint64{10, 30, 40}})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)
	assert.Equal(t, uint(1), a.sharedFieldReferences[types.Slashings].Refs(), "Expected a single reference for RANDAO mixes")

	func() {
		// Create object in a different scope for GC
		b := a.Copy()
		assert.Equal(t, uint(2), a.sharedFieldReferences[types.Slashings].Refs(), "Expected 2 references to RANDAO mixes")
		_ = b
	}()

	runtime.GC() // Should run finalizer on object b
	assert.Equal(t, uint(1), a.sharedFieldReferences[types.Slashings].Refs(), "Expected 1 shared reference to RANDAO mixes!")

	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)
	assert.Equal(t, uint(2), b.sharedFieldReferences[types.Slashings].Refs(), "Expected 2 shared references to RANDAO mixes")
	require.NoError(t, b.UpdateSlashingsAtIndex(0, 100))
	if b.sharedFieldReferences[types.Slashings].Refs() != 1 || a.sharedFieldReferences[types.Slashings].Refs() != 1 {
		t.Error("Expected 1 shared reference to Slashings for both a and b")
	}
}

func TestStateReferenceSharing_Finalizer_Deneb(t *testing.T) {
	// This test showcases the logic on the Slashings field with the GC finalizer.

	s, err := InitializeFromProtoUnsafeDeneb(&ethpb.BeaconStateDeneb{Slashings: []uint64{10, 30, 40}})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)
	assert.Equal(t, uint(1), a.sharedFieldReferences[types.Slashings].Refs(), "Expected a single reference for RANDAO mixes")

	func() {
		// Create object in a different scope for GC
		b := a.Copy()
		assert.Equal(t, uint(2), a.sharedFieldReferences[types.Slashings].Refs(), "Expected 2 references to RANDAO mixes")
		_ = b
	}()

	runtime.GC() // Should run finalizer on object b
	assert.Equal(t, uint(1), a.sharedFieldReferences[types.Slashings].Refs(), "Expected 1 shared reference to RANDAO mixes!")

	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)
	assert.Equal(t, uint(2), b.sharedFieldReferences[types.Slashings].Refs(), "Expected 2 shared references to RANDAO mixes")
	require.NoError(t, b.UpdateSlashingsAtIndex(0, 100))
	if b.sharedFieldReferences[types.Slashings].Refs() != 1 || a.sharedFieldReferences[types.Slashings].Refs() != 1 {
		t.Error("Expected 1 shared reference to Slashings for both a and b")
	}
}

func TestStateReferenceCopy_NoUnexpectedSlashingsMutation_Phase0(t *testing.T) {
	val1, val2 := uint64(10), uint64(20)
	s, err := InitializeFromProtoUnsafePhase0(&ethpb.BeaconState{
		Slashings: []uint64{val1},
	})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)
	require.NoError(t, err)
	assertRefCount(t, a, types.Slashings, 1)

	// Copy, increases reference count.
	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)
	assertRefCount(t, a, types.Slashings, 2)
	assertRefCount(t, b, types.Slashings, 2)

	// Assert shared state.
	slashingsA := a.Slashings()
	slashingsB := b.Slashings()
	assertValFound(t, slashingsA, val1)
	assertValFound(t, slashingsB, val1)

	// Mutator should only affect calling state: a.
	require.NoError(t, a.UpdateSlashingsAtIndex(0, val2))

	// Assert no shared state mutation occurred only on state a (copy on write).
	assertValFound(t, a.Slashings(), val2)
	assertValNotFound(t, a.Slashings(), val1)
	assertValFound(t, b.Slashings(), val1)
	assertValNotFound(t, b.Slashings(), val2)
	assertValFound(t, slashingsB, val1)
	assertValNotFound(t, slashingsB, val2)
	assert.DeepEqual(t, val2, a.Slashings()[0], "Expected mutation not found")
	assert.DeepEqual(t, val1, slashingsB[0], "Unexpected mutation found")

	// Copy on write happened, reference counters are reset.
	assertRefCount(t, a, types.Slashings, 1)
	assertRefCount(t, b, types.Slashings, 1)
}

func TestStateReferenceCopy_NoUnexpectedSlashingMutation_Altair(t *testing.T) {
	val1, val2 := uint64(10), uint64(20)
	s, err := InitializeFromProtoUnsafeAltair(&ethpb.BeaconStateAltair{
		Slashings: []uint64{val1},
	})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)
	require.NoError(t, err)
	assertRefCount(t, a, types.Slashings, 1)

	// Copy, increases reference count.
	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)
	assertRefCount(t, a, types.Slashings, 2)
	assertRefCount(t, b, types.Slashings, 2)

	// Assert shared state.
	slashingsA := a.Slashings()
	slashingsB := b.Slashings()
	assertValFound(t, slashingsA, val1)
	assertValFound(t, slashingsB, val1)

	// Mutator should only affect calling state: a.
	require.NoError(t, a.UpdateSlashingsAtIndex(0, val2))

	// Assert no shared state mutation occurred only on state a (copy on write).
	assertValFound(t, a.Slashings(), val2)
	assertValNotFound(t, a.Slashings(), val1)
	assertValFound(t, b.Slashings(), val1)
	assertValNotFound(t, b.Slashings(), val2)
	assertValFound(t, slashingsB, val1)
	assertValNotFound(t, slashingsB, val2)
	assert.DeepEqual(t, val2, a.Slashings()[0], "Expected mutation not found")
	assert.DeepEqual(t, val1, slashingsB[0], "Unexpected mutation found")

	// Copy on write happened, reference counters are reset.
	assertRefCount(t, a, types.Slashings, 1)
	assertRefCount(t, b, types.Slashings, 1)
}

func TestStateReferenceCopy_NoUnexpectedSlashingMutation_Bellatrix(t *testing.T) {
	val1, val2 := uint64(10), uint64(20)
	s, err := InitializeFromProtoUnsafeBellatrix(&ethpb.BeaconStateBellatrix{
		Slashings: []uint64{val1},
	})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)
	require.NoError(t, err)
	assertRefCount(t, a, types.Slashings, 1)

	// Copy, increases reference count.
	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)
	assertRefCount(t, a, types.Slashings, 2)
	assertRefCount(t, b, types.Slashings, 2)

	// Assert shared state.
	slashingsA := a.Slashings()
	slashingsB := b.Slashings()
	assertValFound(t, slashingsA, val1)
	assertValFound(t, slashingsB, val1)

	// Mutator should only affect calling state: a.
	require.NoError(t, a.UpdateSlashingsAtIndex(0, val2))

	// Assert no shared state mutation occurred only on state a (copy on write).
	assertValFound(t, a.Slashings(), val2)
	assertValNotFound(t, a.Slashings(), val1)
	assertValFound(t, b.Slashings(), val1)
	assertValNotFound(t, b.Slashings(), val2)
	assertValFound(t, slashingsB, val1)
	assertValNotFound(t, slashingsB, val2)
	assert.DeepEqual(t, val2, a.Slashings()[0], "Expected mutation not found")
	assert.DeepEqual(t, val1, slashingsB[0], "Unexpected mutation found")

	// Copy on write happened, reference counters are reset.
	assertRefCount(t, a, types.Slashings, 1)
	assertRefCount(t, b, types.Slashings, 1)
}

func TestStateReferenceCopy_NoUnexpectedSlashingMutation_Capella(t *testing.T) {
	val1, val2 := uint64(10), uint64(20)
	s, err := InitializeFromProtoUnsafeCapella(&ethpb.BeaconStateCapella{
		Slashings: []uint64{val1},
	})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)
	require.NoError(t, err)
	assertRefCount(t, a, types.Slashings, 1)

	// Copy, increases reference count.
	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)
	assertRefCount(t, a, types.Slashings, 2)
	assertRefCount(t, b, types.Slashings, 2)

	// Assert shared state.
	slashingsA := a.Slashings()
	slashingsB := b.Slashings()
	assertValFound(t, slashingsA, val1)
	assertValFound(t, slashingsB, val1)

	// Mutator should only affect calling state: a.
	require.NoError(t, a.UpdateSlashingsAtIndex(0, val2))

	// Assert no shared state mutation occurred only on state a (copy on write).
	assertValFound(t, a.Slashings(), val2)
	assertValNotFound(t, a.Slashings(), val1)
	assertValFound(t, b.Slashings(), val1)
	assertValNotFound(t, b.Slashings(), val2)
	assertValFound(t, slashingsB, val1)
	assertValNotFound(t, slashingsB, val2)
	assert.DeepEqual(t, val2, a.Slashings()[0], "Expected mutation not found")
	assert.DeepEqual(t, val1, slashingsB[0], "Unexpected mutation found")

	// Copy on write happened, reference counters are reset.
	assertRefCount(t, a, types.Slashings, 1)
	assertRefCount(t, b, types.Slashings, 1)
}

func TestStateReferenceCopy_NoUnexpectedSlashingMutation_Deneb(t *testing.T) {
	val1, val2 := uint64(10), uint64(20)
	s, err := InitializeFromProtoUnsafeDeneb(&ethpb.BeaconStateDeneb{
		Slashings: []uint64{val1},
	})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)
	require.NoError(t, err)
	assertRefCount(t, a, types.Slashings, 1)

	// Copy, increases reference count.
	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)
	assertRefCount(t, a, types.Slashings, 2)
	assertRefCount(t, b, types.Slashings, 2)

	// Assert shared state.
	slashingsA := a.Slashings()
	slashingsB := b.Slashings()
	assertValFound(t, slashingsA, val1)
	assertValFound(t, slashingsB, val1)

	// Mutator should only affect calling state: a.
	require.NoError(t, a.UpdateSlashingsAtIndex(0, val2))

	// Assert no shared state mutation occurred only on state a (copy on write).
	assertValFound(t, a.Slashings(), val2)
	assertValNotFound(t, a.Slashings(), val1)
	assertValFound(t, b.Slashings(), val1)
	assertValNotFound(t, b.Slashings(), val2)
	assertValFound(t, slashingsB, val1)
	assertValNotFound(t, slashingsB, val2)
	assert.DeepEqual(t, val2, a.Slashings()[0], "Expected mutation not found")
	assert.DeepEqual(t, val1, slashingsB[0], "Unexpected mutation found")

	// Copy on write happened, reference counters are reset.
	assertRefCount(t, a, types.Slashings, 1)
	assertRefCount(t, b, types.Slashings, 1)
}

func TestStateReferenceCopy_NoUnexpectedAttestationsMutation(t *testing.T) {
	assertAttFound := func(vals []*ethpb.PendingAttestation, val uint64) {
		for i := range vals {
			if reflect.DeepEqual(vals[i].AggregationBits, bitfield.NewBitlist(val)) {
				return
			}
		}
		t.Log(string(debug.Stack()))
		t.Fatalf("Expected attestation not found (%v), want: %v", vals, val)
	}
	assertAttNotFound := func(vals []*ethpb.PendingAttestation, val uint64) {
		for i := range vals {
			if reflect.DeepEqual(vals[i].AggregationBits, bitfield.NewBitlist(val)) {
				t.Log(string(debug.Stack()))
				t.Fatalf("Unexpected attestation found (%v): %v", vals, val)
				return
			}
		}
	}

	s, err := InitializeFromProtoUnsafePhase0(&ethpb.BeaconState{})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)
	assertRefCount(t, a, types.PreviousEpochAttestations, 1)
	assertRefCount(t, a, types.CurrentEpochAttestations, 1)

	// Update initial state.
	atts := []*ethpb.PendingAttestation{
		{AggregationBits: bitfield.NewBitlist(1)},
		{AggregationBits: bitfield.NewBitlist(2)},
	}
	a.setPreviousEpochAttestations(atts[:1])
	a.setCurrentEpochAttestations(atts[:1])
	curAtt, err := a.CurrentEpochAttestations()
	require.NoError(t, err)
	assert.Equal(t, 1, len(curAtt), "Unexpected number of attestations")
	preAtt, err := a.PreviousEpochAttestations()
	require.NoError(t, err)
	assert.Equal(t, 1, len(preAtt), "Unexpected number of attestations")

	// Copy, increases reference count.
	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)
	assertRefCount(t, a, types.PreviousEpochAttestations, 2)
	assertRefCount(t, a, types.CurrentEpochAttestations, 2)
	assertRefCount(t, b, types.PreviousEpochAttestations, 2)
	assertRefCount(t, b, types.CurrentEpochAttestations, 2)
	prevAtts, err := b.PreviousEpochAttestations()
	require.NoError(t, err)
	assert.Equal(t, 1, len(prevAtts), "Unexpected number of attestations")
	currAtts, err := b.CurrentEpochAttestations()
	require.NoError(t, err)
	assert.Equal(t, 1, len(currAtts), "Unexpected number of attestations")

	// Assert shared state.
	currAttsA, err := a.CurrentEpochAttestations()
	require.NoError(t, err)
	prevAttsA, err := a.PreviousEpochAttestations()
	require.NoError(t, err)
	currAttsB, err := b.CurrentEpochAttestations()
	require.NoError(t, err)
	prevAttsB, err := b.PreviousEpochAttestations()
	require.NoError(t, err)
	if len(currAttsA) != len(currAttsB) || len(currAttsA) < 1 {
		t.Errorf("Unexpected number of attestations, want: %v", 1)
	}
	if len(prevAttsA) != len(prevAttsB) || len(prevAttsA) < 1 {
		t.Errorf("Unexpected number of attestations, want: %v", 1)
	}
	assertAttFound(currAttsA, 1)
	assertAttFound(prevAttsA, 1)
	assertAttFound(currAttsB, 1)
	assertAttFound(prevAttsB, 1)

	// Extend state attestations.
	require.NoError(t, a.AppendCurrentEpochAttestations(atts[1]))
	require.NoError(t, a.AppendPreviousEpochAttestations(atts[1]))
	curAtt, err = a.CurrentEpochAttestations()
	require.NoError(t, err)
	assert.Equal(t, 2, len(curAtt), "Unexpected number of attestations")
	preAtt, err = a.PreviousEpochAttestations()
	require.NoError(t, err)
	assert.Equal(t, 2, len(preAtt), "Unexpected number of attestations")
	currAttsA, err = a.CurrentEpochAttestations()
	require.NoError(t, err)
	prevAttsA, err = a.PreviousEpochAttestations()
	require.NoError(t, err)
	currAttsB, err = b.CurrentEpochAttestations()
	require.NoError(t, err)
	prevAttsB, err = b.PreviousEpochAttestations()
	require.NoError(t, err)
	assertAttFound(currAttsA, 1)
	assertAttFound(prevAttsA, 1)
	assertAttFound(currAttsA, 2)
	assertAttFound(prevAttsA, 2)
	assertAttFound(currAttsB, 1)
	assertAttFound(prevAttsB, 1)
	assertAttNotFound(currAttsB, 2)
	assertAttNotFound(prevAttsB, 2)

	// Mutator should only affect calling state: a.
	applyToEveryAttestation := func(state *BeaconState) {
		// One MUST copy on write.
		currEpochAtts, err := state.CurrentEpochAttestations()
		require.NoError(t, err)
		atts = make([]*ethpb.PendingAttestation, len(currEpochAtts))
		copy(atts, currEpochAtts)
		state.setCurrentEpochAttestations(atts)
		currEpochAtts, err = state.CurrentEpochAttestations()
		require.NoError(t, err)
		for i := range currEpochAtts {
			att := currEpochAtts[i].Copy()
			att.AggregationBits = bitfield.NewBitlist(3)
			currEpochAtts[i] = att
		}
		state.setCurrentEpochAttestations(currEpochAtts)

		prevEpochAtts, err := state.PreviousEpochAttestations()
		require.NoError(t, err)
		atts = make([]*ethpb.PendingAttestation, len(prevEpochAtts))
		copy(atts, prevEpochAtts)
		state.setPreviousEpochAttestations(atts)
		prevEpochAtts, err = state.PreviousEpochAttestations()
		require.NoError(t, err)
		for i := range prevEpochAtts {
			att := prevEpochAtts[i].Copy()
			att.AggregationBits = bitfield.NewBitlist(3)
			prevEpochAtts[i] = att
		}
		state.setPreviousEpochAttestations(prevEpochAtts)
	}
	applyToEveryAttestation(a)

	// Assert no shared state mutation occurred only on state a (copy on write).
	currAttsA, err = a.CurrentEpochAttestations()
	require.NoError(t, err)
	prevAttsA, err = a.PreviousEpochAttestations()
	require.NoError(t, err)
	assertAttFound(currAttsA, 3)
	assertAttFound(prevAttsA, 3)
	assertAttNotFound(currAttsA, 1)
	assertAttNotFound(prevAttsA, 1)
	assertAttNotFound(currAttsA, 2)
	assertAttNotFound(prevAttsA, 2)
	// State b must be unaffected.
	currAttsB, err = b.CurrentEpochAttestations()
	require.NoError(t, err)
	prevAttsB, err = b.PreviousEpochAttestations()
	require.NoError(t, err)
	assertAttNotFound(currAttsB, 3)
	assertAttNotFound(prevAttsB, 3)
	assertAttFound(currAttsB, 1)
	assertAttFound(prevAttsB, 1)
	assertAttNotFound(currAttsB, 2)
	assertAttNotFound(prevAttsB, 2)

	// Copy on write happened, reference counters are reset.
	assertRefCount(t, a, types.CurrentEpochAttestations, 1)
	assertRefCount(t, b, types.CurrentEpochAttestations, 1)
	assertRefCount(t, a, types.PreviousEpochAttestations, 1)
	assertRefCount(t, b, types.PreviousEpochAttestations, 1)
}

func TestValidatorReferences_RemainsConsistent_Phase0(t *testing.T) {
	s, err := InitializeFromProtoUnsafePhase0(&ethpb.BeaconState{
		Validators: []*ethpb.Validator{
			{PublicKey: []byte{'A'}},
			{PublicKey: []byte{'B'}},
			{PublicKey: []byte{'C'}},
			{PublicKey: []byte{'D'}},
			{PublicKey: []byte{'E'}},
		},
	})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)

	// Create a second state.
	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)

	// Update First Validator.
	assert.NoError(t, a.UpdateValidatorAtIndex(0, &ethpb.Validator{PublicKey: []byte{'Z'}}))

	assert.DeepNotEqual(t, a.Validators()[0], b.Validators()[0], "validators are equal when they are supposed to be different")
	// Modify all validators from copied state.
	assert.NoError(t, b.ApplyToEveryValidator(func(idx int, val state.ReadOnlyValidator) (*ethpb.Validator, error) {
		return &ethpb.Validator{PublicKey: []byte{'V'}}, nil
	}))

	// Ensure reference is properly accounted for.
	assert.NoError(t, a.ReadFromEveryValidator(func(idx int, val state.ReadOnlyValidator) error {
		assert.NotEqual(t, bytesutil.ToBytes48([]byte{'V'}), val.PublicKey())
		return nil
	}))
}

func TestValidatorReferences_RemainsConsistent_Altair(t *testing.T) {
	s, err := InitializeFromProtoUnsafeAltair(&ethpb.BeaconStateAltair{
		Validators: []*ethpb.Validator{
			{PublicKey: []byte{'A'}},
			{PublicKey: []byte{'B'}},
			{PublicKey: []byte{'C'}},
			{PublicKey: []byte{'D'}},
			{PublicKey: []byte{'E'}},
		},
	})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)

	// Create a second state.
	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)

	// Update First Validator.
	assert.NoError(t, a.UpdateValidatorAtIndex(0, &ethpb.Validator{PublicKey: []byte{'Z'}}))

	assert.DeepNotEqual(t, a.Validators()[0], b.Validators()[0], "validators are equal when they are supposed to be different")
	// Modify all validators from copied state.
	assert.NoError(t, b.ApplyToEveryValidator(func(idx int, val state.ReadOnlyValidator) (*ethpb.Validator, error) {
		return &ethpb.Validator{PublicKey: []byte{'V'}}, nil
	}))

	// Ensure reference is properly accounted for.
	assert.NoError(t, a.ReadFromEveryValidator(func(idx int, val state.ReadOnlyValidator) error {
		assert.NotEqual(t, bytesutil.ToBytes48([]byte{'V'}), val.PublicKey())
		return nil
	}))
}

func TestValidatorReferences_RemainsConsistent_Capella(t *testing.T) {
	s, err := InitializeFromProtoUnsafeCapella(&ethpb.BeaconStateCapella{
		Validators: []*ethpb.Validator{
			{PublicKey: []byte{'A'}},
			{PublicKey: []byte{'B'}},
			{PublicKey: []byte{'C'}},
			{PublicKey: []byte{'D'}},
			{PublicKey: []byte{'E'}},
		},
	})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)

	// Create a second state.
	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)

	// Update First Validator.
	assert.NoError(t, a.UpdateValidatorAtIndex(0, &ethpb.Validator{PublicKey: []byte{'Z'}}))

	assert.DeepNotEqual(t, a.Validators()[0], b.Validators()[0], "validators are equal when they are supposed to be different")
	// Modify all validators from copied state.
	assert.NoError(t, b.ApplyToEveryValidator(func(idx int, val state.ReadOnlyValidator) (*ethpb.Validator, error) {
		return &ethpb.Validator{PublicKey: []byte{'V'}}, nil
	}))

	// Ensure reference is properly accounted for.
	assert.NoError(t, a.ReadFromEveryValidator(func(idx int, val state.ReadOnlyValidator) error {
		assert.NotEqual(t, bytesutil.ToBytes48([]byte{'V'}), val.PublicKey())
		return nil
	}))
}

func TestValidatorReferences_RemainsConsistent_Deneb(t *testing.T) {
	s, err := InitializeFromProtoUnsafeDeneb(&ethpb.BeaconStateDeneb{
		Validators: []*ethpb.Validator{
			{PublicKey: []byte{'A'}},
			{PublicKey: []byte{'B'}},
			{PublicKey: []byte{'C'}},
			{PublicKey: []byte{'D'}},
			{PublicKey: []byte{'E'}},
		},
	})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)

	// Create a second state.
	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)

	// Update First Validator.
	assert.NoError(t, a.UpdateValidatorAtIndex(0, &ethpb.Validator{PublicKey: []byte{'Z'}}))

	assert.DeepNotEqual(t, a.Validators()[0], b.Validators()[0], "validators are equal when they are supposed to be different")
	// Modify all validators from copied state.
	assert.NoError(t, b.ApplyToEveryValidator(func(idx int, val state.ReadOnlyValidator) (*ethpb.Validator, error) {
		return &ethpb.Validator{PublicKey: []byte{'V'}}, nil
	}))

	// Ensure reference is properly accounted for.
	assert.NoError(t, a.ReadFromEveryValidator(func(idx int, val state.ReadOnlyValidator) error {
		assert.NotEqual(t, bytesutil.ToBytes48([]byte{'V'}), val.PublicKey())
		return nil
	}))
}

func TestValidatorReferences_RemainsConsistent_Bellatrix(t *testing.T) {
	s, err := InitializeFromProtoUnsafeBellatrix(&ethpb.BeaconStateBellatrix{
		Validators: []*ethpb.Validator{
			{PublicKey: []byte{'A'}},
			{PublicKey: []byte{'B'}},
			{PublicKey: []byte{'C'}},
			{PublicKey: []byte{'D'}},
			{PublicKey: []byte{'E'}},
		},
	})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)

	// Create a second state.
	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)

	// Update First Validator.
	assert.NoError(t, a.UpdateValidatorAtIndex(0, &ethpb.Validator{PublicKey: []byte{'Z'}}))

	assert.DeepNotEqual(t, a.Validators()[0], b.Validators()[0], "validators are equal when they are supposed to be different")
	// Modify all validators from copied state.
	assert.NoError(t, b.ApplyToEveryValidator(func(idx int, val state.ReadOnlyValidator) (*ethpb.Validator, error) {
		return &ethpb.Validator{PublicKey: []byte{'V'}}, nil
	}))

	// Ensure reference is properly accounted for.
	assert.NoError(t, a.ReadFromEveryValidator(func(idx int, val state.ReadOnlyValidator) error {
		assert.NotEqual(t, bytesutil.ToBytes48([]byte{'V'}), val.PublicKey())
		return nil
	}))
}

func TestValidatorReferences_ApplyValidator_BalancesRead(t *testing.T) {
	s, err := InitializeFromProtoUnsafeAltair(&ethpb.BeaconStateAltair{
		Validators: []*ethpb.Validator{
			{PublicKey: []byte{'A'}},
			{PublicKey: []byte{'B'}},
			{PublicKey: []byte{'C'}},
			{PublicKey: []byte{'D'}},
			{PublicKey: []byte{'E'}},
		},
		Balances: []uint64{0, 0, 0, 0, 0},
	})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)

	// Create a second state.
	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)

	// Modify all validators from copied state, it should not deadlock.
	assert.NoError(t, b.ApplyToEveryValidator(func(idx int, val state.ReadOnlyValidator) (*ethpb.Validator, error) {
		b, err := b.BalanceAtIndex(0)
		if err != nil {
			return nil, err
		}
		newVal := val.Copy()
		newVal.EffectiveBalance += b
		return newVal, nil
	}))
}

// assertRefCount checks whether reference count for a given state
// at a given index is equal to expected amount.
func assertRefCount(t *testing.T, b *BeaconState, idx types.FieldIndex, want uint) {
	if cnt := b.sharedFieldReferences[idx].Refs(); cnt != want {
		t.Errorf("Unexpected count of references for index %d, want: %v, got: %v", idx, want, cnt)
	}
}

// assertValFound checks whether item with a given value exists in list.
func assertValFound(t *testing.T, vals []uint64, val uint64) {
	for i := range vals {
		if reflect.DeepEqual(vals[i], val) {
			return
		}
	}
	t.Log(string(debug.Stack()))
	t.Fatalf("Expected value not found (%v), want: %v", vals, val)
}

// assertValNotFound checks whether item with a given value doesn't exist in list.
func assertValNotFound(t *testing.T, vals []uint64, val uint64) {
	for i := range vals {
		if reflect.DeepEqual(vals[i], val) {
			t.Log(string(debug.Stack()))
			t.Errorf("Unexpected value found (%v),: %v", vals, val)
			return
		}
	}
}
