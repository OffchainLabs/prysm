package fieldtrie_test

import (
	"encoding/binary"
	"math"
	"testing"

	. "github.com/OffchainLabs/prysm/v7/beacon-chain/state/fieldtrie"
	customtypes "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/custom-types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	mvslice "github.com/OffchainLabs/prysm/v7/container/multi-value-slice"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestFieldTrie_NewTrie(t *testing.T) {
	t.Run("native state", func(t *testing.T) {
		runNewTrie(t)
	})
}

func runNewTrie(t *testing.T) {
	newState, _ := util.DeterministicGenesisState(t, 40)
	roots := newState.BlockRoots()
	blockRoots := make([][32]byte, len(roots))
	for i, r := range roots {
		blockRoots[i] = [32]byte(r)
	}
	mvRoots := buildTestCompositeSlice[[32]byte](blockRoots)
	elements := mvslice.MultiValueSliceComposite[[32]byte]{
		Identifiable:    mockIdentifier{},
		MultiValueSlice: mvRoots,
	}

	trie, err := NewFieldTrie(types.BlockRoots, types.BasicArray, elements, uint64(params.BeaconConfig().SlotsPerHistoricalRoot))
	require.NoError(t, err)
	root, err := stateutil.RootsArrayHashTreeRoot(newState.BlockRoots(), uint64(params.BeaconConfig().SlotsPerHistoricalRoot))
	require.NoError(t, err)
	newRoot, err := trie.TrieRoot()
	require.NoError(t, err)
	assert.Equal(t, root, newRoot)
}

func TestFieldTrie_NewTrie_NilElements(t *testing.T) {
	trie, err := NewFieldTrie(types.BlockRoots, types.BasicArray, nil, 8234)
	require.NoError(t, err)
	_, err = trie.TrieRoot()
	require.ErrorIs(t, err, ErrEmptyFieldTrie)
}

func TestFieldTrie_RecomputeTrie(t *testing.T) {
	t.Run("native state", func(t *testing.T) {
		runRecomputeTrie(t)
	})
}

func runRecomputeTrie(t *testing.T) {
	newState, _ := util.DeterministicGenesisState(t, 32)

	compactVals := stateutil.CompactValidatorsFromProto(newState.Validators())
	mvRoots := buildTestCompositeSlice[stateutil.CompactValidator](compactVals)
	elements := mvslice.MultiValueSliceComposite[stateutil.CompactValidator]{
		Identifiable:    mockIdentifier{},
		MultiValueSlice: mvRoots,
	}

	trie, err := NewFieldTrie(types.Validators, types.CompositeArray, elements, params.BeaconConfig().ValidatorRegistryLimit)
	require.NoError(t, err)

	oldroot, err := trie.TrieRoot()
	require.NoError(t, err)
	require.NotEmpty(t, oldroot)

	changedIdx := []uint64{2, 29}
	val1, err := newState.ValidatorAtIndex(10)
	require.NoError(t, err)
	val2, err := newState.ValidatorAtIndex(11)
	require.NoError(t, err)
	val1.Slashed = true
	val1.ExitEpoch = 20

	val2.Slashed = true
	val2.ExitEpoch = 40

	changedVals := []*ethpb.Validator{val1, val2}
	require.NoError(t, newState.UpdateValidatorAtIndex(primitives.ValidatorIndex(changedIdx[0]), changedVals[0]))
	require.NoError(t, newState.UpdateValidatorAtIndex(primitives.ValidatorIndex(changedIdx[1]), changedVals[1]))

	expectedRoot, err := stateutil.ValidatorRegistryRoot(stateutil.CompactValidatorsFromProto(newState.Validators()))
	require.NoError(t, err)
	root, err := trie.RecomputeTrie(changedIdx, stateutil.CompactValidatorsFromProto(newState.Validators()))
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, root)
}

func TestFieldTrie_RecomputeTrie_CompressedArray(t *testing.T) {
	t.Run("native state", func(t *testing.T) {
		runRecomputeTrie_CompressedArray(t)
	})
}

