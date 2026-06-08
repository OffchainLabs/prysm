package blocks

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestComputeBlockBodyFieldRoots_Phase0(t *testing.T) {
	blockBodyPhase0 := hydrateBeaconBlockBody()
	i, err := NewBeaconBlockBody(blockBodyPhase0)
	require.NoError(t, err)

	b, ok := i.(*BeaconBlockBody)
	require.Equal(t, true, ok)

	fieldRoots, err := ComputeBlockBodyFieldRoots(t.Context(), b)
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

	fieldRoots, err := ComputeBlockBodyFieldRoots(t.Context(), b)
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

	fieldRoots, err := ComputeBlockBodyFieldRoots(t.Context(), b)
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

	fieldRoots, err := ComputeBlockBodyFieldRoots(t.Context(), b)
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

	fieldRoots, err := ComputeBlockBodyFieldRoots(t.Context(), b)
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

	fieldRoots, err := ComputeBlockBodyFieldRoots(t.Context(), b)
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

func TestComputeBlockBodyFieldRoots_Gloas_ProgressiveSSZGate(t *testing.T) {
	blockBodyGloas := hydrateBeaconBlockBodyGloas()
	i, err := NewBeaconBlockBody(blockBodyGloas)
	require.NoError(t, err)

	b, ok := i.(*BeaconBlockBody)
	require.Equal(t, true, ok)

	reset := features.InitWithReset(&features.Flags{})
	defer reset()

	legacyRoots, err := ComputeBlockBodyFieldRoots(t.Context(), b)
	require.NoError(t, err)
	require.Equal(t, 13, len(legacyRoots))

	payloadAttestations, err := b.PayloadAttestations()
	require.NoError(t, err)
	expectedLegacyPayloadAttestationsRoot, err := ssz.MerkleizeListSSZ(payloadAttestations, fieldparams.MaxPayloadAttestations)
	require.NoError(t, err)
	require.DeepEqual(t, expectedLegacyPayloadAttestationsRoot[:], legacyRoots[11])

	reset = features.InitWithReset(&features.Flags{EnableProgressiveSSZ: true})
	defer reset()

	progressiveRoots, err := ComputeBlockBodyFieldRoots(t.Context(), b)
	require.NoError(t, err)
	require.Equal(t, 13, len(progressiveRoots))

	expectedProgressivePayloadAttestationsRoot, err := ssz.MerkleizeListSSZProgressive(payloadAttestations)
	require.NoError(t, err)
	require.DeepEqual(t, expectedProgressivePayloadAttestationsRoot[:], progressiveRoots[11])
	require.DeepNotSSZEqual(t, legacyRoots[11], progressiveRoots[11])
}
