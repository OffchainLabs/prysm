package peerdas_test

// Test helpers for reconstruction tests

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// testBlobSetup holds common test data for blob reconstruction tests.
type testBlobSetup struct {
	blobCount                    int
	blobs                        []kzg.Blob
	roBlock                      blocks.ROBlock
	roDataColumnSidecars         []blocks.RODataColumn
	verifiedRoDataColumnSidecars []blocks.VerifiedRODataColumn
}

// setupTestBlobs creates a complete test setup with blobs, cells, proofs, and data column sidecars.
func setupTestBlobs(t *testing.T, blobCount int) *testBlobSetup {
	_, roBlobSidecars := util.GenerateTestElectraBlockWithSidecar(t, [32]byte{}, 42, blobCount)

	blobs := make([]kzg.Blob, blobCount)
	for i := range blobCount {
		copy(blobs[i][:], roBlobSidecars[i].Blob)
	}

	cellsPerBlob, proofsPerBlob := util.GenerateCellsAndProofs(t, blobs)

	fs := util.SlotAtEpoch(t, params.BeaconConfig().FuluForkEpoch)
	roBlock, _, _ := util.GenerateTestFuluBlockWithSidecars(t, blobCount, util.WithSlot(fs))

	roDataColumnSidecars, err := peerdas.DataColumnSidecars(cellsPerBlob, proofsPerBlob, peerdas.PopulateFromBlock(roBlock))
	require.NoError(t, err)

	verifiedRoSidecars := toVerifiedSidecars(roDataColumnSidecars)

	return &testBlobSetup{
		blobCount:                    blobCount,
		blobs:                        blobs,
		roBlock:                      roBlock,
		roDataColumnSidecars:         roDataColumnSidecars,
		verifiedRoDataColumnSidecars: verifiedRoSidecars,
	}
}

// toVerifiedSidecars converts a slice of RODataColumn to VerifiedRODataColumn.
func toVerifiedSidecars(roDataColumnSidecars []blocks.RODataColumn) []blocks.VerifiedRODataColumn {
	verifiedRoSidecars := make([]blocks.VerifiedRODataColumn, 0, len(roDataColumnSidecars))
	for _, roDataColumnSidecar := range roDataColumnSidecars {
		verifiedRoSidecar := blocks.NewVerifiedRODataColumn(roDataColumnSidecar)
		verifiedRoSidecars = append(verifiedRoSidecars, verifiedRoSidecar)
	}
	return verifiedRoSidecars
}

// filterEvenIndexedSidecars returns only the even-indexed sidecars (0, 2, 4, ...).
// This is useful for forcing reconstruction in tests.
func filterEvenIndexedSidecars(sidecars []blocks.VerifiedRODataColumn) []blocks.VerifiedRODataColumn {
	filtered := make([]blocks.VerifiedRODataColumn, 0, len(sidecars)/2)
	for i := 0; i < len(sidecars); i += 2 {
		filtered = append(filtered, sidecars[i])
	}
	return filtered
}

// setupFuluForkEpoch sets up the test configuration with Fulu fork after Electra.
func setupFuluForkEpoch(t *testing.T) primitives.Slot {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().FuluForkEpoch = params.BeaconConfig().ElectraForkEpoch + 4096*2
	return util.SlotAtEpoch(t, params.BeaconConfig().FuluForkEpoch)
}
