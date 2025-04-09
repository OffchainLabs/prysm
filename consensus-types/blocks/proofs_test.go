package blocks

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/container/trie"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestComputeBlockBodyFieldRoots_Phase0(t *testing.T) {
	blockBodyPhase0 := hydrateBeaconBlockBody()
	i, err := NewBeaconBlockBody(blockBodyPhase0)
	require.NoError(t, err)

	b, ok := i.(*BeaconBlockBody)
	require.Equal(t, true, ok)

	fieldRoots, err := ComputeBlockBodyFieldRoots(context.Background(), b)
	require.NoError(t, err)
	trie, err := trie.GenerateTrieFromItems(fieldRoots, 3)
	require.NoError(t, err)
	layers := trie.ToProto().GetLayers()

	hash := layers[len(layers)-1].Layer[0]
	require.NoError(t, err)

	correctHash, err := b.HashTreeRoot()
	require.NoError(t, err)

	require.DeepEqual(t, correctHash[:], hash)
}

func TestComputeBlockBodyFieldRoots_Altair(t *testing.T) {
	blockBodyAltair := hydrateBeaconBlockBodyAltair()
	i, err := NewBeaconBlockBody(blockBodyAltair)
	require.NoError(t, err)

	b, ok := i.(*BeaconBlockBody)
	require.Equal(t, true, ok)

	fieldRoots, err := ComputeBlockBodyFieldRoots(context.Background(), b)
	require.NoError(t, err)
	trie, err := trie.GenerateTrieFromItems(fieldRoots, 4)
	require.NoError(t, err)
	layers := trie.ToProto().GetLayers()

	hash := layers[len(layers)-1].Layer[0]
	require.NoError(t, err)

	correctHash, err := b.HashTreeRoot()
	require.NoError(t, err)

	require.DeepEqual(t, correctHash[:], hash)
}

func TestComputeBlockBodyFieldRoots_Bellatrix(t *testing.T) {
	blockBodyBellatrix := hydrateBeaconBlockBodyBellatrix()
	i, err := NewBeaconBlockBody(blockBodyBellatrix)
	require.NoError(t, err)

	b, ok := i.(*BeaconBlockBody)
	require.Equal(t, true, ok)

	fieldRoots, err := ComputeBlockBodyFieldRoots(context.Background(), b)
	require.NoError(t, err)
	trie, err := trie.GenerateTrieFromItems(fieldRoots, 4)
	require.NoError(t, err)
	layers := trie.ToProto().GetLayers()

	hash := layers[len(layers)-1].Layer[0]
	require.NoError(t, err)

	correctHash, err := b.HashTreeRoot()
	require.NoError(t, err)

	require.DeepEqual(t, correctHash[:], hash)
}

func TestComputeBlockBodyFieldRoots_Capella(t *testing.T) {
	blockBodyCapella := hydrateBeaconBlockBodyCapella()
	i, err := NewBeaconBlockBody(blockBodyCapella)
	require.NoError(t, err)

	b, ok := i.(*BeaconBlockBody)
	require.Equal(t, true, ok)

	fieldRoots, err := ComputeBlockBodyFieldRoots(context.Background(), b)
	require.NoError(t, err)
	trie, err := trie.GenerateTrieFromItems(fieldRoots, 4)
	require.NoError(t, err)
	layers := trie.ToProto().GetLayers()

	hash := layers[len(layers)-1].Layer[0]
	require.NoError(t, err)

	correctHash, err := b.HashTreeRoot()
	require.NoError(t, err)

	require.DeepEqual(t, correctHash[:], hash)
}

func TestComputeBlockBodyFieldRoots_Deneb(t *testing.T) {
	blockBodyDeneb := hydrateBeaconBlockBodyDeneb()
	i, err := NewBeaconBlockBody(blockBodyDeneb)
	require.NoError(t, err)

	b, ok := i.(*BeaconBlockBody)
	require.Equal(t, true, ok)

	fieldRoots, err := ComputeBlockBodyFieldRoots(context.Background(), b)
	require.NoError(t, err)
	trie, err := trie.GenerateTrieFromItems(fieldRoots, 4)
	require.NoError(t, err)
	layers := trie.ToProto().GetLayers()

	hash := layers[len(layers)-1].Layer[0]
	require.NoError(t, err)

	correctHash, err := b.HashTreeRoot()
	require.NoError(t, err)

	require.DeepEqual(t, correctHash[:], hash)
}

func TestComputeBlockBodyFieldRoots_Electra(t *testing.T) {
	blockBodyElectra := hydrateBeaconBlockBodyElectra()
	i, err := NewBeaconBlockBody(blockBodyElectra)
	require.NoError(t, err)

	b, ok := i.(*BeaconBlockBody)
	require.Equal(t, true, ok)

	fieldRoots, err := ComputeBlockBodyFieldRoots(context.Background(), b)
	require.NoError(t, err)
	trie, err := trie.GenerateTrieFromItems(fieldRoots, 4)
	require.NoError(t, err)
	layers := trie.ToProto().GetLayers()

	hash := layers[len(layers)-1].Layer[0]
	require.NoError(t, err)

	correctHash, err := b.HashTreeRoot()
	require.NoError(t, err)

	require.DeepEqual(t, correctHash[:], hash)
}