func runRecomputeTrie_CompressedArray(t *testing.T) {
	newState, _ := util.DeterministicGenesisState(t, 32)
	mvBals := buildTestCompositeSlice(newState.Balances())
	elements := mvslice.MultiValueSliceComposite[uint64]{
		Identifiable:    mockIdentifier{},
		MultiValueSlice: mvBals,
	}

	trie, err := NewFieldTrie(types.Balances, types.CompressedArray, elements, stateutil.ValidatorLimitForBalancesChunks())
	require.NoError(t, err)
	require.Equal(t, trie.Length(), stateutil.ValidatorLimitForBalancesChunks())
	changedIdx := []uint64{4, 8}
	require.NoError(t, newState.UpdateBalancesAtIndex(primitives.ValidatorIndex(changedIdx[0]), uint64(100000000)))
	require.NoError(t, newState.UpdateBalancesAtIndex(primitives.ValidatorIndex(changedIdx[1]), uint64(200000000)))
	expectedRoot, err := stateutil.Uint64ListRootWithRegistryLimit(newState.Balances())
	require.NoError(t, err)
	root, err := trie.RecomputeTrie(changedIdx, newState.Balances())
	require.NoError(t, err)

	// not equal for some reason :(
	assert.Equal(t, expectedRoot, root)
}

func TestNewFieldTrie_UnknownType(t *testing.T) {
	newState, _ := util.DeterministicGenesisState(t, 32)
	_, err := NewFieldTrie(types.Balances, 4, newState.Balances(), 32)
	require.ErrorContains(t, "unrecognized data type", err)
}

func TestFieldTrie_CopyTrieImmutable(t *testing.T) {
	// CopyTrie uses lazy copy-on-write. The production contract is:
	// after CopyTrie, the COPY may be mutated while the ORIGINAL stays
	// unchanged. This test verifies mutating the copy does not affect the original.
	newState, _ := util.DeterministicGenesisState(t, 32)
	mixes := newState.RandaoMixes()
	randaoMixes := make([][32]byte, len(mixes))
	for i, r := range mixes {
		randaoMixes[i] = [32]byte(r)
	}

	originalTrie, err := NewFieldTrie(types.RandaoMixes, types.BasicArray, customtypes.RandaoMixes(randaoMixes), uint64(params.BeaconConfig().EpochsPerHistoricalVector))
	require.NoError(t, err)
	originalRoot, err := originalTrie.TrieRoot()
	require.NoError(t, err)

	copiedTrie := originalTrie.CopyTrie()

	// Verify the copy initially has the same root.
	copyRoot, err := copiedTrie.TrieRoot()
	require.NoError(t, err)
	require.Equal(t, originalRoot, copyRoot, "copy should initially have same root")

	// Mutate the COPY (the production pattern).
	changedIdx := []uint64{2, 29}
	changedVals := [][32]byte{{'A', 'B'}, {'C', 'D'}}
	require.NoError(t, newState.UpdateRandaoMixesAtIndex(changedIdx[0], changedVals[0]))
	require.NoError(t, newState.UpdateRandaoMixesAtIndex(changedIdx[1], changedVals[1]))
	mixes = newState.RandaoMixes()
	randaoMixes = make([][32]byte, len(mixes))
	for i, r := range mixes {
		randaoMixes[i] = [32]byte(r)
	}
	mutatedRoot, err := copiedTrie.RecomputeTrie(changedIdx, customtypes.RandaoMixes(randaoMixes))
	require.NoError(t, err)

	// The original should be unchanged.
	rootAfter, err := originalTrie.TrieRoot()
	require.NoError(t, err)
	require.Equal(t, originalRoot, rootAfter, "original should be unchanged after copy mutation")

	// The mutated copy should have a different root.
	if mutatedRoot == originalRoot {
		t.Errorf("Wanted roots to be different, but they are the same: %#x", mutatedRoot)
	}
}

func TestFieldTrie_CopyAndTransferEmpty(t *testing.T) {
	trie, err := NewFieldTrie(types.RandaoMixes, types.BasicArray, nil, uint64(params.BeaconConfig().EpochsPerHistoricalVector))
	require.NoError(t, err)

	require.DeepEqual(t, trie, trie.CopyTrie())
	require.DeepEqual(t, trie, trie.TransferTrie())
}

