package util

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestInclusionProofs(t *testing.T) {
	_, blobs := GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 0, params.BeaconConfig().MaxBlobsPerBlock(0))
	for i := range blobs {
		require.NoError(t, blocks.VerifyKZGInclusionProof(blobs[i]))
	}
}
