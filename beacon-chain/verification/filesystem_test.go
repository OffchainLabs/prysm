package verification

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/spf13/afero"
)

func TestVerifiedROBlobFromDisk(t *testing.T) {
	// Create test data.
	_, blobs := util.GenerateTestDenebBlockWithSidecar(t, [fieldparams.RootLength]byte{}, 0, 1)
	originalBlob := blobs[0]

	// Marshal the blob sidecar to SSZ.
	sszData, err := originalBlob.MarshalSSZ()
	require.NoError(t, err)

	// Create in-memory filesystem.
	fs := afero.NewMemMapFs()

	// Write test data to file..
	filePath := "/test/blob.ssz"
	err = afero.WriteFile(fs, filePath, sszData, 0o644)
	require.NoError(t, err)

	// Test the function.
	blockRoot := originalBlob.BlockRoot()
	verifiedBlob, err := VerifiedROBlobFromDisk(fs, blockRoot, filePath)
	require.NoError(t, err)

	// Verify the result.
	require.Equal(t, originalBlob.Index, verifiedBlob.ROBlob.Index)
	require.Equal(t, originalBlob.Slot(), verifiedBlob.ROBlob.Slot())
	require.Equal(t, blockRoot, verifiedBlob.ROBlob.BlockRoot())
}

func TestVerifiedRODataColumnFromDisk(t *testing.T) {
	// Generate test data columns.
	columns := GenerateTestDataColumns(t, [fieldparams.RootLength]byte{}, 1, 1)
	originalColumn := columns[0]
	blockRoot := originalColumn.BlockRoot()

	// Marshal the data column sidecar to SSZ.
	sszData, err := originalColumn.DataColumnSidecar().MarshalSSZ()
	require.NoError(t, err)
	sszSize := uint32(len(sszData))

	t.Run("unexpected size", func(t *testing.T) {
		// Create in-memory filesystem with smaller data.
		fs := afero.NewMemMapFs()

		// Write partial data.
		filePath := "/test/partial.ssz"
		partialData := sszData[:len(sszData)/2]
		err := afero.WriteFile(fs, filePath, partialData, 0o644)
		require.NoError(t, err)

		// Open file for reading.
		file, err := fs.Open(filePath)
		require.NoError(t, err)

		// Test the function.
		_, err = VerifiedRODataColumnFromDisk(file, blockRoot, sszSize, 0)
		require.NotNil(t, err)

		err = file.Close()
		require.NoError(t, err)
	})

	t.Run("nominal", func(t *testing.T) {
		// Create in-memory filesystem.
		fs := afero.NewMemMapFs()

		// Write test data to file.
		filePath := "/test/datacolumn.ssz"
		err := afero.WriteFile(fs, filePath, sszData, 0o644)
		require.NoError(t, err)

		// Open file for reading.
		file, err := fs.Open(filePath)
		require.NoError(t, err)

		// Test the function.
		verifiedColumn, err := VerifiedRODataColumnFromDisk(file, blockRoot, sszSize, 0)
		require.NoError(t, err)

		// Verify the result.
		require.Equal(t, originalColumn.Index(), verifiedColumn.RODataColumn.Index())
		require.Equal(t, originalColumn.Slot(), verifiedColumn.RODataColumn.Slot())
		require.Equal(t, blockRoot, verifiedColumn.RODataColumn.BlockRoot())

		err = file.Close()
		require.NoError(t, err)
	})

	t.Run("gloas roundtrip", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		blockRoot := [fieldparams.RootLength]byte{0xaa, 0xbb}
		slot := primitives.Slot(5)
		idx := uint64(42)

		numCells := 3
		column := make([][]byte, numCells)
		kzgProofs := make([][]byte, numCells)
		for i := range numCells {
			cell := make([]byte, 2048)
			cell[0] = byte(i + 1)
			column[i] = cell
			kzgProofs[i] = make([]byte, 48)
		}

		original := &ethpb.DataColumnSidecarGloas{
			Index:           idx,
			Slot:            slot,
			BeaconBlockRoot: blockRoot[:],
			Column:          column,
			KzgProofs:       kzgProofs,
		}

		sszData, err := original.MarshalSSZ()
		require.NoError(t, err)
		sszSize := uint32(len(sszData))

		fs := afero.NewMemMapFs()
		filePath := "/test/gloas_column.ssz"
		err = afero.WriteFile(fs, filePath, sszData, 0o644)
		require.NoError(t, err)

		file, err := fs.Open(filePath)
		require.NoError(t, err)
		defer func() { require.NoError(t, file.Close()) }()

		gloasEpoch := primitives.Epoch(0)
		verifiedColumn, err := VerifiedRODataColumnFromDisk(file, blockRoot, sszSize, gloasEpoch)
		require.NoError(t, err)

		col := verifiedColumn.RODataColumn
		require.Equal(t, true, col.IsGloas())
		require.Equal(t, idx, col.Index())
		require.Equal(t, slot, col.Slot())
		require.Equal(t, blockRoot, col.BlockRoot())
		require.Equal(t, numCells, len(col.Column()))
		require.Equal(t, numCells, len(col.KzgProofs()))
		require.Equal(t, byte(1), col.Column()[0][0])
		require.Equal(t, byte(2), col.Column()[1][0])
		require.Equal(t, byte(3), col.Column()[2][0])
	})
}
