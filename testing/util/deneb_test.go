package util

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestInclusionProofs(t *testing.T) {
	ds := SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)
	_, blobs := GenerateTestDenebBlockWithSidecar(t, [32]byte{}, ds, params.BeaconConfig().MaxBlobsPerBlock(ds))
	for i := range blobs {
		require.NoError(t, blocks.VerifyKZGInclusionProof(blobs[i]))
	}
}
