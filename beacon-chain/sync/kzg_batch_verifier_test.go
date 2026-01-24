package sync

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func createValidTestDataColumns(t *testing.T, count int) []blocks.RODataColumn {
	_, roSidecars, _ := util.GenerateTestFuluBlockWithSidecars(t, count)
	if len(roSidecars) >= count {
		return roSidecars[:count]
	}
	return roSidecars
}

func createInvalidTestDataColumns(t *testing.T, count int) []blocks.RODataColumn {
	dataColumns := createValidTestDataColumns(t, count)

	if len(dataColumns) > 0 {
		sidecar := dataColumns[0].DataColumnSidecar
		if len(sidecar.Column) > 0 && len(sidecar.Column[0]) > 0 {
			corruptedSidecar := &ethpb.DataColumnSidecar{
				Index:                        sidecar.Index,
				KzgCommitments:               make([][]byte, len(sidecar.KzgCommitments)),
				KzgProofs:                    make([][]byte, len(sidecar.KzgProofs)),
				KzgCommitmentsInclusionProof: make([][]byte, len(sidecar.KzgCommitmentsInclusionProof)),
				SignedBlockHeader:            sidecar.SignedBlockHeader,
				Column:                       make([][]byte, len(sidecar.Column)),
			}

			for i, commitment := range sidecar.KzgCommitments {
				corruptedSidecar.KzgCommitments[i] = make([]byte, len(commitment))
				copy(corruptedSidecar.KzgCommitments[i], commitment)
			}

			for i, proof := range sidecar.KzgProofs {
				corruptedSidecar.KzgProofs[i] = make([]byte, len(proof))
				copy(corruptedSidecar.KzgProofs[i], proof)
			}

			for i, proof := range sidecar.KzgCommitmentsInclusionProof {
				corruptedSidecar.KzgCommitmentsInclusionProof[i] = make([]byte, len(proof))
				copy(corruptedSidecar.KzgCommitmentsInclusionProof[i], proof)
			}

			for i, col := range sidecar.Column {
				corruptedSidecar.Column[i] = make([]byte, len(col))
				copy(corruptedSidecar.Column[i], col)
			}
			corruptedSidecar.Column[0][0] ^= 0xFF // Flip bits to corrupt

			corruptedRO, err := blocks.NewRODataColumn(corruptedSidecar)
			require.NoError(t, err)
			dataColumns[0] = corruptedRO
		}
	}
	return dataColumns
}
