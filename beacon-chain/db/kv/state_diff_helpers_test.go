package kv

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestMakeKeyForStateDiffTree_KeyLength(t *testing.T) {
	// Existing databases store state diff keys at this exact length. Changing
	// it would make all persisted keys unreadable on restart.
	key := makeKeyForStateDiffTree(0, 0)
	require.Equal(t, 16, len(key))

	key = makeKeyForStateDiffTree(3, 1<<40)
	require.Equal(t, 16, len(key))
}
