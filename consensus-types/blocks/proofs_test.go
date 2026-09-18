package blocks

import (
	"testing"

	methodicalssz "github.com/OffchainLabs/methodical-ssz/ssz"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
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

	reset := features.InitWithReset(&features.Flags{DisableProgressiveSSZ: true})
	defer reset()

	legacyRoots, err := ComputeBlockBodyFieldRoots(t.Context(), b)
	require.NoError(t, err)
	require.Equal(t, 13, len(legacyRoots))

	payloadAttestations, err := b.PayloadAttestations()
	require.NoError(t, err)
	expectedLegacyPayloadAttestationsRoot, err := ssz.MerkleizeListSSZ(payloadAttestations, fieldparams.MaxPayloadAttestations)
	require.NoError(t, err)
	require.DeepEqual(t, expectedLegacyPayloadAttestationsRoot[:], legacyRoots[11])

	reset = features.InitWithReset(&features.Flags{})
	defer reset()

	progressiveRoots, err := ComputeBlockBodyFieldRoots(t.Context(), b)
	require.NoError(t, err)
	require.Equal(t, 13, len(progressiveRoots))

	expectedProgressivePayloadAttestationsRoot, err := ssz.MerkleizeListSSZProgressive(payloadAttestations)
	require.NoError(t, err)
	require.DeepEqual(t, expectedProgressivePayloadAttestationsRoot[:], progressiveRoots[11])
	require.DeepNotSSZEqual(t, legacyRoots[11], progressiveRoots[11])

	rootChunks := make([][32]byte, len(progressiveRoots))
	activeFields := make([]bool, len(progressiveRoots))
	for i := range progressiveRoots {
		copy(rootChunks[i][:], progressiveRoots[i])
		activeFields[i] = true
	}
	computedBodyRoot, err := ssz.ContainerRootProgressive(rootChunks, activeFields)
	require.NoError(t, err)
	expectedBodyRoot, err := b.HashTreeRoot()
	require.NoError(t, err)
	require.DeepEqual(t, expectedBodyRoot, computedBodyRoot)
}

func TestPayloadProof(t *testing.T) {
	t.Run("pre-Gloas", func(t *testing.T) {
		body := hydrateBeaconBlockBodyBellatrix()
		block, err := NewBeaconBlock(&eth.BeaconBlockBellatrix{Body: body})
		require.NoError(t, err)

		proof, err := PayloadProof(t.Context(), block)
		require.NoError(t, err)
		bodyRoot, err := block.Body().HashTreeRoot()
		require.NoError(t, err)
		payload, err := block.Body().Execution()
		require.NoError(t, err)
		payloadRoot, err := payload.HashTreeRoot()
		require.NoError(t, err)
		require.Equal(t, true, trie.VerifyMerkleProof(bodyRoot[:], payloadRoot[:], 25, proof))
	})

	t.Run("rejects Gloas", func(t *testing.T) {
		block, err := NewBeaconBlock(&eth.BeaconBlockGloas{Body: hydrateBeaconBlockBodyGloas()})
		require.NoError(t, err)
		_, err = PayloadProof(t.Context(), block)
		require.ErrorContains(t, "use ExecutionBlockHashProof", err)
	})
}

func TestProgressiveContainerActiveFields(t *testing.T) {
	tests := []struct {
		name      string
		container progressiveContainerType
		want      [32]byte
	}{
		{
			name:      "execution payload bid",
			container: executionPayloadBidProgressiveContainer,
			want:      [32]byte{0xff, 0x0f},
		},
		{
			name:      "beacon block body",
			container: beaconBlockBodyProgressiveContainer,
			want:      [32]byte{0xff, 0x1f},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			activeFields, err := progressiveContainerActiveFields(version.Gloas, test.container)
			require.NoError(t, err)
			packed, err := ssz.PackActiveFields(activeFields)
			require.NoError(t, err)
			require.DeepEqual(t, test.want, packed)
		})
	}

	t.Run("unsupported version", func(t *testing.T) {
		_, err := progressiveContainerActiveFields(version.Fulu, beaconBlockBodyProgressiveContainer)
		require.ErrorContains(t, "not defined for fulu", err)
	})

	t.Run("unknown container", func(t *testing.T) {
		_, err := progressiveContainerActiveFields(version.Gloas, progressiveContainerType(2))
		require.ErrorContains(t, "unknown progressive container", err)
	})
}

