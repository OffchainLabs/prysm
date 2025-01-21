package rlnc

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestTrustedSetup(t *testing.T) {
	// Load the trusted setup.
	committer, err := LoadTrustedSetup()
	require.NoError(t, err)
	require.NotNil(t, committer)
	require.Equal(t, maxChunkSize, uint(committer.num()))
}
