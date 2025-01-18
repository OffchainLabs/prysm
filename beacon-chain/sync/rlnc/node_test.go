package rlnc

import (
	"crypto/rand"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestPrepareMessage(t *testing.T) {
	numChunks := uint(10)
	chunkSize := uint(100)
	committer := newCommitter(chunkSize)
	block := make([]byte, numChunks*chunkSize*31)
	_, err := rand.Read(block)
	require.NoError(t, err)
	node, err := NewSource(committer, numChunks, block)
	require.NoError(t, err)
	message, err := node.prepareMessage()
	require.NoError(t, err)
	require.NotNil(t, message)
	require.Equal(t, true, message.Verify(committer))

	emptyNode := NewNode(committer, numChunks)
	_, err = emptyNode.prepareMessage()
	require.ErrorIs(t, ErrNoData, err)
}

func TestReceive(t *testing.T) {
	numChunks := uint(2)
	chunkSize := uint(100)
	committer := newCommitter(chunkSize)
	block := make([]byte, numChunks*chunkSize*31)
	_, err := rand.Read(block)
	require.NoError(t, err)
	node, err := NewSource(committer, numChunks, block)
	require.NoError(t, err)
	// Send one message
	message, err := node.prepareMessage()
	require.NoError(t, err)
	require.NotNil(t, message)
	receiver := NewNode(committer, numChunks)
	require.NoError(t, receiver.receive(message))
	require.Equal(t, 1, len(receiver.chunks))

	// Send another message
	message, err = node.prepareMessage()
	require.NoError(t, err)
	require.NotNil(t, message)
	require.NoError(t, receiver.receive(message))
	require.Equal(t, 2, len(receiver.chunks))

	// The third one should fail
	message, err = node.prepareMessage()
	require.NoError(t, err)
	require.NotNil(t, message)
	require.ErrorIs(t, ErrLinearlyDependentMessage, receiver.receive(message))
	require.Equal(t, 2, len(receiver.chunks))
}
