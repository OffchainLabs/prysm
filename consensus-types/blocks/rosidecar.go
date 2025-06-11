package blocks

import (
	"github.com/pkg/errors"
)

// ROSidecar represents a read-only sidecar with its block root.
type ROSidecar struct {
	blob       *ROBlob
	dataColumn *RODataColumn
}

var (
	errBlobNeeded       = errors.New("blob sidecar needed")
	errDataColumnNeeded = errors.New("data column sidecar needed")
)

// NewSidecarFromBlobSidecar creates a new read-only (generic) sidecar from a read-only blob sidecar.
func NewSidecarFromBlobSidecar(blob ROBlob) ROSidecar {
	return ROSidecar{blob: &blob}
}

// NewSidecarFromDataColumnSidecar creates a new read-only (generic) sidecar from a read-only data column sidecar.
func NewSidecarFromDataColumnSidecar(dataColumn RODataColumn) ROSidecar {
	return ROSidecar{dataColumn: &dataColumn}
}

// NewSidecarsFromBlobSidecars creates a new slice of read-only (generic) sidecars from a slice of read-only blobs sidecars.
func NewSidecarsFromBlobSidecars(blobSidecars []ROBlob) []ROSidecar {
	sidecars := make([]ROSidecar, 0, len(blobSidecars))
	for _, blob := range blobSidecars {
		blobSidecar := ROSidecar{blob: &blob} // #nosec G601
		sidecars = append(sidecars, blobSidecar)
	}

	return sidecars
}

// NewSidecarsFromDataColumnSidecars creates a new slice of read-only (generic) sidecars from a slice of read-only data column sidecars.
func NewSidecarsFromDataColumnSidecars(dataColumnSidecars []RODataColumn) []ROSidecar {
	sidecars := make([]ROSidecar, 0, len(dataColumnSidecars))
	for _, dataColumn := range dataColumnSidecars {
		dataColumnSidecar := ROSidecar{dataColumn: &dataColumn} // #nosec G601
		sidecars = append(sidecars, dataColumnSidecar)
	}

	return sidecars
}

// Blob returns the blob sidecar.
func (sc *ROSidecar) Blob() (ROBlob, error) {
	if sc.blob == nil {
		return ROBlob{}, errBlobNeeded
	}

	return *sc.blob, nil
}

// DataColumn returns the data column sidecar.
func (sc *ROSidecar) DataColumn() (RODataColumn, error) {
	if sc.dataColumn == nil {
		return RODataColumn{}, errDataColumnNeeded
	}

	return *sc.dataColumn, nil
}

// BlobSidecarsFromSidecars creates a new slice of read-only blobs sidecars from a slice of read-only (generic) sidecars.
func BlobSidecarsFromSidecars(sidecars []ROSidecar) ([]ROBlob, error) {
	blobSidecars := make([]ROBlob, 0, len(sidecars))
	for _, sidecar := range sidecars {
		blob, err := sidecar.Blob()
		if err != nil {
			return nil, errors.Wrap(err, "blob")
		}

		blobSidecars = append(blobSidecars, blob)
	}

	return blobSidecars, nil
}

// DataColumnSidecarsFromSidecars creates a new slice of read-only data column sidecars from a slice of read-only (generic) sidecars.
func DataColumnSidecarsFromSidecars(sidecars []ROSidecar) ([]RODataColumn, error) {
	dataColumnSidecars := make([]RODataColumn, 0, len(sidecars))
	for _, sidecar := range sidecars {
		dataColumn, err := sidecar.DataColumn()
		if err != nil {
			return nil, errors.Wrap(err, "data column")
		}

		dataColumnSidecars = append(dataColumnSidecars, dataColumn)
	}

	return dataColumnSidecars, nil
}
