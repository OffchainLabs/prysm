package verification

import (
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"

	"github.com/spf13/afero"
)

// VerifiedROBlobFromDisk creates a verified read-only blob sidecar from an error.
func VerifiedROBlobFromDisk(fs afero.Fs, root [32]byte, path string) (blocks.VerifiedROBlob, error) {
	encoded, err := afero.ReadFile(fs, path)
	if err != nil {
		return VerifiedROBlobError(err)
	}
	s := &ethpb.BlobSidecar{}
	if err := s.UnmarshalSSZ(encoded); err != nil {
		return VerifiedROBlobError(err)
	}
	ro, err := blocks.NewROBlobWithRoot(s, root)
	if err != nil {
		return VerifiedROBlobError(err)
	}
	return blocks.NewVerifiedROBlob(ro), nil
}

// VerifiedRODataColumnFromDisk creates a verified read-only data column sidecar from disk.
// The file cursor must be positioned at the start of the data column sidecar SSZ data.
func VerifiedRODataColumnFromDisk(file afero.File, root [fieldparams.RootLength]byte, sszEncodedDataColumnSidecarSize uint32) (blocks.VerifiedRODataColumn, error) {
	// Read the ssz encoded data column sidecar from the file
	sszEncodedDataColumnSidecar := make([]byte, sszEncodedDataColumnSidecarSize)
	count, err := file.Read(sszEncodedDataColumnSidecar)
	if err != nil {
		return VerifiedRODataColumnError(err)
	}
	if uint32(count) != sszEncodedDataColumnSidecarSize {
		return VerifiedRODataColumnError(errors.Errorf("read %d bytes while expecting %d", count, sszEncodedDataColumnSidecarSize))
	}

	// Unmarshal the SSZ encoded data column sidecar.
	dataColumnSidecar := &ethpb.DataColumnSidecar{}
	if err := dataColumnSidecar.UnmarshalSSZ(sszEncodedDataColumnSidecar); err != nil {
		return VerifiedRODataColumnError(err)
	}

	// Create a RO data column.
	roDataColumnSidecar, err := blocks.NewRODataColumnWithRoot(dataColumnSidecar, root)
	if err != nil {
		return VerifiedRODataColumnError(err)
	}

	// Create a verified RO data column.
	verifiedRODataColumn := blocks.NewVerifiedRODataColumn(roDataColumnSidecar)

	return verifiedRODataColumn, nil
}
