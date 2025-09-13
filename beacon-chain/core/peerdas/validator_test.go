package peerdas_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestValidatorsCustodyRequirement(t *testing.T) {
	testCases := []struct {
		name     string
		count    uint64
		expected uint64
	}{
		{name: "0 validators", count: 0, expected: 8},
		{name: "1 validator", count: 1, expected: 8},
		{name: "8 validators", count: 8, expected: 8},
		{name: "9 validators", count: 9, expected: 9},
		{name: "100 validators", count: 100, expected: 100},
		{name: "128 validators", count: 128, expected: 128},
		{name: "129 validators", count: 129, expected: 128},
		{name: "1000 validators", count: 1000, expected: 128},
	}

	const balance = uint64(32_000_000_000)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validators := make([]*ethpb.Validator, 0, tc.count)
			for range tc.count {
				validator := &ethpb.Validator{
					EffectiveBalance: balance,
				}

				validators = append(validators, validator)
			}

			validatorsIndex := make(map[primitives.ValidatorIndex]bool)
			for i := range tc.count {
				validatorsIndex[primitives.ValidatorIndex(i)] = true
			}

			beaconState, err := state_native.InitializeFromProtoFulu(&ethpb.BeaconStateFulu{Validators: validators})
			require.NoError(t, err)

			actual, err := peerdas.ValidatorsCustodyRequirement(beaconState, validatorsIndex)
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestDataColumnSidecarsFromBlock(t *testing.T) {
	t.Run("sizes mismatch", func(t *testing.T) {
		// Create a protobuf signed beacon block.
		signedBeaconBlockPb := util.NewBeaconBlockDeneb()

		// Create a signed beacon block from the protobuf.
		signedBeaconBlock, err := blocks.NewSignedBeaconBlock(signedBeaconBlockPb)
		require.NoError(t, err)

		// Create cells and proofs.
		cellsAndProofs := []kzg.CellsAndProofs{
			{
				Cells:  make([]kzg.Cell, params.BeaconConfig().NumberOfColumns),
				Proofs: make([]kzg.Proof, params.BeaconConfig().NumberOfColumns),
			},
		}

		rob, err := blocks.NewROBlock(signedBeaconBlock)
		require.NoError(t, err)
		_, err = peerdas.ConstructDataColumnSidecar(cellsAndProofs, peerdas.PopulateFromBlock(rob))
		require.ErrorIs(t, err, peerdas.ErrSizeMismatch)
	})

	t.Run("cells array too short for column index", func(t *testing.T) {
		// Create a Fulu block with a blob commitment.
		signedBeaconBlockPb := util.NewBeaconBlockFulu()
		signedBeaconBlockPb.Block.Body.BlobKzgCommitments = [][]byte{make([]byte, 48)}

		// Create a signed beacon block from the protobuf.
		signedBeaconBlock, err := blocks.NewSignedBeaconBlock(signedBeaconBlockPb)
		require.NoError(t, err)

		// Create cells and proofs with insufficient cells for the number of columns.
		// This simulates a scenario where cellsAndProofs has fewer cells than expected columns.
		cellsAndProofs := []kzg.CellsAndProofs{
			{
				Cells:  make([]kzg.Cell, 10),  // Only 10 cells
				Proofs: make([]kzg.Proof, 10), // Only 10 proofs
			},
		}

		// This should fail because the function will try to access columns up to NumberOfColumns
		// but we only have 10 cells/proofs.
		rob, err := blocks.NewROBlock(signedBeaconBlock)
		require.NoError(t, err)
		_, err = peerdas.ConstructDataColumnSidecar(cellsAndProofs, peerdas.PopulateFromBlock(rob))
		require.ErrorIs(t, err, peerdas.ErrNotEnoughDataColumnSidecars)
	})

	t.Run("proofs array too short for column index", func(t *testing.T) {
		// Create a Fulu block with a blob commitment.
		signedBeaconBlockPb := util.NewBeaconBlockFulu()
		signedBeaconBlockPb.Block.Body.BlobKzgCommitments = [][]byte{make([]byte, 48)}

		// Create a signed beacon block from the protobuf.
		signedBeaconBlock, err := blocks.NewSignedBeaconBlock(signedBeaconBlockPb)
		require.NoError(t, err)

		// Create cells and proofs with sufficient cells but insufficient proofs.
		numberOfColumns := params.BeaconConfig().NumberOfColumns
		cellsAndProofs := []kzg.CellsAndProofs{
			{
				Cells:  make([]kzg.Cell, numberOfColumns),
				Proofs: make([]kzg.Proof, 5), // Only 5 proofs, less than columns
			},
		}

		// This should fail when trying to access proof beyond index 4.
		rob, err := blocks.NewROBlock(signedBeaconBlock)
		require.NoError(t, err)
		_, err = peerdas.ConstructDataColumnSidecar(cellsAndProofs, peerdas.PopulateFromBlock(rob))
		require.ErrorIs(t, err, peerdas.ErrNotEnoughDataColumnSidecars)
		require.ErrorContains(t, "not enough proofs", err)
	})

	t.Run("nominal", func(t *testing.T) {
		// Create a Fulu block with blob commitments.
		signedBeaconBlockPb := util.NewBeaconBlockFulu()
		commitment1 := make([]byte, 48)
		commitment2 := make([]byte, 48)

		// Set different values to distinguish commitments
		commitment1[0] = 0x01
		commitment2[0] = 0x02
		signedBeaconBlockPb.Block.Body.BlobKzgCommitments = [][]byte{commitment1, commitment2}

		// Create a signed beacon block from the protobuf.
		signedBeaconBlock, err := blocks.NewSignedBeaconBlock(signedBeaconBlockPb)
		require.NoError(t, err)

		// Create cells and proofs with correct dimensions.
		numberOfColumns := params.BeaconConfig().NumberOfColumns
		cellsAndProofs := []kzg.CellsAndProofs{
			{
				Cells:  make([]kzg.Cell, numberOfColumns),
				Proofs: make([]kzg.Proof, numberOfColumns),
			},
			{
				Cells:  make([]kzg.Cell, numberOfColumns),
				Proofs: make([]kzg.Proof, numberOfColumns),
			},
		}

		// Set distinct values in cells and proofs for testing
		for i := range numberOfColumns {
			cellsAndProofs[0].Cells[i][0] = byte(i)
			cellsAndProofs[0].Proofs[i][0] = byte(i)
			cellsAndProofs[1].Cells[i][0] = byte(i + 128)
			cellsAndProofs[1].Proofs[i][0] = byte(i + 128)
		}

		rob, err := blocks.NewROBlock(signedBeaconBlock)
		require.NoError(t, err)
		sidecars, err := peerdas.ConstructDataColumnSidecar(cellsAndProofs, peerdas.PopulateFromBlock(rob))
		require.NoError(t, err)
		require.NotNil(t, sidecars)
		require.Equal(t, int(numberOfColumns), len(sidecars))

		// Verify each sidecar has the expected structure
		for i, sidecar := range sidecars {
			require.Equal(t, uint64(i), sidecar.Index)
			require.Equal(t, 2, len(sidecar.Column))
			require.Equal(t, 2, len(sidecar.KzgCommitments))
			require.Equal(t, 2, len(sidecar.KzgProofs))

			// Verify commitments match what we set
			require.DeepEqual(t, commitment1, sidecar.KzgCommitments[0])
			require.DeepEqual(t, commitment2, sidecar.KzgCommitments[1])

			// Verify column data comes from the correct cells
			require.Equal(t, byte(i), sidecar.Column[0][0])
			require.Equal(t, byte(i+128), sidecar.Column[1][0])

			// Verify proofs come from the correct proofs
			require.Equal(t, byte(i), sidecar.KzgProofs[0][0])
			require.Equal(t, byte(i+128), sidecar.KzgProofs[1][0])
		}
	})
}

func TestDataColumnSidecarsFromColumnSidecar(t *testing.T) {
	// Create KZG commitments for 2 blobs
	commitment1 := make([]byte, 48)
	commitment2 := make([]byte, 48)
	commitment1[0] = 0x01
	commitment2[0] = 0x02
	kzgCommitments := [][]byte{commitment1, commitment2}

	// Create column data for 2 blobs
	columnData := make([][]byte, 2)
	columnData[0] = make([]byte, kzg.BytesPerCell)
	columnData[1] = make([]byte, kzg.BytesPerCell)
	columnData[0][0] = 0x11 // Distinct values
	columnData[1][0] = 0x22

	// Create KZG proofs for 2 blobs
	kzgProofs := make([][]byte, 2)
	kzgProofs[0] = make([]byte, 48)
	kzgProofs[1] = make([]byte, 48)
	kzgProofs[0][0] = 0x33
	kzgProofs[1][0] = 0x44

	// Create inclusion proof
	inclusionProof := make([][]byte, 4)
	for i := range inclusionProof {
		inclusionProof[i] = make([]byte, 32)
		inclusionProof[i][0] = byte(i + 0x50)
	}

	// Create the input VerifiedRODataColumn sidecar using test utility
	_, verifiedSidecars := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{
		{
			Index:                        5, // Column index 5
			Column:                       columnData,
			KzgCommitments:               kzgCommitments,
			KzgProofs:                    kzgProofs,
			KzgCommitmentsInclusionProof: inclusionProof,
			Slot:                         42,
			ProposerIndex:                7,
			ParentRoot:                   make([]byte, 32),
			StateRoot:                    make([]byte, 32),
			BodyRoot:                     make([]byte, 32),
		},
	})
	require.Equal(t, 1, len(verifiedSidecars))
	verifiedInputSidecar := verifiedSidecars[0]

	// Create cells and proofs with correct dimensions for 2 blobs
	numberOfColumns := params.BeaconConfig().NumberOfColumns
	cellsAndProofs := []kzg.CellsAndProofs{
		{
			Cells:  make([]kzg.Cell, numberOfColumns),
			Proofs: make([]kzg.Proof, numberOfColumns),
		},
		{
			Cells:  make([]kzg.Cell, numberOfColumns),
			Proofs: make([]kzg.Proof, numberOfColumns),
		},
	}

	// Set distinct values in cells and proofs for testing
	for i := range numberOfColumns {
		cellsAndProofs[0].Cells[i][0] = byte(i)
		cellsAndProofs[0].Proofs[i][0] = byte(i + 10)
		cellsAndProofs[1].Cells[i][0] = byte(i + 128)
		cellsAndProofs[1].Proofs[i][0] = byte(i + 138)
	}

	// Call the function
	sidecars, err := peerdas.ConstructDataColumnSidecar(cellsAndProofs, peerdas.PopulateFromSidecar(verifiedInputSidecar.RODataColumn))
	require.NoError(t, err)
	require.NotNil(t, sidecars)
	require.Equal(t, int(numberOfColumns), len(sidecars))

	// Verify each sidecar has the expected structure
	for i, sidecar := range sidecars {
		require.Equal(t, uint64(i), sidecar.Index)
		require.Equal(t, 2, len(sidecar.Column))
		require.Equal(t, 2, len(sidecar.KzgCommitments))
		require.Equal(t, 2, len(sidecar.KzgProofs))

		// Verify commitments match input
		require.DeepEqual(t, commitment1, sidecar.KzgCommitments[0])
		require.DeepEqual(t, commitment2, sidecar.KzgCommitments[1])

		// Verify column data comes from the correct cells
		require.Equal(t, byte(i), sidecar.Column[0][0])
		require.Equal(t, byte(i+128), sidecar.Column[1][0])

		// Verify proofs come from the correct proofs
		require.Equal(t, byte(i+10), sidecar.KzgProofs[0][0])
		require.Equal(t, byte(i+138), sidecar.KzgProofs[1][0])

		// Verify inclusion proof is preserved
		require.Equal(t, len(inclusionProof), len(sidecar.KzgCommitmentsInclusionProof))
		for j, proof := range sidecar.KzgCommitmentsInclusionProof {
			require.Equal(t, byte(j+0x50), proof[0])
		}

		// Verify signed block header is preserved
		require.Equal(t, primitives.Slot(42), sidecar.SignedBlockHeader.Header.Slot)
		require.Equal(t, primitives.ValidatorIndex(7), sidecar.SignedBlockHeader.Header.ProposerIndex)
	}
}
