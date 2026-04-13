package initialsync

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// makeGloasBlock creates a Gloas ROBlock with the given slot, parentRoot, and parentBlockHash in the bid.
func makeGloasBlock(t *testing.T, slot primitives.Slot, parentRoot [32]byte, parentBlockHash [32]byte) blocks.ROBlock {
	blk := util.NewBeaconBlockGloas()
	blk.Block.Slot = slot
	blk.Block.ParentRoot = parentRoot[:]
	blk.Block.Body.SignedExecutionPayloadBid.Message.ParentBlockHash = parentBlockHash[:]
	signed, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	ro, err := blocks.NewROBlock(signed)
	require.NoError(t, err)
	return ro
}

// makeEnvelope creates an ROSignedExecutionPayloadEnvelope with the given slot, blockHash, and parentHash.
func makeEnvelope(t *testing.T, slot primitives.Slot, blockHash [32]byte, parentHash [32]byte) interfaces.ROSignedExecutionPayloadEnvelope {
	env := &ethpb.SignedExecutionPayloadEnvelope{
		Signature: make([]byte, fieldparams.BLSSignatureLength),
		Message: &ethpb.ExecutionPayloadEnvelope{
			Slot:              slot,
			BeaconBlockRoot:   make([]byte, fieldparams.RootLength),
			ExecutionRequests: &enginev1.ExecutionRequests{},
			Payload: &enginev1.ExecutionPayloadDeneb{
				ParentHash:    parentHash[:],
				FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:     make([]byte, fieldparams.RootLength),
				ReceiptsRoot:  make([]byte, fieldparams.RootLength),
				LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:    make([]byte, fieldparams.RootLength),
				BaseFeePerGas: make([]byte, fieldparams.RootLength),
				BlockHash:     blockHash[:],
			},
		},
	}
	wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(env)
	require.NoError(t, err)
	return wrapped
}

func TestCheckAllBlocksBuildOnEmpty(t *testing.T) {
	parentHash := [32]byte{1}
	// Block 0: root will be computed, parentBlockHash = parentHash
	b0 := makeGloasBlock(t, 10, [32]byte{}, parentHash)
	// Block 1: parentRoot = b0.Root(), same parentBlockHash (builds on empty)
	b1 := makeGloasBlock(t, 11, b0.Root(), parentHash)
	// Block 2: parentRoot = b1.Root(), same parentBlockHash (builds on empty)
	b2 := makeGloasBlock(t, 12, b1.Root(), parentHash)

	t.Run("all build on empty", func(t *testing.T) {
		bwb := []blocks.BlockWithROSidecars{
			{Block: b0},
			{Block: b1},
			{Block: b2},
		}
		err := checkAllBlocksBuildOnEmpty(bwb)
		require.NoError(t, err)
	})

	t.Run("block does not descend from previous", func(t *testing.T) {
		// b2's parentRoot is b1.Root(), not b0.Root(), so [b0, b2] is invalid
		bwb := []blocks.BlockWithROSidecars{
			{Block: b0},
			{Block: b2},
		}
		err := checkAllBlocksBuildOnEmpty(bwb)
		require.ErrorContains(t, "does not descend from", err)
	})

	t.Run("different parent block hash", func(t *testing.T) {
		differentHash := [32]byte{2}
		bDiff := makeGloasBlock(t, 11, b0.Root(), differentHash)
		bwb := []blocks.BlockWithROSidecars{
			{Block: b0},
			{Block: bDiff},
		}
		err := checkAllBlocksBuildOnEmpty(bwb)
		require.ErrorContains(t, "does not build on top of the empty block", err)
	})
}

func TestBlockBuiltOnEnvelope(t *testing.T) {
	blockHash := [32]byte{0xaa}
	parentHash := [32]byte{0xbb}

	t.Run("envelope matches block parent hash", func(t *testing.T) {
		env := makeEnvelope(t, 10, blockHash, [32]byte{})
		blk := makeGloasBlock(t, 11, [32]byte{}, blockHash)
		full, err := blocks.BlockBuiltOnEnvelope(env, blk)
		require.NoError(t, err)
		require.Equal(t, true, full)
	})

	t.Run("envelope does not match block parent hash", func(t *testing.T) {
		env := makeEnvelope(t, 10, blockHash, [32]byte{})
		blk := makeGloasBlock(t, 11, [32]byte{}, parentHash)
		full, err := blocks.BlockBuiltOnEnvelope(env, blk)
		require.NoError(t, err)
		require.Equal(t, false, full)
	})
}

