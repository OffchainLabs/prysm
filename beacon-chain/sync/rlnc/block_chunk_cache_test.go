package rlnc

import (
	"crypto/rand"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/chunks"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestBlockChunkCache(t *testing.T) {
	// Create a new block chunk cache.
	committer := newCommitter(10)
	cache := NewBlockChunkCache(committer)
	require.NotNil(t, cache)

	require.Equal(t, 0, len(cache.nodes))

	chunkSize := uint(4)
	block := make([]byte, features.Get().RLNCNumChunks*chunkSize*31)
	_, err := rand.Read(block)
	require.NoError(t, err)
	node, err := NewSource(committer, features.Get().RLNCNumChunks, block)
	require.NoError(t, err)

	// Prepare a message
	msg, err := node.PrepareMessage()
	require.NoError(t, err)
	require.NotNil(t, msg)
	chunkProto := &ethpb.BeaconBlockChunk{
		Data:         msg.Data(),
		Coefficients: msg.Coefficients(),
		Header: &ethpb.BeaconBlockChunkHeader{
			Slot:          1,
			ProposerIndex: 1,
			ParentRoot:    make([]byte, 32),
			Commitments:   msg.Commitments(),
		},
		Signature: make([]byte, 96),
	}

	chunk, err := chunks.NewBlockChunk(chunkProto)
	require.NoError(t, err)
	// Add the chunk to the cache.
	require.ErrorIs(t, ErrSignatureNotVerified, cache.AddChunk(chunk))
	require.Equal(t, 1, len(cache.nodes))

	// Prepare a second message
	msg, err = node.PrepareMessage()
	require.NoError(t, err)
	require.NotNil(t, msg)
	chunkProto = &ethpb.BeaconBlockChunk{
		Data:         msg.Data(),
		Coefficients: msg.Coefficients(),
		Header: &ethpb.BeaconBlockChunkHeader{
			Slot:          1,
			ProposerIndex: 1,
			ParentRoot:    make([]byte, 32),
			Commitments:   msg.Commitments(),
		},
		Signature: make([]byte, 96),
	}

	chunk, err = chunks.NewBlockChunk(chunkProto)
	require.NoError(t, err)
	// Add the chunk to the cache
	require.NoError(t, cache.AddChunk(chunk)) // No error this time as the signature was verified before
	require.Equal(t, 1, len(cache.nodes))     // No new node, same block chunk
	cachedNode := cache.nodes[1][1]
	require.Equal(t, 2, len(cachedNode.chunks))

	message, err := cache.PrepareMessage(chunk)
	require.NoError(t, err)
	require.DeepEqual(t, message.Header.Commitments, chunkProto.Header.Commitments)
}
