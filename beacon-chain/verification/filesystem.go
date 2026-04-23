package verification

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
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
func VerifiedRODataColumnFromDisk(file afero.File, root [fieldparams.RootLength]byte, sszEncodedDataColumnSidecarSize uint32, epoch primitives.Epoch) (blocks.VerifiedRODataColumn, error) {
	sszEncodedDataColumnSidecar := make([]byte, sszEncodedDataColumnSidecarSize)
	count, err := file.Read(sszEncodedDataColumnSidecar)
	if err != nil {
		return VerifiedRODataColumnError(err)
	}
	if uint32(count) != sszEncodedDataColumnSidecarSize {
		return VerifiedRODataColumnError(errors.Errorf("read %d bytes while expecting %d", count, sszEncodedDataColumnSidecarSize))
	}

	var roDataColumnSidecar blocks.RODataColumn
	if epoch >= params.BeaconConfig().GloasForkEpoch {
		dc := &ethpb.DataColumnSidecarGloas{}
		if err := dc.UnmarshalSSZ(sszEncodedDataColumnSidecar); err != nil {
			return VerifiedRODataColumnError(err)
		}
		roDataColumnSidecar, err = blocks.NewRODataColumnGloasWithRoot(dc, root)
	} else {
		dc := &ethpb.DataColumnSidecar{}
		if err := dc.UnmarshalSSZ(sszEncodedDataColumnSidecar); err != nil {
			return VerifiedRODataColumnError(err)
		}
		roDataColumnSidecar, err = blocks.NewRODataColumnWithRoot(dc, root)
	}
	if err != nil {
		return VerifiedRODataColumnError(err)
	}

	return blocks.NewVerifiedRODataColumn(roDataColumnSidecar), nil
}