func TestFieldTrie_TransferTrie(t *testing.T) {
	newState, _ := util.DeterministicGenesisState(t, 32)
	maxLength := (params.BeaconConfig().ValidatorRegistryLimit*8 + 31) / 32
	trie, err := NewFieldTrie(types.Balances, types.CompressedArray, newState.Balances(), maxLength)
	require.NoError(t, err)
	oldRoot, err := trie.TrieRoot()
	require.NoError(t, err)

	newTrie := trie.TransferTrie()
	root, err := trie.TrieRoot()
	require.ErrorIs(t, err, ErrEmptyFieldTrie)
	require.Equal(t, root, [32]byte{})
	require.NotNil(t, newTrie)
	newRoot, err := newTrie.TrieRoot()
	require.NoError(t, err)
	require.DeepEqual(t, oldRoot, newRoot)
}

func FuzzFieldTrie(f *testing.F) {
	newState, _ := util.DeterministicGenesisState(f, 40)
	var data []byte
	for _, root := range newState.StateRoots() {
		data = append(data, root...)
	}
	f.Add(5, int(types.BasicArray), data, uint64(params.BeaconConfig().SlotsPerHistoricalRoot))

	f.Fuzz(func(t *testing.T, idx, typ int, data []byte, slotsPerHistRoot uint64) {
		var roots [][]byte
		for i := 32; i < len(data); i += 32 {
			roots = append(roots, data[i-32:i])
		}
		trie, err := NewFieldTrie(types.FieldIndex(idx), types.DataType(typ), roots, slotsPerHistRoot)
		if err != nil {
			return // invalid inputs
		}
		_, err = trie.TrieRoot()
		if err != nil {
			return
		}
	})
}

func TestOverlayCreation(t *testing.T) {
	newState, _ := util.DeterministicGenesisState(t, 32)
	mixes := newState.RandaoMixes()
	randaoMixes := make([][32]byte, len(mixes))
	for i, r := range mixes {
		randaoMixes[i] = [32]byte(r)
	}

	owned, err := NewFieldTrie(types.RandaoMixes, types.BasicArray, customtypes.RandaoMixes(randaoMixes), uint64(params.BeaconConfig().EpochsPerHistoricalVector))
	require.NoError(t, err)
	require.Equal(t, false, owned.IsOverlay())

	ownedRoot, err := owned.TrieRoot()
	require.NoError(t, err)

	overlay := owned.CopyTrie()
	require.Equal(t, true, overlay.IsOverlay())

	overlayRoot, err := overlay.TrieRoot()
	require.NoError(t, err)
	require.Equal(t, ownedRoot, overlayRoot, "overlay root should match owned root")
}

func TestOverlayCopy(t *testing.T) {
	newState, _ := util.DeterministicGenesisState(t, 32)
	compactVals := stateutil.CompactValidatorsFromProto(newState.Validators())
	mvRoots := buildTestCompositeSlice[stateutil.CompactValidator](compactVals)
	elements := mvslice.MultiValueSliceComposite[stateutil.CompactValidator]{
		Identifiable:    mockIdentifier{},
		MultiValueSlice: mvRoots,
	}

	owned, err := NewFieldTrie(types.Validators, types.CompositeArray, elements, params.BeaconConfig().ValidatorRegistryLimit)
	require.NoError(t, err)
	ownedRoot, err := owned.TrieRoot()
	require.NoError(t, err)

	// Create overlay on owned.
	overlay1 := owned.CopyTrie()
	require.Equal(t, true, overlay1.IsOverlay())

	// Create overlay on overlay (copy of copy).
	overlay2 := overlay1.CopyTrie()
	require.Equal(t, true, overlay2.IsOverlay())

	overlay2Root, err := overlay2.TrieRoot()
	require.NoError(t, err)
	require.Equal(t, ownedRoot, overlay2Root, "overlay-on-overlay root should match owned root")
}

