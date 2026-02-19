package stateutil_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestReturnTrieLayer_OK(t *testing.T) {
	newState, _ := util.DeterministicGenesisState(t, 32)
	root, err := stateutil.RootsArrayHashTreeRoot(newState.BlockRoots(), uint64(params.BeaconConfig().SlotsPerHistoricalRoot))
	require.NoError(t, err)
	roots := retrieveBlockRoots(newState)
	nodes, offsets, err := stateutil.ReturnTrieLayer(roots, uint64(len(roots)))
	assert.NoError(t, err)
	depth := len(offsets) - 2
	newRoot := nodes[offsets[depth]]
	assert.Equal(t, root, newRoot)

	nodes, offsets, err = stateutil.ReturnTrieLayer(roots, uint64(len(roots)))
	assert.NoError(t, err)
	depth = len(offsets) - 2
	lastRoot := nodes[offsets[depth]]
	assert.Equal(t, root, lastRoot)
}

func BenchmarkReturnTrieLayer_NormalAlgorithm(b *testing.B) {
	newState, _ := util.DeterministicGenesisState(b, 32)
	root, err := stateutil.RootsArrayHashTreeRoot(newState.BlockRoots(), uint64(params.BeaconConfig().SlotsPerHistoricalRoot))
	require.NoError(b, err)
	roots := retrieveBlockRoots(newState)

	for b.Loop() {
		nodes, offsets, err := stateutil.ReturnTrieLayer(roots, uint64(len(roots)))
		assert.NoError(b, err)
		depth := len(offsets) - 2
		newRoot := nodes[offsets[depth]]
		assert.Equal(b, root, newRoot)
	}
}

func BenchmarkReturnTrieLayer_VectorizedAlgorithm(b *testing.B) {
	newState, _ := util.DeterministicGenesisState(b, 32)
	root, err := stateutil.RootsArrayHashTreeRoot(newState.BlockRoots(), uint64(params.BeaconConfig().SlotsPerHistoricalRoot))
	require.NoError(b, err)
	roots := retrieveBlockRoots(newState)

	for b.Loop() {
		nodes, offsets, err := stateutil.ReturnTrieLayer(roots, uint64(len(roots)))
		assert.NoError(b, err)
		depth := len(offsets) - 2
		newRoot := nodes[offsets[depth]]
		assert.Equal(b, root, newRoot)
	}
}

func TestReturnTrieLayerVariable_OK(t *testing.T) {
	newState, _ := util.DeterministicGenesisState(t, 32)
	root, err := stateutil.ValidatorRegistryRoot(stateutil.CompactValidatorsFromProto(newState.Validators()))
	require.NoError(t, err)
	validators := newState.Validators()
	roots := make([][32]byte, 0, len(validators))
	for _, val := range validators {
		rt, err := stateutil.ValidatorRootWithHasher(val)
		require.NoError(t, err)
		roots = append(roots, rt)
	}
	nodes, offsets := stateutil.ReturnTrieLayerVariable(roots, params.BeaconConfig().ValidatorRegistryLimit)
	depth := len(offsets) - 2
	newRoot := nodes[offsets[depth]]
	newRoot, err = stateutil.AddInMixin(newRoot, uint64(len(validators)))
	require.NoError(t, err)
	assert.Equal(t, root, newRoot)

	nodes, offsets = stateutil.ReturnTrieLayerVariable(roots, params.BeaconConfig().ValidatorRegistryLimit)
	depth = len(offsets) - 2
	lastRoot := nodes[offsets[depth]]
	lastRoot, err = stateutil.AddInMixin(lastRoot, uint64(len(validators)))
	require.NoError(t, err)
	assert.Equal(t, root, lastRoot)

}

func BenchmarkReturnTrieLayerVariable_NormalAlgorithm(b *testing.B) {
	newState, _ := util.DeterministicGenesisState(b, 16000)
	root, err := stateutil.ValidatorRegistryRoot(stateutil.CompactValidatorsFromProto(newState.Validators()))
	require.NoError(b, err)
	validators := newState.Validators()
	roots := make([][32]byte, 0, len(validators))
	for _, val := range validators {
		rt, err := stateutil.ValidatorRootWithHasher(val)
		require.NoError(b, err)
		roots = append(roots, rt)
	}

	for b.Loop() {
		nodes, offsets := stateutil.ReturnTrieLayerVariable(roots, params.BeaconConfig().ValidatorRegistryLimit)
		depth := len(offsets) - 2
		newRoot := nodes[offsets[depth]]
		newRoot, err = stateutil.AddInMixin(newRoot, uint64(len(validators)))
		require.NoError(b, err)
		assert.Equal(b, root, newRoot)
	}
}

func BenchmarkReturnTrieLayerVariable_VectorizedAlgorithm(b *testing.B) {

	newState, _ := util.DeterministicGenesisState(b, 16000)
	root, err := stateutil.ValidatorRegistryRoot(stateutil.CompactValidatorsFromProto(newState.Validators()))
	require.NoError(b, err)
	validators := newState.Validators()
	roots := make([][32]byte, 0, len(validators))
	for _, val := range validators {
		rt, err := stateutil.ValidatorRootWithHasher(val)
		require.NoError(b, err)
		roots = append(roots, rt)
	}

	for b.Loop() {
		nodes, offsets := stateutil.ReturnTrieLayerVariable(roots, params.BeaconConfig().ValidatorRegistryLimit)
		depth := len(offsets) - 2
		newRoot := nodes[offsets[depth]]
		newRoot, err = stateutil.AddInMixin(newRoot, uint64(len(validators)))
		require.NoError(b, err)
		assert.Equal(b, root, newRoot)
	}
}