func TestExecutionBlockHashProof(t *testing.T) {
	newGloasBlock := func(t *testing.T) *BeaconBlock {
		body := hydrateBeaconBlockBodyGloas()
		body.SignedExecutionPayloadBid.Message.ParentBlockHash[0] = 0x42
		body.SignedExecutionPayloadBid.Message.GasLimit = 30_000_000
		block, err := NewBeaconBlock(&eth.BeaconBlockGloas{Body: body})
		require.NoError(t, err)
		wrapped, ok := block.(*BeaconBlock)
		require.Equal(t, true, ok)
		return wrapped
	}

	t.Run("proves Gloas parent block hash", func(t *testing.T) {
		reset := features.InitWithReset(&features.Flags{})
		defer reset()

		block := newGloasBlock(t)
		proof, err := ExecutionBlockHashProof(t.Context(), block)
		require.NoError(t, err)
		require.Equal(t, 11, len(proof))
		bodyRoot, err := block.Body().HashTreeRoot()
		require.NoError(t, err)
		signedBid, err := block.Body().SignedExecutionPayloadBid()
		require.NoError(t, err)
		require.Equal(t, true, trie.VerifyMerkleProof(
			bodyRoot[:], signedBid.Message.ParentBlockHash, 2856, proof,
		))
	})

	t.Run("rejects pre-Gloas block", func(t *testing.T) {
		block, err := NewBeaconBlock(&eth.BeaconBlockBellatrix{Body: hydrateBeaconBlockBodyBellatrix()})
		require.NoError(t, err)
		_, err = ExecutionBlockHashProof(t.Context(), block)
		require.ErrorContains(t, version.String(version.Bellatrix), err)
	})

	t.Run("requires progressive SSZ", func(t *testing.T) {
		reset := features.InitWithReset(&features.Flags{DisableProgressiveSSZ: true})
		defer reset()
		_, err := ExecutionBlockHashProof(t.Context(), newGloasBlock(t))
		require.ErrorContains(t, "requires progressive SSZ", err)
	})

	t.Run("rejects missing bid", func(t *testing.T) {
		reset := features.InitWithReset(&features.Flags{})
		defer reset()
		block := newGloasBlock(t)
		block.body.signedExecutionPayloadBid = nil
		_, err := ExecutionBlockHashProof(t.Context(), block)
		require.ErrorContains(t, "bid is nil", err)
	})

	t.Run("rejects invalid bid field", func(t *testing.T) {
		reset := features.InitWithReset(&features.Flags{})
		defer reset()
		block := newGloasBlock(t)
		block.body.signedExecutionPayloadBid.Message.ParentBlockHash = nil
		_, err := ExecutionBlockHashProof(t.Context(), block)
		require.ErrorIs(t, err, methodicalssz.ErrBytesLength)
	})

	t.Run("rejects invalid commitment", func(t *testing.T) {
		reset := features.InitWithReset(&features.Flags{})
		defer reset()
		block := newGloasBlock(t)
		block.body.signedExecutionPayloadBid.Message.BlobKzgCommitments[0] = nil
		_, err := ExecutionBlockHashProof(t.Context(), block)
		require.ErrorIs(t, err, methodicalssz.ErrBytesLength)
	})

	t.Run("rejects invalid signature", func(t *testing.T) {
		reset := features.InitWithReset(&features.Flags{})
		defer reset()
		block := newGloasBlock(t)
		block.body.signedExecutionPayloadBid.Signature = nil
		_, err := ExecutionBlockHashProof(t.Context(), block)
		require.ErrorIs(t, err, methodicalssz.ErrBytesLength)
	})
}