func TestOverlayRecompute_CompositeArray(t *testing.T) {
	newState, _ := util.DeterministicGenesisState(t, 32)
	compactVals := stateutil.CompactValidatorsFromProto(newState.Validators())
	mvRoots := buildTestCompositeSlice[stateutil.CompactValidator](compactVals)
	elements := mvslice.MultiValueSliceComposite[stateutil.CompactValidator]{
		Identifiable:    mockIdentifier{},
		MultiValueSlice: mvRoots,
	}

	// Build two identical tries — one owned, one overlay.
	owned, err := NewFieldTrie(types.Validators, types.CompositeArray, elements, params.BeaconConfig().ValidatorRegistryLimit)
	require.NoError(t, err)
	overlay := owned.CopyTrie()
	require.Equal(t, true, overlay.IsOverlay())

	// Mutate validators.
	changedIdx := []uint64{2, 29}
	val1, err := newState.ValidatorAtIndex(10)
	require.NoError(t, err)
	val2, err := newState.ValidatorAtIndex(11)
	require.NoError(t, err)
	val1.Slashed = true
	val1.ExitEpoch = 20
	val2.Slashed = true
	val2.ExitEpoch = 40

	require.NoError(t, newState.UpdateValidatorAtIndex(primitives.ValidatorIndex(changedIdx[0]), val1))
	require.NoError(t, newState.UpdateValidatorAtIndex(primitives.ValidatorIndex(changedIdx[1]), val2))
	newCompactVals := stateutil.CompactValidatorsFromProto(newState.Validators())

	// Compute expected root from scratch.
	expectedRoot, err := stateutil.ValidatorRegistryRoot(newCompactVals)
	require.NoError(t, err)

	// Recompute on overlay.
	overlayRoot, err := overlay.RecomputeTrie(changedIdx, newCompactVals)
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, overlayRoot, "overlay recompute should match reference root")
}

func TestOverlayRecompute_CompressedArray(t *testing.T) {
	newState, _ := util.DeterministicGenesisState(t, 32)
	mvBals := buildTestCompositeSlice(newState.Balances())
	elements := mvslice.MultiValueSliceComposite[uint64]{
		Identifiable:    mockIdentifier{},
		MultiValueSlice: mvBals,
	}

	owned, err := NewFieldTrie(types.Balances, types.CompressedArray, elements, stateutil.ValidatorLimitForBalancesChunks())
	require.NoError(t, err)
	overlay := owned.CopyTrie()
	require.Equal(t, true, overlay.IsOverlay())

	changedIdx := []uint64{4, 8}
	require.NoError(t, newState.UpdateBalancesAtIndex(primitives.ValidatorIndex(changedIdx[0]), uint64(100000000)))
	require.NoError(t, newState.UpdateBalancesAtIndex(primitives.ValidatorIndex(changedIdx[1]), uint64(200000000)))

	expectedRoot, err := stateutil.Uint64ListRootWithRegistryLimit(newState.Balances())
	require.NoError(t, err)

	overlayRoot, err := overlay.RecomputeTrie(changedIdx, newState.Balances())
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, overlayRoot, "overlay compressed array recompute should match reference root")
}

func TestOverlayRecompute_BasicArray(t *testing.T) {
	newState, _ := util.DeterministicGenesisState(t, 32)
	mixes := newState.RandaoMixes()
	randaoMixes := make([][32]byte, len(mixes))
	for i, r := range mixes {
		randaoMixes[i] = [32]byte(r)
	}

	owned, err := NewFieldTrie(types.RandaoMixes, types.BasicArray, customtypes.RandaoMixes(randaoMixes), uint64(params.BeaconConfig().EpochsPerHistoricalVector))
	require.NoError(t, err)
	overlay := owned.CopyTrie()
	require.Equal(t, true, overlay.IsOverlay())

	changedIdx := []uint64{2, 29}
	changedVals := [][32]byte{{'A', 'B'}, {'C', 'D'}}
	require.NoError(t, newState.UpdateRandaoMixesAtIndex(changedIdx[0], changedVals[0]))
	require.NoError(t, newState.UpdateRandaoMixesAtIndex(changedIdx[1], changedVals[1]))
	mixes = newState.RandaoMixes()
	randaoMixes = make([][32]byte, len(mixes))
	for i, r := range mixes {
		randaoMixes[i] = [32]byte(r)
	}

	// Compute expected root from scratch.
	expectedRoot, err := stateutil.RootsArrayHashTreeRoot(newState.RandaoMixes(), uint64(params.BeaconConfig().EpochsPerHistoricalVector))
	require.NoError(t, err)

	overlayRoot, err := overlay.RecomputeTrie(changedIdx, customtypes.RandaoMixes(randaoMixes))
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, overlayRoot, "overlay basic array recompute should match reference root")
}

