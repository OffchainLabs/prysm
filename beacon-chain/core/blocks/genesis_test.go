package blocks_test

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestGenesisBlock_InitializedCorrectly(t *testing.T) {
	stateHash := bytesutil.PadTo([]byte{0}, 32)
	b1 := blocks.NewGenesisBlock(stateHash)

	assert.NotNil(t, b1.Block.ParentRoot, "Genesis block missing ParentHash field")
	assert.DeepEqual(t, b1.Block.StateRoot, stateHash, "Genesis block StateRootHash32 isn't initialized correctly")
}

func TestHezeGenesisBlock(t *testing.T) {
	root := [32]byte{0x01}

	t.Run("nil bid gives a zero bid", func(t *testing.T) {
		blk := blocks.HezeGenesisBlock(root, nil)
		require.DeepEqual(t, root[:], blk.StateRoot)
		bid := blk.Body.SignedExecutionPayloadBid.Message
		require.DeepEqual(t, make([]byte, 32), bid.BlockHash)
		require.DeepEqual(t, bitfield.NewBitvector16(), bid.InclusionListBits)
		_, err := blk.HashTreeRoot()
		require.NoError(t, err)
	})

	t.Run("copies the state bid", func(t *testing.T) {
		latest := &ethpb.ExecutionPayloadBidHeze{
			ParentBlockHash:       make([]byte, 32),
			ParentBlockRoot:       make([]byte, 32),
			BlockHash:             bytes.Repeat([]byte{0xAB}, 32),
			PrevRandao:            make([]byte, 32),
			FeeRecipient:          make([]byte, 20),
			ExecutionRequestsRoot: make([]byte, 32),
			InclusionListBits:     []byte{0x01, 0x80},
		}
		blk := blocks.HezeGenesisBlock(root, latest)
		bid := blk.Body.SignedExecutionPayloadBid.Message
		require.DeepEqual(t, latest, bid)
		latest.InclusionListBits[0] = 0xFF
		require.DeepEqual(t, bitfield.Bitvector16{0x01, 0x80}, bid.InclusionListBits)
	})
}