func TestRecomputeFromLayer_FixedSizedArray(t *testing.T) {
	newState, _ := util.DeterministicGenesisState(t, 32)
	roots := retrieveBlockRoots(newState)

	nodes, offsets, err := stateutil.ReturnTrieLayer(roots, uint64(len(roots)))
	require.NoError(t, err)

	changedIdx := []uint64{24, 41}
	changedRoots := [][32]byte{{'A', 'B', 'C'}, {'D', 'E', 'F'}}
	require.NoError(t, newState.UpdateBlockRootAtIndex(changedIdx[0], changedRoots[0]))
	require.NoError(t, newState.UpdateBlockRootAtIndex(changedIdx[1], changedRoots[1]))

	expectedRoot, err := stateutil.RootsArrayHashTreeRoot(newState.BlockRoots(), uint64(params.BeaconConfig().SlotsPerHistoricalRoot))
	require.NoError(t, err)
	root, err := stateutil.RecomputeFromLayer(changedRoots, changedIdx, nodes, offsets)
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, root)
}

func TestRecomputeFromLayer_VariableSizedArray(t *testing.T) {
	newState, _ := util.DeterministicGenesisState(t, 32)
	validators := newState.Validators()
	roots := make([][32]byte, 0, len(validators))
	for _, val := range validators {
		rt, err := stateutil.ValidatorRootWithHasher(val)
		require.NoError(t, err)
		roots = append(roots, rt)
	}
	nodes, offsets := stateutil.ReturnTrieLayerVariable(roots, params.BeaconConfig().ValidatorRegistryLimit)

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
	roots = make([][32]byte, 0, len(changedVals))
	for _, val := range changedVals {
		rt, err := stateutil.ValidatorRootWithHasher(val)
		require.NoError(t, err)
		roots = append(roots, rt)
	}
	root, _, _, err := stateutil.RecomputeFromLayerVariable(roots, changedIdx, nodes, offsets)
	require.NoError(t, err)
	root, err = stateutil.AddInMixin(root, uint64(len(validators)))
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, root)
}

func TestGrowFlatBuffer_ZeroHashInitialization(t *testing.T) {
	// Build a trie with 4 leaves, then grow to 8 and recompute only leaf 4.
	// This tests that new upper-level entries are initialized to ZeroHashes[level],
	// not [32]byte{}. Without correct initialization, the neighbor at level 1
	// index 3 would be read as [32]byte{} instead of ZeroHashes[1], producing
	// an incorrect root.
	depth := 3
	leaves := [][32]byte{
		{1}, {2}, {3}, {4},
	}
	offsets := stateutil.ComputeOffsetsVariable(depth, len(leaves))
	nodes := make([][32]byte, offsets[depth+1])
	copy(nodes, leaves)
	stateutil.HashUpFromLeaves(nodes, offsets)

	// Compute the expected root by building a full 8-leaf trie from scratch
	// with the 5th leaf set and leaves 5-7 as zero.
	expectedLeaves := make([][32]byte, 8)
	copy(expectedLeaves, leaves)
	expectedLeaves[4] = [32]byte{5}
	expectedOffsets := stateutil.ComputeOffsetsVariable(depth, 8)
	expectedNodes := make([][32]byte, expectedOffsets[depth+1])
	copy(expectedNodes, expectedLeaves)
	stateutil.HashUpFromLeaves(expectedNodes, expectedOffsets)
	expectedRoot := expectedNodes[expectedOffsets[depth]]

	// Grow the original trie from 4 to 8 leaves in one step, then
	// recompute only the branch for leaf 4.
	nodes, offsets = stateutil.GrowFlatBuffer(nodes, offsets, 8)
	changedLeaves := [][32]byte{{5}}
	changedIdx := []uint64{4}
	root, _, _, err := stateutil.RecomputeFromLayerVariable(changedLeaves, changedIdx, nodes, offsets)
	require.NoError(t, err)

	assert.Equal(t, expectedRoot, root,
		"Root mismatch: GrowFlatBuffer must initialize new upper-level entries to ZeroHashes[level]")
}

func TestMerkleizeTrieLeaves_BadHashLayer(t *testing.T) {
	hashLayer := make([][32]byte, 12)
	layers := make([][][32]byte, 20)
	_, _, err := stateutil.MerkleizeTrieLeaves(layers, hashLayer)
	assert.ErrorContains(t, "hash layer is a non power of 2", err)
}

func retrieveBlockRoots(b state.BeaconState) [][32]byte {
	blockRts := b.BlockRoots()
	roots := make([][32]byte, 0, len(blockRts))
	for _, rt := range blockRts {
		roots = append(roots, bytesutil.ToBytes32(rt))
	}
	return roots
}