func TestOverlayImmutability(t *testing.T) {
	newState, _ := util.DeterministicGenesisState(t, 32)
	compactVals := stateutil.CompactValidatorsFromProto(newState.Validators())
	mvRoots := buildTestCompositeSlice[stateutil.CompactValidator](compactVals)
	elements := mvslice.MultiValueSliceComposite[stateutil.CompactValidator]{
		Identifiable:    mockIdentifier{},
		MultiValueSlice: mvRoots,
	}

	base, err := NewFieldTrie(types.Validators, types.CompositeArray, elements, params.BeaconConfig().ValidatorRegistryLimit)
	require.NoError(t, err)
	baseRoot, err := base.TrieRoot()
	require.NoError(t, err)

	// Two overlays from same base.
	overlay1 := base.CopyTrie()
	overlay2 := base.CopyTrie()

	// Mutate overlay1.
	val1, err := newState.ValidatorAtIndex(10)
	require.NoError(t, err)
	val1.Slashed = true
	val1.ExitEpoch = 20
	require.NoError(t, newState.UpdateValidatorAtIndex(2, val1))
	newCompactVals := stateutil.CompactValidatorsFromProto(newState.Validators())

	_, err = overlay1.RecomputeTrie([]uint64{2}, newCompactVals)
	require.NoError(t, err)

	// Base should be unchanged.
	baseRootAfter, err := base.TrieRoot()
	require.NoError(t, err)
	require.Equal(t, baseRoot, baseRootAfter, "base should be unchanged after overlay mutation")

	// Sibling overlay should be unchanged.
	overlay2Root, err := overlay2.TrieRoot()
	require.NoError(t, err)
	require.Equal(t, baseRoot, overlay2Root, "sibling overlay should be unchanged")
}

func TestOverlayAllIndicesDirty(t *testing.T) {
	// Verify correctness when all indices are dirty (simulates epoch boundary
	// for small validator sets where promotion threshold isn't reached).
	newState, _ := util.DeterministicGenesisState(t, 32)
	compactVals := stateutil.CompactValidatorsFromProto(newState.Validators())
	mvRoots := buildTestCompositeSlice[stateutil.CompactValidator](compactVals)
	elements := mvslice.MultiValueSliceComposite[stateutil.CompactValidator]{
		Identifiable:    mockIdentifier{},
		MultiValueSlice: mvRoots,
	}

	owned, err := NewFieldTrie(types.Validators, types.CompositeArray, elements, params.BeaconConfig().ValidatorRegistryLimit)
	require.NoError(t, err)
	overlay := owned.CopyTrie()
	require.Equal(t, true, overlay.IsOverlay())

	// All indices dirty.
	numVals := len(newState.Validators())
	allIdx := make([]uint64, numVals)
	for i := range allIdx {
		allIdx[i] = uint64(i)
	}

	expectedRoot, err := stateutil.ValidatorRegistryRoot(compactVals)
	require.NoError(t, err)

	overlayRoot, err := overlay.RecomputeTrie(allIdx, compactVals)
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, overlayRoot, "overlay with all indices dirty should produce correct root")
}

