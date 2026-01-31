package blocks_test

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	consensus_types "github.com/OffchainLabs/prysm/v7/consensus-types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func validExecutionPayloadBid() *ethpb.ExecutionPayloadBid {
	return &ethpb.ExecutionPayloadBid{
		ParentBlockHash:        bytes.Repeat([]byte{0x01}, 32),
		ParentBlockRoot:        bytes.Repeat([]byte{0x02}, 32),
		BlockHash:              bytes.Repeat([]byte{0x03}, 32),
		PrevRandao:             bytes.Repeat([]byte{0x04}, 32),
		GasLimit:               123,
		BuilderIndex:           5,
		Slot:                   6,
		Value:                  7,
		ExecutionPayment:       8,
		BlobKzgCommitments:     [][]byte{bytes.Repeat([]byte{0x05}, 48)},
		FeeRecipient:           bytes.Repeat([]byte{0x06}, 20),
	}
}

func TestWrappedROExecutionPayloadBid(t *testing.T) {
	t.Run("returns error on invalid lengths", func(t *testing.T) {
		testCases := []struct {
			name   string
			mutate func(*ethpb.ExecutionPayloadBid)
		}{
			{
				name:   "parent block hash",
				mutate: func(b *ethpb.ExecutionPayloadBid) { b.ParentBlockHash = []byte{0x01} },
			},
			{
				name:   "parent block root",
				mutate: func(b *ethpb.ExecutionPayloadBid) { b.ParentBlockRoot = []byte{0x02} },
			},
			{
				name:   "block hash",
				mutate: func(b *ethpb.ExecutionPayloadBid) { b.BlockHash = []byte{0x03} },
			},
			{
				name:   "prev randao",
				mutate: func(b *ethpb.ExecutionPayloadBid) { b.PrevRandao = []byte{0x04} },
			},
			{
				name:   "blob kzg commitments length",
				mutate: func(b *ethpb.ExecutionPayloadBid) { b.BlobKzgCommitments = [][]byte{[]byte{0x05}} },
			},
			{
				name:   "fee recipient",
				mutate: func(b *ethpb.ExecutionPayloadBid) { b.FeeRecipient = []byte{0x06} },
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				invalid := validExecutionPayloadBid()
				tc.mutate(invalid)

				_, err := blocks.WrappedROExecutionPayloadBid(invalid)
				require.Equal(t, consensus_types.ErrNilObjectWrapped, err)
			})
		}
	})

	t.Run("wraps and exposes fields", func(t *testing.T) {
		bid := validExecutionPayloadBid()
		wrapped, err := blocks.WrappedROExecutionPayloadBid(bid)
		require.NoError(t, err)

		require.Equal(t, primitives.BuilderIndex(5), wrapped.BuilderIndex())
		require.Equal(t, primitives.Slot(6), wrapped.Slot())
		require.Equal(t, primitives.Gwei(7), wrapped.Value())
		require.Equal(t, primitives.Gwei(8), wrapped.ExecutionPayment())
		assert.DeepEqual(t, [32]byte(bytes.Repeat([]byte{0x01}, 32)), wrapped.ParentBlockHash())
		assert.DeepEqual(t, [32]byte(bytes.Repeat([]byte{0x02}, 32)), wrapped.ParentBlockRoot())
		assert.DeepEqual(t, [32]byte(bytes.Repeat([]byte{0x03}, 32)), wrapped.BlockHash())
		assert.DeepEqual(t, [32]byte(bytes.Repeat([]byte{0x04}, 32)), wrapped.PrevRandao())
		assert.DeepEqual(t, [][]byte{bytes.Repeat([]byte{0x05}, 48)}, wrapped.BlobKzgCommitments())
		assert.DeepEqual(t, [20]byte(bytes.Repeat([]byte{0x06}, 20)), wrapped.FeeRecipient())
	})
}

func TestWrappedROSignedExecutionPayloadBid(t *testing.T) {
	t.Run("returns error for invalid signature length", func(t *testing.T) {
		signed := &ethpb.SignedExecutionPayloadBid{
			Message:   validExecutionPayloadBid(),
			Signature: bytes.Repeat([]byte{0xAA}, 95),
		}
		_, err := blocks.WrappedROSignedExecutionPayloadBid(signed)
		require.Equal(t, consensus_types.ErrNilObjectWrapped, err)
	})

	t.Run("wraps and provides bid/signing data", func(t *testing.T) {
		sig := bytes.Repeat([]byte{0xAB}, 96)
		signed := &ethpb.SignedExecutionPayloadBid{
			Message:   validExecutionPayloadBid(),
			Signature: sig,
		}

		wrapped, err := blocks.WrappedROSignedExecutionPayloadBid(signed)
		require.NoError(t, err)

		bid, err := wrapped.Bid()
		require.NoError(t, err)
		require.Equal(t, primitives.Gwei(8), bid.ExecutionPayment())

		gotSig := wrapped.Signature()
		assert.DeepEqual(t, [96]byte(sig), gotSig)

		domain := bytes.Repeat([]byte{0xCC}, 32)
		wantRoot, err := signing.ComputeSigningRoot(signed.Message, domain)
		require.NoError(t, err)
		gotRoot, err := wrapped.SigningRoot(domain)
		require.NoError(t, err)
		require.Equal(t, wantRoot, gotRoot)
	})
}
