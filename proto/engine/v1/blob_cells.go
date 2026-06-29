package enginev1

import (
	"encoding/json"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

// BlobCellsAndProofsV1 represents the response for engine_getBlobsV4.
// It contains partial cells and their KZG proofs for a single blob.
type BlobCellsAndProofsV1 struct {
	BlobCells []*[]byte `json:"blob_cells"`
	Proofs    []*[]byte `json:"proofs"`
}

// BlobCellsAndProofsV1Json is the JSON representation for BlobCellsAndProofsV1.
type BlobCellsAndProofsV1Json struct {
	BlobCells []*hexutil.Bytes `json:"blob_cells"`
	Proofs    []*hexutil.Bytes `json:"proofs"`
}

func (b *BlobCellsAndProofsV1) UnmarshalJSON(enc []byte) error {
	var dec *BlobCellsAndProofsV1Json
	if err := json.Unmarshal(enc, &dec); err != nil {
		return err
	}
	if dec == nil {
		return nil
	}

	b.BlobCells = make([]*[]byte, len(dec.BlobCells))
	for i, cell := range dec.BlobCells {
		if cell != nil {
			c := []byte(*cell)
			b.BlobCells[i] = &c
		}
	}

	b.Proofs = make([]*[]byte, len(dec.Proofs))
	for i, proof := range dec.Proofs {
		if proof != nil {
			p := []byte(*proof)
			b.Proofs[i] = &p
		}
	}

	return nil
}