// TestOverlayNoSpuriousRebuild verifies that overlays are not prematurely
// rebuilt when only a small fraction of leaves change across multiple rounds.
//
// The overlay promotion threshold (OverlayPromotionThreshold=10K) should be
// checked against the leaf-level overlay count (overrides[0]), NOT the total
// overlay size across all trie levels. Each dirty leaf propagates entries up
// through the entire trie depth during recomputeOverlay, so the total overlay
// size grows at ~2x the leaf rate. For deep tries (validators: depth=40),
// this caused premature rebuilds when the total crossed 10K even though only
// a few thousand leaves had actually changed.
func TestOverlayNoSpuriousRebuild(t *testing.T) {
	t.Run("CompositeArray_Validators", testOverlayNoSpuriousRebuild_Validators)
	t.Run("BasicArray_BlockRoots", testOverlayNoSpuriousRebuild_BlockRoots)
	t.Run("CompressedArray_Balances", testOverlayNoSpuriousRebuild_Balances)
}

func testOverlayNoSpuriousRebuild_Validators(t *testing.T) {
	// Validators use CompositeArray with depth=40 (ValidatorRegistryLimit=2^40).
	// This is the most affected case: each dirty leaf creates entries at ~20
	// intermediate levels, so overlaySize() grows at ~2x the leaf rate.
	const numVals = 5000

	vals := make([]stateutil.CompactValidator, numVals)
	for i := range vals {
		var pk [48]byte
		binary.BigEndian.PutUint64(pk[:], uint64(i))
		var wc [32]byte
		wc[0] = 0x01
		binary.BigEndian.PutUint64(wc[12:], uint64(i))
		vals[i] = stateutil.CompactValidator{
			PublicKey:                  pk,
			WithdrawalCredentials:      wc,
			EffectiveBalance:           32000000000,
			ActivationEligibilityEpoch: 0,
			ActivationEpoch:            0,
			ExitEpoch:                  primitives.Epoch(math.MaxUint64),
			WithdrawableEpoch:          primitives.Epoch(math.MaxUint64),
		}
	}

	mvSlice := buildTestCompositeSlice(vals)
	elements := mvslice.MultiValueSliceComposite[stateutil.CompactValidator]{
		Identifiable:    mockIdentifier{},
		MultiValueSlice: mvSlice,
	}
	owned, err := NewFieldTrie(types.Validators, types.CompositeArray, elements, params.BeaconConfig().ValidatorRegistryLimit)
	require.NoError(t, err)
	overlay := owned.CopyTrie()
	require.Equal(t, true, overlay.IsOverlay())

	// 15 rounds x 500 dirty = up to 5000 unique dirty leaves (all of them).
	// Total overlay entries across all 40 levels is approximately 10K+, but
	// the leaf-level count is 5000. The old overlaySize() check would have
	// triggered rebuild; the leaf-only check does not.
	const rounds = 15
	const dirtyPerRound = 500

	var lastRoot [32]byte
	for round := range rounds {
		start := (round * dirtyPerRound) % numVals
		dirtyIdx := make([]uint64, dirtyPerRound)
		for i := range dirtyPerRound {
			idx := (start + i) % numVals
			dirtyIdx[i] = uint64(idx)
			vals[idx].EffectiveBalance = uint64(32000000000 + round*1000 + i)
		}
		root, err := overlay.RecomputeTrie(dirtyIdx, vals)
		require.NoError(t, err)
		require.NotEqual(t, [32]byte{}, root)
		lastRoot = root
	}

	// Overlay must NOT have been rebuilt.
	require.Equal(t, true, overlay.IsOverlay(),
		"overlay should not have been rebuilt — leaf overlay count is under threshold")

	// Verify correctness against from-scratch computation.
	expectedRoot, err := stateutil.ValidatorRegistryRoot(vals)
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, lastRoot,
		"overlay root should match from-scratch computation after many small updates")
}

