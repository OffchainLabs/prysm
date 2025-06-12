package blocks

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewSidecarFromBlobSidecar(t *testing.T) {
	blob := ROBlob{}
	sidecar := NewSidecarFromBlobSidecar(blob)

	// Check that the blob is set
	retrievedBlob, err := sidecar.Blob()
	require.NoError(t, err)
	require.Equal(t, blob, retrievedBlob)

	// Check that data column is not set
	_, err = sidecar.DataColumn()
	require.ErrorIs(t, err, errDataColumnNeeded)
}

func TestNewSidecarFromDataColumnSidecar(t *testing.T) {
	dataColumn := RODataColumn{}
	sidecar := NewSidecarFromDataColumnSidecar(dataColumn)

	// Check that the data column is set
	retrievedDataColumn, err := sidecar.DataColumn()
	require.NoError(t, err)
	require.Equal(t, dataColumn, retrievedDataColumn)

	// Check that blob is not set
	_, err = sidecar.Blob()
	require.ErrorIs(t, err, errBlobNeeded)
}

func TestNewSidecarsFromBlobSidecars(t *testing.T) {
	blobSidecars := []ROBlob{{}, {}}
	sidecars := NewSidecarsFromBlobSidecars(blobSidecars)

	require.Equal(t, len(blobSidecars), len(sidecars))

	for i, sidecar := range sidecars {
		retrievedBlob, err := sidecar.Blob()
		require.NoError(t, err)
		require.Equal(t, blobSidecars[i], retrievedBlob)
	}
}

func TestNewSidecarsFromDataColumnSidecars(t *testing.T) {
	dataColumnSidecars := []RODataColumn{{}, {}}
	sidecars := NewSidecarsFromDataColumnSidecars(dataColumnSidecars)

	require.Equal(t, len(dataColumnSidecars), len(sidecars))

	for i, sidecar := range sidecars {
		retrievedDataColumn, err := sidecar.DataColumn()
		require.NoError(t, err)
		require.Equal(t, dataColumnSidecars[i], retrievedDataColumn)
	}
}

func TestBlobSidecarsFromSidecars(t *testing.T) {
	// Create sidecars with blobs
	blobSidecars := []ROBlob{{}, {}}
	sidecars := NewSidecarsFromBlobSidecars(blobSidecars)

	// Convert back to blob sidecars
	retrievedBlobSidecars, err := BlobSidecarsFromSidecars(sidecars)
	require.NoError(t, err)
	require.Equal(t, len(blobSidecars), len(retrievedBlobSidecars))

	for i, blob := range retrievedBlobSidecars {
		require.Equal(t, blobSidecars[i], blob)
	}

	// Test with a mix of sidecar types
	mixedSidecars := []ROSidecar{
		NewSidecarFromBlobSidecar(ROBlob{}),
		NewSidecarFromDataColumnSidecar(RODataColumn{}),
	}

	_, err = BlobSidecarsFromSidecars(mixedSidecars)
	require.Error(t, err)
}

func TestDataColumnSidecarsFromSidecars(t *testing.T) {
	// Create sidecars with data columns
	dataColumnSidecars := []RODataColumn{{}, {}}
	sidecars := NewSidecarsFromDataColumnSidecars(dataColumnSidecars)

	// Convert back to data column sidecars
	retrievedDataColumnSidecars, err := DataColumnSidecarsFromSidecars(sidecars)
	require.NoError(t, err)
	require.Equal(t, len(dataColumnSidecars), len(retrievedDataColumnSidecars))

	for i, dataColumn := range retrievedDataColumnSidecars {
		require.Equal(t, dataColumnSidecars[i], dataColumn)
	}

	// Test with a mix of sidecar types
	mixedSidecars := []ROSidecar{
		NewSidecarFromDataColumnSidecar(RODataColumn{}),
		NewSidecarFromBlobSidecar(ROBlob{}),
	}

	_, err = DataColumnSidecarsFromSidecars(mixedSidecars)
	require.Error(t, err)
}
