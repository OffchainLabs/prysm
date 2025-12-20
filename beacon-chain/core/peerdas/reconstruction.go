package peerdas

import (
	"sort"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
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
	return (fieldparams.NumberOfColumns + 1) / 2
}

// MinimumCustodyGroupCountToReconstruct returns the minimum number of custody groups needed to
// custody enough data columns for reconstruction. This accounts for the relationship between
// custody groups and columns, making it future-proof if these values change.
// Returns an error if the configuration values are invalid (zero or would cause division by zero).
func MinimumCustodyGroupCountToReconstruct() (uint64, error) {
	const numberOfColumns = fieldparams.NumberOfColumns
	cfg := params.BeaconConfig()

	// Validate configuration values
	if numberOfColumns == 0 {
		return 0, errors.New("NumberOfColumns cannot be zero")
	}
	if cfg.NumberOfCustodyGroups == 0 {
		return 0, errors.New("NumberOfCustodyGroups cannot be zero")
	}

	minimumColumnCount := MinimumColumnCountToReconstruct()

	// Calculate how many columns each custody group represents
	columnsPerGroup := numberOfColumns / cfg.NumberOfCustodyGroups

	// If there are more groups than columns (columnsPerGroup = 0), this is an invalid configuration
	// for reconstruction purposes as we cannot determine a meaningful custody group count
	if columnsPerGroup == 0 {
		return 0, errors.Errorf("invalid configuration: NumberOfCustodyGroups (%d) exceeds NumberOfColumns (%d)",
			cfg.NumberOfCustodyGroups, numberOfColumns)
	}

	// Use ceiling division to ensure we have enough groups to cover the minimum columns
	// ceiling(a/b) = (a + b - 1) / b
	return (minimumColumnCount + columnsPerGroup - 1) / columnsPerGroup, nil
}

