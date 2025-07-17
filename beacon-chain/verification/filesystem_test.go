package verification

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
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
	err = afero.WriteFile(fs, filePath, sszData, 0644)
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
	sszData, err := originalColumn.MarshalSSZ()
	require.NoError(t, err)
	sszSize := uint32(len(sszData))

	t.Run("unexpected size", func(t *testing.T) {
		// Create in-memory filesystem with smaller data.
		fs := afero.NewMemMapFs()

		// Write partial data.
		filePath := "/test/partial.ssz"
		partialData := sszData[:len(sszData)/2]
		err := afero.WriteFile(fs, filePath, partialData, 0644)
		require.NoError(t, err)

		// Open file for reading.
		file, err := fs.Open(filePath)
		require.NoError(t, err)

		// Test the function.
		_, err = VerifiedRODataColumnFromDisk(file, blockRoot, sszSize)
		require.NotNil(t, err)

		err = file.Close()
		require.NoError(t, err)
	})

	t.Run("nominal", func(t *testing.T) {
		// Create in-memory filesystem.
		fs := afero.NewMemMapFs()

		// Write test data to file.
		filePath := "/test/datacolumn.ssz"
		err := afero.WriteFile(fs, filePath, sszData, 0644)
		require.NoError(t, err)

		// Open file for reading.
		file, err := fs.Open(filePath)
		require.NoError(t, err)

		// Test the function.
		verifiedColumn, err := VerifiedRODataColumnFromDisk(file, blockRoot, sszSize)
		require.NoError(t, err)

		// Verify the result.
		require.Equal(t, originalColumn.Index, verifiedColumn.RODataColumn.Index)
		require.Equal(t, originalColumn.Slot(), verifiedColumn.RODataColumn.Slot())
		require.Equal(t, blockRoot, verifiedColumn.RODataColumn.BlockRoot())

		err = file.Close()
		require.NoError(t, err)
	})
}
