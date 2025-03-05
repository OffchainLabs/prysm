package util

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestFuluBlockInclusionProofs(t *testing.T) {
	err := kzg.Start()
	require.NoError(t, err)

	_, columns := GenerateTestFuluBlockWithSidecar(t, [32]byte{}, 0, 1)
	for _, col := range columns {
		require.NoError(t, blocks.VerifyKZGInclusionProofColumn(col))
	}
}