// recoverCellsForBlobs reconstructs cells for specified blobs from the given data column sidecars.
// This is optimized to only recover cells without computing proofs.
// Returns a map from blob index to recovered cells.
func recoverCellsForBlobs(verifiedRoSidecars []blocks.VerifiedRODataColumn, blobIndices []int) (map[int][]kzg.Cell, error) {
	sidecarCount := len(verifiedRoSidecars)
	var wg errgroup.Group

	cellsPerBlob := make(map[int][]kzg.Cell, len(blobIndices))
	var mu sync.Mutex

	for _, blobIndex := range blobIndices {
		wg.Go(func() error {
			cellsIndices := make([]uint64, 0, sidecarCount)
			cells := make([]kzg.Cell, 0, sidecarCount)

			for _, sidecar := range verifiedRoSidecars {
				cell := sidecar.Column[blobIndex]
				cells = append(cells, kzg.Cell(cell))
				cellsIndices = append(cellsIndices, sidecar.Index)
			}

			recoveredCells, err := kzg.RecoverCells(cellsIndices, cells)
			if err != nil {
				return errors.Wrapf(err, "recover cells for blob %d", blobIndex)
			}

			mu.Lock()
			cellsPerBlob[blobIndex] = recoveredCells
			mu.Unlock()
			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		return nil, errors.Wrap(err, "wait for RecoverCells")
	}
	return cellsPerBlob, nil
}

// recoverCellsAndProofsForBlobs reconstructs both cells and proofs for specified blobs from the given data column sidecars.
func recoverCellsAndProofsForBlobs(verifiedRoSidecars []blocks.VerifiedRODataColumn, blobIndices []int) ([][]kzg.Cell, [][]kzg.Proof, error) {
	sidecarCount := len(verifiedRoSidecars)
	var wg errgroup.Group

	cellsPerBlob := make([][]kzg.Cell, len(blobIndices))
	proofsPerBlob := make([][]kzg.Proof, len(blobIndices))

	for i, blobIndex := range blobIndices {
		wg.Go(func() error {
			cellsIndices := make([]uint64, 0, sidecarCount)
			cells := make([]kzg.Cell, 0, sidecarCount)

			for _, sidecar := range verifiedRoSidecars {
				cell := sidecar.Column[blobIndex]
				cells = append(cells, kzg.Cell(cell))
				cellsIndices = append(cellsIndices, sidecar.Index)
			}

			recoveredCells, recoveredProofs, err := kzg.RecoverCellsAndKZGProofs(cellsIndices, cells)
			if err != nil {
				return errors.Wrapf(err, "recover cells and KZG proofs for blob %d", blobIndex)
			}
			cellsPerBlob[i] = recoveredCells
			proofsPerBlob[i] = recoveredProofs
			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		return nil, nil, errors.Wrap(err, "wait for RecoverCellsAndKZGProofs")
	}
	return cellsPerBlob, proofsPerBlob, nil
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
	blobIndices := make([]int, blobCount)
	for i := range blobIndices {
		blobIndices[i] = i
	}
	cellsPerBlob, proofsPerBlob, err := recoverCellsAndProofsForBlobs(verifiedRoSidecars, blobIndices)
	if err != nil {
		return nil, errors.Wrap(err, "recover cells and proofs for blobs")
	}

	outSidecars, err := DataColumnSidecars(cellsPerBlob, proofsPerBlob, PopulateFromSidecar(referenceSidecar))
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

// reconstructIfNeeded validates the input data column sidecars and returns the prepared sidecars
// (reconstructed if necessary). This function performs common validation and reconstruction logic used by
// both ReconstructBlobs and ReconstructBlobSidecars.
func reconstructIfNeeded(verifiedDataColumnSidecars []blocks.VerifiedRODataColumn) ([]blocks.VerifiedRODataColumn, error) {
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

	// If all column sidecars corresponding to (non-extended) blobs are present, no need to reconstruct.
	if verifiedDataColumnSidecars[cellsPerBlob-1].Index == uint64(cellsPerBlob-1) {
		return verifiedDataColumnSidecars, nil
	}

	// We need to reconstruct the data column sidecars.
	return ReconstructDataColumnSidecars(verifiedDataColumnSidecars)
}

// ReconstructBlobSidecars constructs verified read only blobs sidecars from verified read only blob sidecars.
// The following constraints must be satisfied:
//   - All `dataColumnSidecars` has to be committed to the same block, and
//   - `dataColumnSidecars` must be sorted by index and should not contain duplicates.
//   - `dataColumnSidecars` must contain either all sidecars corresponding to (non-extended) blobs,
//   - either enough sidecars to reconstruct the blobs.
func ReconstructBlobSidecars(block blocks.ROBlock, verifiedDataColumnSidecars []blocks.VerifiedRODataColumn, indices []int) ([]*blocks.VerifiedROBlob, error) {
	// Return early if no blobs are requested.
	if len(indices) == 0 {
		return nil, nil
	}

	// Validate and prepare data columns (reconstruct if necessary).
	// This also checks if input is empty.
	preparedDataColumnSidecars, err := reconstructIfNeeded(verifiedDataColumnSidecars)
	if err != nil {
		return nil, err
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
	dataColumnSidecars := make([]blocks.RODataColumn, 0, len(preparedDataColumnSidecars))
	for _, verifiedDataColumnSidecar := range preparedDataColumnSidecars {
		dataColumnSidecar := verifiedDataColumnSidecar.RODataColumn
		dataColumnSidecars = append(dataColumnSidecars, dataColumnSidecar)
	}

	if err := DataColumnsAlignWithBlock(block, dataColumnSidecars); err != nil {
		return nil, errors.Wrap(err, "data columns align with block")
	}

	// Convert verified data column sidecars to verified blob sidecars.
	blobSidecars, err := blobSidecarsFromDataColumnSidecars(block, preparedDataColumnSidecars, indices)
	if err != nil {
		return nil, errors.Wrap(err, "blob sidecars from data column sidecars")
	}

	return blobSidecars, nil
}

// ComputeCellsAndProofsFromFlat computes the cells and proofs from blobs and cell flat proofs.
func ComputeCellsAndProofsFromFlat(blobs [][]byte, cellProofs [][]byte) ([][]kzg.Cell, [][]kzg.Proof, error) {
	const numberOfColumns = fieldparams.NumberOfColumns

	blobCount := uint64(len(blobs))
	cellProofsCount := uint64(len(cellProofs))

	cellsCount := blobCount * numberOfColumns
	if cellsCount != cellProofsCount {
		return nil, nil, ErrBlobsCellsProofsMismatch
	}

	var wg errgroup.Group

	cellsPerBlob := make([][]kzg.Cell, blobCount)
	proofsPerBlob := make([][]kzg.Proof, blobCount)

	for i, blob := range blobs {
		wg.Go(func() error {
			var kzgBlob kzg.Blob
			if copy(kzgBlob[:], blob) != len(kzgBlob) {
				return errors.New("wrong blob size - should never happen")
			}

			// Compute the extended cells from the (non-extended) blob.
			cells, err := kzg.ComputeCells(&kzgBlob)
			if err != nil {
				return errors.Wrap(err, "compute cells")
			}

			proofs := make([]kzg.Proof, 0, numberOfColumns)
			for idx := uint64(i) * numberOfColumns; idx < (uint64(i)+1)*numberOfColumns; idx++ {
				var kzgProof kzg.Proof
				if copy(kzgProof[:], cellProofs[idx]) != len(kzgProof) {
					return errors.New("wrong KZG proof size - should never happen")
				}

				proofs = append(proofs, kzgProof)
			}

			cellsPerBlob[i] = cells
			proofsPerBlob[i] = proofs
			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		return nil, nil, err
	}

	return cellsPerBlob, proofsPerBlob, nil
}

// ComputeCellsAndProofsFromStructured computes the cells and proofs from blobs and cell proofs.
func ComputeCellsAndProofsFromStructured(blobsAndProofs []*pb.BlobAndProofV2) ([][]kzg.Cell, [][]kzg.Proof, error) {
	start := time.Now()
	defer func() {
		cellsAndProofsFromStructuredComputationTime.Observe(float64(time.Since(start).Milliseconds()))
	}()

	var wg errgroup.Group

	cellsPerBlob := make([][]kzg.Cell, len(blobsAndProofs))
	proofsPerBlob := make([][]kzg.Proof, len(blobsAndProofs))

	for i, blobAndProof := range blobsAndProofs {
		if blobAndProof == nil {
			return nil, nil, ErrNilBlobAndProof
		}

		wg.Go(func() error {
			var kzgBlob kzg.Blob
			if copy(kzgBlob[:], blobAndProof.Blob) != len(kzgBlob) {
				return errors.New("wrong blob size - should never happen")
			}

			// Compute the extended cells from the (non-extended) blob.
			cells, err := kzg.ComputeCells(&kzgBlob)
			if err != nil {
				return errors.Wrap(err, "compute cells")
			}

			kzgProofs := make([]kzg.Proof, 0, fieldparams.NumberOfColumns)
			for _, kzgProofBytes := range blobAndProof.KzgProofs {
				if len(kzgProofBytes) != kzg.BytesPerProof {
					return errors.New("wrong KZG proof size - should never happen")
				}

				var kzgProof kzg.Proof
				if copy(kzgProof[:], kzgProofBytes) != len(kzgProof) {
					return errors.New("wrong copied KZG proof size - should never happen")
				}

				kzgProofs = append(kzgProofs, kzgProof)
			}

			cellsPerBlob[i] = cells
			proofsPerBlob[i] = kzgProofs
			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		return nil, nil, err
	}

	return cellsPerBlob, proofsPerBlob, nil
}

// ReconstructBlobs reconstructs blobs from data column sidecars without computing KZG proofs or creating sidecars.
// This is an optimized version for when only the blob data is needed (e.g., for the GetBlobs endpoint).
// The following constraints must be satisfied:
//   - All `dataColumnSidecars` must be committed to the same block, and
//   - `dataColumnSidecars` must be sorted by index and should not contain duplicates.
//   - `dataColumnSidecars` must contain either all sidecars corresponding to (non-extended) blobs,
//   - or enough sidecars to reconstruct the blobs.
func ReconstructBlobs(verifiedDataColumnSidecars []blocks.VerifiedRODataColumn, indices []int, blobCount int) ([][]byte, error) {
	// If no specific indices are requested, populate with all blob indices.
	if len(indices) == 0 {
		indices = make([]int, blobCount)
		for i := range indices {
			indices[i] = i
		}
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

	// Verify that the actual blob count from the first sidecar matches the expected count
	referenceSidecar := verifiedDataColumnSidecars[0]
	actualBlobCount := len(referenceSidecar.Column)
	if actualBlobCount != blobCount {
		return nil, errors.Errorf("blob count mismatch: expected %d, got %d", blobCount, actualBlobCount)
	}

	// Check if the blob index is too high.
	for _, blobIndex := range indices {
		if blobIndex >= blobCount {
			return nil, ErrBlobIndexTooHigh
		}
	}

	// Check if all columns have the same length and are committed to the same block.
	blockRoot := referenceSidecar.BlockRoot()
	for _, sidecar := range verifiedDataColumnSidecars[1:] {
		if len(sidecar.Column) != blobCount {
			return nil, ErrColumnLengthsDiffer
		}

		if sidecar.BlockRoot() != blockRoot {
			return nil, ErrBlockRootMismatch
		}
	}

	// Check if we have all non-extended columns (0..63) - if so, no reconstruction needed.
	hasAllNonExtendedColumns := verifiedDataColumnSidecars[cellsPerBlob-1].Index == uint64(cellsPerBlob-1)

	var reconstructedCells map[int][]kzg.Cell
	if !hasAllNonExtendedColumns {
		// Need to reconstruct cells (but NOT proofs) for the requested blobs only.
		var err error
		reconstructedCells, err = recoverCellsForBlobs(verifiedDataColumnSidecars, indices)
		if err != nil {
			return nil, errors.Wrap(err, "recover cells")
		}
	}

	// Extract blob data without computing proofs.
	blobs := make([][]byte, 0, len(indices))
	for _, blobIndex := range indices {
		var blob kzg.Blob

		// Compute the content of the blob.
		for columnIndex := range cellsPerBlob {
			var cell []byte
			if hasAllNonExtendedColumns {
				// Use existing cells from sidecars
				cell = verifiedDataColumnSidecars[columnIndex].Column[blobIndex]
			} else {
				// Use reconstructed cells
				cell = reconstructedCells[blobIndex][columnIndex][:]
			}

			if copy(blob[kzg.BytesPerCell*columnIndex:], cell) != kzg.BytesPerCell {
				return nil, errors.New("wrong cell size - should never happen")
			}
		}

		blobs = append(blobs, blob[:])
	}

	return blobs, nil
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
