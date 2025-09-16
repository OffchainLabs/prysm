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

func TestDataColumnSidecars(t *testing.T) {
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
		_, err = peerdas.DataColumnSidecars(cellsAndProofs, peerdas.PopulateFromBlock(rob))
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
		_, err = peerdas.DataColumnSidecars(cellsAndProofs, peerdas.PopulateFromBlock(rob))
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
		_, err = peerdas.DataColumnSidecars(cellsAndProofs, peerdas.PopulateFromBlock(rob))
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
		sidecars, err := peerdas.DataColumnSidecars(cellsAndProofs, peerdas.PopulateFromBlock(rob))
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

func TestReconstructionSource(t *testing.T) {
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
	sidecars, err := peerdas.DataColumnSidecars(cellsAndProofs, peerdas.PopulateFromBlock(rob))
	require.NoError(t, err)
	require.NotNil(t, sidecars)
	require.Equal(t, int(numberOfColumns), len(sidecars))

	t.Run("from block", func(t *testing.T) {
		src := peerdas.PopulateFromBlock(rob)
		require.Equal(t, rob.Block().Slot(), src.Slot())
		require.Equal(t, rob.Root(), src.Root())
		require.Equal(t, rob.Block().ProposerIndex(), src.ProposerIndex())

		commitments, err := src.Commitments()
		require.NoError(t, err)
		require.Equal(t, 2, len(commitments))
		require.DeepEqual(t, commitment1, commitments[0])
		require.DeepEqual(t, commitment2, commitments[1])

		require.Equal(t, peerdas.BlockType, src.Type())
	})

	t.Run("from sidecar", func(t *testing.T) {
		referenceSidecar := blocks.NewVerifiedRODataColumn(sidecars[0])
		src := peerdas.PopulateFromSidecar(referenceSidecar)
		require.Equal(t, referenceSidecar.Slot(), src.Slot())
		require.Equal(t, referenceSidecar.BlockRoot(), src.Root())
		require.Equal(t, referenceSidecar.ProposerIndex(), src.ProposerIndex())

		commitments, err := src.Commitments()
		require.NoError(t, err)
		require.Equal(t, 2, len(commitments))
		require.DeepEqual(t, commitment1, commitments[0])
		require.DeepEqual(t, commitment2, commitments[1])

		require.Equal(t, peerdas.SidecarType, src.Type())
	})
}