func testOverlayNoSpuriousRebuild_BlockRoots(t *testing.T) {
	// BlockRoots use BasicArray with depth=13 (SlotsPerHistoricalRoot=8192).
	// Shallower than validators, but the overlay amplification (~2x leaf rate)
	// still causes overlaySize() to exceed 10K before leaf count does.
	const numRoots = 8192

	roots := make(customtypes.BlockRoots, numRoots)
	for i := range roots {
		binary.BigEndian.PutUint64(roots[i][:], uint64(i))
	}

	owned, err := NewFieldTrie(types.BlockRoots, types.BasicArray, roots, uint64(params.BeaconConfig().SlotsPerHistoricalRoot))
	require.NoError(t, err)
	overlay := owned.CopyTrie()
	require.Equal(t, true, overlay.IsOverlay())

	// 15 rounds x 600 dirty = up to 8192 unique dirty leaves.
	// Total overlay across 13 levels is approximately 2x the leaf count. When
	// the leaf count reaches ~5500, overlaySize() exceeds 10K but the leaf
	// count is still below the threshold.
	const rounds = 15
	const dirtyPerRound = 600

	var lastRoot [32]byte
	for round := range rounds {
		start := (round * dirtyPerRound) % numRoots
		dirtyIdx := make([]uint64, dirtyPerRound)
		for i := range dirtyPerRound {
			idx := (start + i) % numRoots
			dirtyIdx[i] = uint64(idx)
			binary.BigEndian.PutUint64(roots[idx][:], uint64(round*numRoots+idx))
		}
		root, err := overlay.RecomputeTrie(dirtyIdx, roots)
		require.NoError(t, err)
		require.NotEqual(t, [32]byte{}, root)
		lastRoot = root
	}

	require.Equal(t, true, overlay.IsOverlay(),
		"overlay should not have been rebuilt — leaf overlay count is under threshold")

	// Verify correctness: convert to [][]byte for reference root.
	rootsSlice := make([][]byte, numRoots)
	for i, r := range roots {
		cp := r
		rootsSlice[i] = cp[:]
	}
	expectedRoot, err := stateutil.RootsArrayHashTreeRoot(rootsSlice, uint64(params.BeaconConfig().SlotsPerHistoricalRoot))
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, lastRoot,
		"overlay root should match from-scratch computation after many small updates")
}

func testOverlayNoSpuriousRebuild_Balances(t *testing.T) {
	// Balances use CompressedArray with depth=38 (ValidatorLimitForBalancesChunks).
	// 4 uint64 balances pack into one 32-byte chunk, so 40000 balances = 10000
	// chunks. With 10000 unique chunks dirty, overlaySize() significantly
	// exceeds 10K across all levels (~20K total), but the leaf overlay count
	// stays at 10000 (not > threshold since check is strict >).
	const numBals = 40000

	bals := make([]uint64, numBals)
	for i := range bals {
		bals[i] = 32000000000
	}

	owned, err := NewFieldTrie(types.Balances, types.CompressedArray, bals, stateutil.ValidatorLimitForBalancesChunks())
	require.NoError(t, err)
	overlay := owned.CopyTrie()
	require.Equal(t, true, overlay.IsOverlay())

	// 20 rounds x 2000 dirty balances = 40000 balance indices = 10000 unique
	// chunks. Total overlay across 38 levels is approximately 2x the chunk
	// count (~20K), well above the 10K threshold.
	const rounds = 20
	const dirtyPerRound = 2000

	var lastRoot [32]byte
	for round := range rounds {
		start := (round * dirtyPerRound) % numBals
		dirtyIdx := make([]uint64, dirtyPerRound)
		for i := range dirtyPerRound {
			idx := (start + i) % numBals
			dirtyIdx[i] = uint64(idx)
			bals[idx] = uint64(32000000000 + round*1000 + i)
		}
		root, err := overlay.RecomputeTrie(dirtyIdx, bals)
		require.NoError(t, err)
		require.NotEqual(t, [32]byte{}, root)
		lastRoot = root
	}

	require.Equal(t, true, overlay.IsOverlay(),
		"overlay should not have been rebuilt — leaf overlay count is under threshold")

	// Verify correctness.
	expectedRoot, err := stateutil.Uint64ListRootWithRegistryLimit(bals)
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, lastRoot,
		"overlay root should match from-scratch computation after many small updates")
}

func buildTestCompositeSlice[V comparable](values []V) mvslice.MultiValueSliceComposite[V] {
	obj := &mvslice.Slice[V]{}
	mock := mockIdentifier{}
	obj.Init(values, mock.Id())
	return mvslice.MultiValueSliceComposite[V]{
		Identifiable:    mock,
		MultiValueSlice: obj,
	}
}

type mockIdentifier struct{}

func (_ mockIdentifier) Id() mvslice.Id {
	return 0
}
