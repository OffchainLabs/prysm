package rlnc

import (
	"crypto/rand"
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
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

func TestDecode(t *testing.T) {
	numChunks := uint(3)
	chunkSize := uint(10)
	committer := newCommitter(chunkSize)
	block := make([]byte, numChunks*chunkSize*31)
	_, err := rand.Read(block)
	require.NoError(t, err)
	node, err := NewSource(committer, numChunks, block)
	require.NoError(t, err)
	emptyNode := NewNode(committer, numChunks)

	for i := 0; i < int(numChunks); i++ {
		_, err = emptyNode.decode()
		require.ErrorIs(t, ErrNoData, err)
		message, err := node.prepareMessage()
		require.NoError(t, err)
		require.NoError(t, emptyNode.receive(message))
	}
	decoded, err := emptyNode.decode()
	require.NoError(t, err)
	require.DeepEqual(t, block, decoded)
}
