package enginev1

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// BlobCellsAndProofsV1 represents the response for engine_getBlobsV4.
// It contains partial cells and their KZG proofs for a single blob.
type BlobCellsAndProofsV1 struct {
	BlobCells []*hexutil.Bytes `json:"blob_cells"`
	Proofs    []*hexutil.Bytes `json:"proofs"`
}
