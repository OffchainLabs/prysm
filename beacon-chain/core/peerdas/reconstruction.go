package peerdas

import (
	"sort"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	pb "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

var (
	ErrColumnLengthsDiffer      = errors.New("columns do not have the same length")
	ErrBlobIndexTooHigh         = errors.New("blob index is too high")
	ErrBlockRootMismatch        = errors.New("block root mismatch")
	ErrBlobsCellsProofsMismatch = errors.New("blobs and cells proofs mismatch")
	ErrNilBlobAndProof          = errors.New("nil blob and proof")
)

// MinimumColumnCountToReconstruct return the minimum number of columns needed to proceed to a reconstruction.
func MinimumColumnCountToReconstruct() uint64 {
	// If the number of columns is odd, then we need total / 2 + 1 columns to reconstruct.
	// If the number of columns is even, then we need total / 2 columns to reconstruct.
	return (params.BeaconConfig().NumberOfColumns + 1) / 2
}

// ReconstructDataColumnSidecars reconstructs all the data column sidecars from the given input data column sidecars.
// All input sidecars must be committed to the same block.
// `inVerifiedRoSidecars` should contain enough sidecars to reconstruct the missing columns, and should not contain any duplicate.
// WARNING: This function sorts inplace `verifiedRoSidecars` by index.
func ReconstructDataColumnSidecars(verifiedRoSidecars []blocks.VerifiedRODataColumn) ([]blocks.VerifiedRODataColumn, error) {
	// Check if there is at least one input sidecar.
	if len(verifiedRoSidecars) == 0 {
		return nil, ErrNotEnoughDataColumnSidecars
	}

	// Safely retrieve the first sidecar as a reference.
	referenceSidecar := verifiedRoSidecars[0]

	// Check if all columns have the same length and are commmitted to the same block.
	blobCount := len(referenceSidecar.Column)
	blockRoot := referenceSidecar.BlockRoot()
	for _, sidecar := range verifiedRoSidecars[1:] {
		if len(sidecar.Column) != blobCount {
			return nil, ErrColumnLengthsDiffer
		}

		if sidecar.BlockRoot() != blockRoot {
			return nil, ErrBlockRootMismatch
		}
	}

	// Check if there is enough sidecars to reconstruct the missing columns.
	sidecarCount := len(verifiedRoSidecars)
	if uint64(sidecarCount) < MinimumColumnCountToReconstruct() {
		return nil, ErrNotEnoughDataColumnSidecars
	}

	// Sort the input sidecars by index.
	sort.Slice(verifiedRoSidecars, func(i, j int) bool {
		return verifiedRoSidecars[i].Index < verifiedRoSidecars[j].Index
	})

	// Recover cells and compute proofs in parallel.
	var wg errgroup.Group
	cellsAndProofs := make([]kzg.CellsAndProofs, blobCount)
	for blobIndex := range uint64(blobCount) {
		wg.Go(func() error {
			cellsIndices := make([]uint64, 0, sidecarCount)
			cells := make([]kzg.Cell, 0, sidecarCount)

			for _, sidecar := range verifiedRoSidecars {
				cell := sidecar.Column[blobIndex]
				cells = append(cells, kzg.Cell(cell))
				cellsIndices = append(cellsIndices, sidecar.Index)
			}

			// Recover the cells and proofs for the corresponding blob
			cellsAndProofsForBlob, err := kzg.RecoverCellsAndKZGProofs(cellsIndices, cells)

			if err != nil {
				return errors.Wrapf(err, "recover cells and KZG proofs for blob %d", blobIndex)
			}

			// It is safe for multiple goroutines to concurrently write to the same slice,
			// as long as they are writing to different indices, which is the case here.
			cellsAndProofs[blobIndex] = cellsAndProofsForBlob
			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		return nil, errors.Wrap(err, "wait for RecoverCellsAndKZGProofs")
	}

	outSidecars, err := DataColumnSidecars(cellsAndProofs, PopulateFromSidecar(referenceSidecar))
	if err != nil {
		return nil, errors.Wrap(err, "data column sidecars from items")
	}

	// Input sidecars are verified, and we reconstructed ourselves the missing sidecars.
	// As a consequence, reconstructed sidecars are also verified.
	reconstructedVerifiedRoSidecars := make([]blocks.VerifiedRODataColumn, 0, len(outSidecars))
	for _, sidecar := range outSidecars {
		verifiedRoSidecar := blocks.NewVerifiedRODataColumn(sidecar)
		reconstructedVerifiedRoSidecars = append(reconstructedVerifiedRoSidecars, verifiedRoSidecar)
	}

	return reconstructedVerifiedRoSidecars, nil
}

// ReconstructBlobs constructs verified read only blobs sidecars from verified read only blob sidecars.
// The following constraints must be satisfied:
//   - All `dataColumnSidecars` has to be committed to the same block, and
//   - `dataColumnSidecars` must be sorted by index and should not contain duplicates.
//   - `dataColumnSidecars` must contain either all sidecars corresponding to (non-extended) blobs,
//     or either enough sidecars to reconstruct the blobs.
func ReconstructBlobs(block blocks.ROBlock, verifiedDataColumnSidecars []blocks.VerifiedRODataColumn, indices []int) ([]*blocks.VerifiedROBlob, error) {
	// Return early if no blobs are requested.
	if len(indices) == 0 {
		return nil, nil
	}

	if len(verifiedDataColumnSidecars) == 0 {
		return nil, ErrNotEnoughDataColumnSidecars
	}

	// Check if the sidecars are sorted by index and do not contain duplicates.
	previousColumnIndex := verifiedDataColumnSidecars[0].Index
	for _, dataColumnSidecar := range verifiedDataColumnSidecars[1:] {
		columnIndex := dataColumnSidecar.Index
		if columnIndex <= previousColumnIndex {
			return nil, ErrDataColumnSidecarsNotSortedByIndex
		}

		previousColumnIndex = columnIndex
	}

	// Check if we have enough columns.
	cellsPerBlob := fieldparams.CellsPerBlob
	if len(verifiedDataColumnSidecars) < cellsPerBlob {
		return nil, ErrNotEnoughDataColumnSidecars
	}

	// Check if the blob index is too high.
	commitments, err := block.Block().Body().BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrap(err, "blob KZG commitments")
	}

	for _, blobIndex := range indices {
		if blobIndex >= len(commitments) {
			return nil, ErrBlobIndexTooHigh
		}
	}

	// Check if the data column sidecars are aligned with the block.
	dataColumnSidecars := make([]blocks.RODataColumn, 0, len(verifiedDataColumnSidecars))
	for _, verifiedDataColumnSidecar := range verifiedDataColumnSidecars {
		dataColumnSidecar := verifiedDataColumnSidecar.RODataColumn
		dataColumnSidecars = append(dataColumnSidecars, dataColumnSidecar)
	}

	if err := DataColumnsAlignWithBlock(block, dataColumnSidecars); err != nil {
		return nil, errors.Wrap(err, "data columns align with block")
	}

	// If all column sidecars corresponding to (non-extended) blobs are present, no need to reconstruct.
	if verifiedDataColumnSidecars[cellsPerBlob-1].Index == uint64(cellsPerBlob-1) {
		// Convert verified data column sidecars to verified blob sidecars.
		blobSidecars, err := blobSidecarsFromDataColumnSidecars(block, verifiedDataColumnSidecars, indices)
		if err != nil {
			return nil, errors.Wrap(err, "blob sidecars from data column sidecars")
		}

		return blobSidecars, nil
	}

	// We need to reconstruct the data column sidecars.
	reconstructedDataColumnSidecars, err := ReconstructDataColumnSidecars(verifiedDataColumnSidecars)
	if err != nil {
		return nil, errors.Wrap(err, "reconstruct data column sidecars")
	}

	// Convert verified data column sidecars to verified blob sidecars.
	blobSidecars, err := blobSidecarsFromDataColumnSidecars(block, reconstructedDataColumnSidecars, indices)
	if err != nil {
		return nil, errors.Wrap(err, "blob sidecars from data column sidecars")
	}

	return blobSidecars, nil
}

// ComputeCellsAndProofsFromFlat computes the cells and proofs from blobs and cell flat proofs.
func ComputeCellsAndProofsFromFlat(blobs [][]byte, cellProofs [][]byte) ([]kzg.CellsAndProofs, error) {
	numberOfColumns := params.BeaconConfig().NumberOfColumns
	blobCount := uint64(len(blobs))
	cellProofsCount := uint64(len(cellProofs))

	cellsCount := blobCount * numberOfColumns
	if cellsCount != cellProofsCount {
		return nil, ErrBlobsCellsProofsMismatch
	}

	cellsAndProofs := make([]kzg.CellsAndProofs, 0, blobCount)
	for i, blob := range blobs {
		var kzgBlob kzg.Blob
		if copy(kzgBlob[:], blob) != len(kzgBlob) {
			return nil, errors.New("wrong blob size - should never happen")
		}

		// Compute the extended cells from the (non-extended) blob.
		cells, err := kzg.ComputeCells(&kzgBlob)
		if err != nil {
			return nil, errors.Wrap(err, "compute cells")
		}

		var proofs []kzg.Proof
		for idx := uint64(i) * numberOfColumns; idx < (uint64(i)+1)*numberOfColumns; idx++ {
			var kzgProof kzg.Proof
			if copy(kzgProof[:], cellProofs[idx]) != len(kzgProof) {
				return nil, errors.New("wrong KZG proof size - should never happen")
			}

			proofs = append(proofs, kzgProof)
		}

		cellsProofs := kzg.CellsAndProofs{Cells: cells, Proofs: proofs}
		cellsAndProofs = append(cellsAndProofs, cellsProofs)
	}

	return cellsAndProofs, nil
}

// ComputeCellsAndProofs computes the cells and proofs from blobs and cell proofs.
func ComputeCellsAndProofsFromStructured(blobsAndProofs []*pb.BlobAndProofV2) ([]kzg.CellsAndProofs, error) {
	numberOfColumns := params.BeaconConfig().NumberOfColumns

	cellsAndProofs := make([]kzg.CellsAndProofs, 0, len(blobsAndProofs))
	for _, blobAndProof := range blobsAndProofs {
		if blobAndProof == nil {
			return nil, ErrNilBlobAndProof
		}

		var kzgBlob kzg.Blob
		if copy(kzgBlob[:], blobAndProof.Blob) != len(kzgBlob) {
			return nil, errors.New("wrong blob size - should never happen")
		}

		// Compute the extended cells from the (non-extended) blob.
		cells, err := kzg.ComputeCells(&kzgBlob)
		if err != nil {
			return nil, errors.Wrap(err, "compute cells")
		}

		kzgProofs := make([]kzg.Proof, 0, numberOfColumns)
		for _, kzgProofBytes := range blobAndProof.KzgProofs {
			if len(kzgProofBytes) != kzg.BytesPerProof {
				return nil, errors.New("wrong KZG proof size - should never happen")
			}

			var kzgProof kzg.Proof
			if copy(kzgProof[:], kzgProofBytes) != len(kzgProof) {
				return nil, errors.New("wrong copied KZG proof size - should never happen")
			}

			kzgProofs = append(kzgProofs, kzgProof)
		}

		cellsProofs := kzg.CellsAndProofs{Cells: cells, Proofs: kzgProofs}
		cellsAndProofs = append(cellsAndProofs, cellsProofs)
	}

	return cellsAndProofs, nil
}

// blobSidecarsFromDataColumnSidecars converts verified data column sidecars to verified blob sidecars.
func blobSidecarsFromDataColumnSidecars(roBlock blocks.ROBlock, dataColumnSidecars []blocks.VerifiedRODataColumn, indices []int) ([]*blocks.VerifiedROBlob, error) {
	referenceSidecar := dataColumnSidecars[0]

	kzgCommitments := referenceSidecar.KzgCommitments
	signedBlockHeader := referenceSidecar.SignedBlockHeader

	verifiedROBlobs := make([]*blocks.VerifiedROBlob, 0, len(indices))
	for _, blobIndex := range indices {
		var blob kzg.Blob

		// Compute the content of the blob.
		for columnIndex := range fieldparams.CellsPerBlob {
			dataColumnSidecar := dataColumnSidecars[columnIndex]
			cell := dataColumnSidecar.Column[blobIndex]
			if copy(blob[kzg.BytesPerCell*columnIndex:], cell) != kzg.BytesPerCell {
				return nil, errors.New("wrong cell size - should never happen")
			}
		}

		// Extract the KZG commitment.
		var kzgCommitment kzg.Commitment
		if copy(kzgCommitment[:], kzgCommitments[blobIndex]) != len(kzgCommitment) {
			return nil, errors.New("wrong KZG commitment size - should never happen")
		}

		// Compute the blob KZG proof.
		blobKzgProof, err := kzg.ComputeBlobKZGProof(&blob, kzgCommitment)
		if err != nil {
			return nil, errors.Wrap(err, "compute blob KZG proof")
		}

		// Build the inclusion proof for the blob.
		var kzgBlob kzg.Blob
		if copy(kzgBlob[:], blob[:]) != len(kzgBlob) {
			return nil, errors.New("wrong blob size - should never happen")
		}

		commitmentInclusionProof, err := blocks.MerkleProofKZGCommitment(roBlock.Block().Body(), blobIndex)
		if err != nil {
			return nil, errors.Wrap(err, "merkle proof KZG commitment")
		}

		// Build the blob sidecar.
		blobSidecar := &ethpb.BlobSidecar{
			Index:                    uint64(blobIndex),
			Blob:                     blob[:],
			KzgCommitment:            kzgCommitment[:],
			KzgProof:                 blobKzgProof[:],
			SignedBlockHeader:        signedBlockHeader,
			CommitmentInclusionProof: commitmentInclusionProof,
		}

		roBlob, err := blocks.NewROBlob(blobSidecar)
		if err != nil {
			return nil, errors.Wrap(err, "new RO blob")
		}

		verifiedROBlob := blocks.NewVerifiedROBlob(roBlob)
		verifiedROBlobs = append(verifiedROBlobs, &verifiedROBlob)
	}

	return verifiedROBlobs, nil
}
