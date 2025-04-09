//go:build !fuzz

package cache

import (
	"testing"

	forkchoicetypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/types"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestProposerCache_Set(t *testing.T) {
	cache := NewProposerIndicesCache()
	bRoot := [32]byte{'A'}
	indices, ok := cache.ProposerIndices(0, bRoot)
	require.Equal(t, false, ok)
	emptyIndices := [fieldparams.SlotsPerEpoch]primitives.ValidatorIndex{}
	require.Equal(t, indices, emptyIndices, "Expected committee count not to exist in empty cache")
	emptyIndices[0] = 1
	cache.Set(0, bRoot, emptyIndices)

	received, ok := cache.ProposerIndices(0, bRoot)
	require.Equal(t, true, ok)
	require.Equal(t, received, emptyIndices)

	newRoot := [32]byte{'B'}
	copy(emptyIndices[3:], []primitives.ValidatorIndex{1, 2, 3, 4, 5, 6})
	cache.Set(0, newRoot, emptyIndices)

	received, ok = cache.ProposerIndices(0, newRoot)
	require.Equal(t, true, ok)
	require.Equal(t, emptyIndices, received)
}

func TestProposerCache_CheckpointAndPrune(t *testing.T) {
	cache := NewProposerIndicesCache()
	indices := [fieldparams.SlotsPerEpoch]primitives.ValidatorIndex{}
	copy(indices[3:], []primitives.ValidatorIndex{1, 2, 3, 4, 5, 6})
	for i := 1; i < 10; i++ {
		root := [32]byte{byte(i)}
		cache.Set(primitives.Epoch(i), root, indices)
		cpRoot := [32]byte{byte(i - 1)}
		cache.SetCheckpoint(forkchoicetypes.Checkpoint{Epoch: primitives.Epoch(i - 1), Root: cpRoot}, root)
	}
	received, ok := cache.ProposerIndices(1, [32]byte{1})
	require.Equal(t, true, ok)
	require.Equal(t, indices, received)

	received, ok = cache.ProposerIndices(4, [32]byte{4})
	require.Equal(t, true, ok)
	require.Equal(t, indices, received)

	received, ok = cache.ProposerIndices(9, [32]byte{9})
	require.Equal(t, true, ok)
	require.Equal(t, indices, received)

	received, ok = cache.IndicesFromCheckpoint(forkchoicetypes.Checkpoint{})
	require.Equal(t, true, ok)
	require.Equal(t, indices, received)

	received, ok = cache.IndicesFromCheckpoint(forkchoicetypes.Checkpoint{Epoch: 3, Root: [32]byte{3}})
	require.Equal(t, true, ok)
	require.Equal(t, indices, received)

	received, ok = cache.IndicesFromCheckpoint(forkchoicetypes.Checkpoint{Epoch: 4, Root: [32]byte{4}})
	require.Equal(t, true, ok)
	require.Equal(t, indices, received)

	received, ok = cache.IndicesFromCheckpoint(forkchoicetypes.Checkpoint{Epoch: 8, Root: [32]byte{8}})
	require.Equal(t, true, ok)
	require.Equal(t, indices, received)

	cache.Prune(5)

	emptyIndices := [fieldparams.SlotsPerEpoch]primitives.ValidatorIndex{}
	received, ok = cache.ProposerIndices(1, [32]byte{1})
	require.Equal(t, false, ok)
	require.Equal(t, emptyIndices, received)

	received, ok = cache.ProposerIndices(4, [32]byte{4})
	require.Equal(t, false, ok)
	require.Equal(t, emptyIndices, received)

	received, ok = cache.ProposerIndices(9, [32]byte{9})
	require.Equal(t, true, ok)
	require.Equal(t, indices, received)

	received, ok = cache.IndicesFromCheckpoint(forkchoicetypes.Checkpoint{Epoch: 0, Root: [32]byte{0}})
	require.Equal(t, false, ok)
	require.Equal(t, emptyIndices, received)

	received, ok = cache.IndicesFromCheckpoint(forkchoicetypes.Checkpoint{Epoch: 3, Root: [32]byte{3}})
	require.Equal(t, false, ok)
	require.Equal(t, emptyIndices, received)

	received, ok = cache.IndicesFromCheckpoint(forkchoicetypes.Checkpoint{Epoch: 4, Root: [32]byte{4}})
	require.Equal(t, true, ok)
	require.Equal(t, indices, received)

	received, ok = cache.IndicesFromCheckpoint(forkchoicetypes.Checkpoint{Epoch: 8, Root: [32]byte{8}})
	require.Equal(t, true, ok)
	require.Equal(t, indices, received)
}