func TestFindFirstForkIndex_Gloas(t *testing.T) {
	fulu := util.NewBeaconBlockFulu()
	signedFulu, err := blocks.NewSignedBeaconBlock(fulu)
	require.NoError(t, err)
	roFulu, err := blocks.NewROBlock(signedFulu)
	require.NoError(t, err)

	gloas := util.NewBeaconBlockGloas()
	signedGloas, err := blocks.NewSignedBeaconBlock(gloas)
	require.NoError(t, err)
	roGloas, err := blocks.NewROBlock(signedGloas)
	require.NoError(t, err)

	deneb := util.NewBeaconBlockDeneb()
	signedDeneb, err := blocks.NewSignedBeaconBlock(deneb)
	require.NoError(t, err)
	roDeneb, err := blocks.NewROBlock(signedDeneb)
	require.NoError(t, err)

	t.Run("all pre-Gloas", func(t *testing.T) {
		bwb := []blocks.BlockWithROSidecars{
			{Block: roDeneb},
			{Block: roFulu},
		}
		idx, err := findFirstForkIndex(bwb, version.Gloas)
		require.NoError(t, err)
		require.Equal(t, 2, idx)
	})

	t.Run("all Gloas", func(t *testing.T) {
		bwb := []blocks.BlockWithROSidecars{
			{Block: roGloas},
			{Block: roGloas},
		}
		idx, err := findFirstForkIndex(bwb, version.Gloas)
		require.NoError(t, err)
		require.Equal(t, 0, idx)
	})

	t.Run("mixed correctly sorted", func(t *testing.T) {
		bwb := []blocks.BlockWithROSidecars{
			{Block: roDeneb},
			{Block: roFulu},
			{Block: roGloas},
		}
		idx, err := findFirstForkIndex(bwb, version.Gloas)
		require.NoError(t, err)
		require.Equal(t, 2, idx)
	})

	t.Run("mixed incorrectly sorted", func(t *testing.T) {
		bwb := []blocks.BlockWithROSidecars{
			{Block: roGloas},
			{Block: roFulu},
		}
		_, err := findFirstForkIndex(bwb, version.Gloas)
		require.NotNil(t, err)
	})
}

func TestValidatePayloadBlockConsistency(t *testing.T) {
	// Setup: create a chain of 3 Gloas blocks where each has a different parent hash
	// (meaning each requires an envelope) and envelopes that match.
	hash0 := [32]byte{0x10}
	hash1 := [32]byte{0x20}
	hash2 := [32]byte{0x30}

	// Block 0: parentBlockHash = hash0
	b0 := makeGloasBlock(t, 10, [32]byte{}, hash0)
	// Block 1: parentRoot = b0.Root(), parentBlockHash = hash1 (different from hash0 => needs envelope)
	b1 := makeGloasBlock(t, 11, b0.Root(), hash1)
	// Block 2: parentRoot = b1.Root(), parentBlockHash = hash2 (different from hash1 => needs envelope)
	b2 := makeGloasBlock(t, 12, b1.Root(), hash2)

	// Envelopes: env0 has blockHash=hash1 (matches b1's parentBlockHash)
	// env1 has blockHash=hash2 (matches b2's parentBlockHash)
	env0 := makeEnvelope(t, 10, hash0, [32]byte{})
	env1 := makeEnvelope(t, 11, hash1, hash0)

	t.Run("consistent envelopes and blocks, envelope is first", func(t *testing.T) {
		f := &blocksFetcher{}
		r := &fetchRequestResponse{
			bwb: []blocks.BlockWithROSidecars{
				{Block: b0},
				{Block: b1},
				{Block: b2},
			},
			envelopes: []interfaces.ROSignedExecutionPayloadEnvelope{env0, env1},
		}
		f.validatePayloadBlockConsistency(r)
		require.NoError(t, r.err)
		require.Equal(t, 2, len(r.envelopes))
	})

	t.Run("not enough envelopes truncates blocks", func(t *testing.T) {
		f := &blocksFetcher{}
		r := &fetchRequestResponse{
			bwb: []blocks.BlockWithROSidecars{
				{Block: b0},
				{Block: b1},
				{Block: b2},
			},
			// Only one envelope, but two are needed
			envelopes: []interfaces.ROSignedExecutionPayloadEnvelope{env0},
		}
		f.validatePayloadBlockConsistency(r)
		// Should truncate bwb to the point where envelopes run out
		require.NoError(t, r.err)
	})

	t.Run("extra envelopes truncated", func(t *testing.T) {
		env2 := makeEnvelope(t, 12, hash2, hash1)
		f := &blocksFetcher{}
		// All blocks have the same parentBlockHash => no envelope transitions needed
		sameHash := [32]byte{0x99}
		sb0 := makeGloasBlock(t, 10, [32]byte{}, sameHash)
		sb1 := makeGloasBlock(t, 11, sb0.Root(), sameHash)

		envFirst := makeEnvelope(t, 10, sameHash, [32]byte{})
		r := &fetchRequestResponse{
			bwb: []blocks.BlockWithROSidecars{
				{Block: sb0},
				{Block: sb1},
			},
			envelopes: []interfaces.ROSignedExecutionPayloadEnvelope{envFirst, env2},
		}
		f.validatePayloadBlockConsistency(r)
		require.NoError(t, r.err)
		// Extra envelope should be truncated
		require.Equal(t, 1, len(r.envelopes))
	})

	t.Run("mismatched envelope sets error", func(t *testing.T) {
		wrongEnv := makeEnvelope(t, 10, [32]byte{0xff}, [32]byte{})
		f := &blocksFetcher{}
		r := &fetchRequestResponse{
			blocksFrom:   "peer1",
			payloadsFrom: "peer2",
			bwb: []blocks.BlockWithROSidecars{
				{Block: b0},
				{Block: b1},
			},
			envelopes: []interfaces.ROSignedExecutionPayloadEnvelope{wrongEnv},
		}
		f.validatePayloadBlockConsistency(r)
		require.ErrorContains(t, "envelope does not match block", r.err)
	})
}
