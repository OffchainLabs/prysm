package util

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestInclusionProofs(t *testing.T) {
	_, blobs := GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 0, params.BeaconConfig().MaxBlobsPerBlock(0))
	for i := range blobs {
		require.NoError(t, blocks.VerifyKZGInclusionProof(blobs[i]))
	}
}
