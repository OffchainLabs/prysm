package peerdas

import (
	stderrors "errors"
	"iter"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/pkg/errors"
)

const kzgPosition = 11 // The index of the KZG commitment list in the Body

var (
	ErrIndexTooLarge               = errors.New("column index is larger than the specified columns count")
	ErrNoKzgCommitments            = errors.New("no KZG commitments found")
	ErrMismatchLength              = errors.New("mismatch in the length of the column, commitments or proofs")
	ErrEmptySegment                = errors.New("empty segment in batch")
	ErrInvalidKZGProof             = errors.New("invalid KZG proof")
	ErrBadRootLength               = errors.New("bad root length")
	ErrInvalidInclusionProof       = errors.New("invalid inclusion proof")
	ErrRecordNil                   = errors.New("record is nil")
	ErrNilBlockHeader              = errors.New("nil beacon block header")
	ErrCannotLoadCustodyGroupCount = errors.New("cannot load the custody group count from peer")
)

// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/p2p-interface.md#custody-group-count
type Cgc uint64

func (Cgc) ENRKey() string { return params.BeaconNetworkConfig().CustodyGroupCountKey }

// VerifyDataColumnSidecar verifies if the data column sidecar is valid.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/p2p-interface.md#verify_data_column_sidecar
func VerifyDataColumnSidecar(sidecar blocks.RODataColumn) error {
	// The sidecar index must be within the valid range.
	if sidecar.Index >= fieldparams.NumberOfColumns {
		return ErrIndexTooLarge
	}

	// A sidecar for zero blobs is invalid.
	if len(sidecar.KzgCommitments) == 0 {
		return ErrNoKzgCommitments
	}

	// A sidecar with more commitments than the max blob count for this block is invalid.
	slot := sidecar.Slot()
	maxBlobsPerBlock := params.BeaconConfig().MaxBlobsPerBlock(slot)
	if len(sidecar.KzgCommitments) > maxBlobsPerBlock {
		return ErrTooManyCommitments
	}

	// The column length must be equal to the number of commitments/proofs.
	if len(sidecar.Column) != len(sidecar.KzgCommitments) || len(sidecar.Column) != len(sidecar.KzgProofs) {
		return ErrMismatchLength
	}

	return nil
}

// CellProofBundleSegment is returned when a batch fails. The caller can call
// the `.Verify` method to verify just this segment.
type CellProofBundleSegment struct {
	indices     []uint64
	commitments []kzg.Bytes48
	cells       []kzg.Cell
	proofs      []kzg.Bytes48
}

// Verify verifies this segment without batching.
func (s CellProofBundleSegment) Verify() error {
	if len(s.cells) == 0 {
		return ErrEmptySegment
	}
	verified, err := kzg.VerifyCellKZGProofBatch(s.commitments, s.indices, s.cells, s.proofs)
	if err != nil {
		return stderrors.Join(err, ErrInvalidKZGProof)
	}
	if !verified {
		return ErrInvalidKZGProof
	}
	return nil
}

func VerifyDataColumnsCellsKZGProofs(sizeHint int, cellProofsIter iter.Seq[blocks.CellProofBundle]) error {
	// ignore the failed segment list since we are just passing in one segment.
	_, err := BatchVerifyDataColumnsCellsKZGProofs(sizeHint, []iter.Seq[blocks.CellProofBundle]{cellProofsIter})
	return err
}

// BatchVerifyDataColumnsCellsKZGProofs verifies if the KZG proofs are correct.
// Note: We are slightly deviating from the specification here:
// The specification verifies the KZG proofs for each sidecar separately,
// while we are verifying all the KZG proofs from multiple sidecars in a batch.
// This is done to improve performance since the internal KZG library is way more
// efficient when verifying in batch. If the batch fails, the failed segments
// are returned to the caller so that they may try segment by segment without
// batching. On success the failed segment list is empty.
//
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/p2p-interface.md#verify_data_column_sidecar_kzg_proofs
func BatchVerifyDataColumnsCellsKZGProofs(sizeHint int, cellProofsIters []iter.Seq[blocks.CellProofBundle]) ( /* failed segment list */ []CellProofBundleSegment, error) {
	commitments := make([]kzg.Bytes48, 0, sizeHint)
	indices := make([]uint64, 0, sizeHint)
	cells := make([]kzg.Cell, 0, sizeHint)
	proofs := make([]kzg.Bytes48, 0, sizeHint)

	var anySegmentEmpty bool
	var segments []CellProofBundleSegment
	for _, cellProofsIter := range cellProofsIters {
		startIdx := len(cells)
		for bundle := range cellProofsIter {
			var (
				commitment kzg.Bytes48
				cell       kzg.Cell
				proof      kzg.Bytes48
			)

			if len(bundle.Commitment) != len(commitment) ||
				len(bundle.Cell) != len(cell) ||
				len(bundle.Proof) != len(proof) {
				return nil, ErrMismatchLength
			}

			copy(commitment[:], bundle.Commitment)
			copy(cell[:], bundle.Cell)
			copy(proof[:], bundle.Proof)

			commitments = append(commitments, commitment)
			indices = append(indices, bundle.ColumnIndex)
			cells = append(cells, cell)
			proofs = append(proofs, proof)
		}
		if len(cells[startIdx:]) == 0 {
			anySegmentEmpty = true
		}
		segments = append(segments, CellProofBundleSegment{
			indices:     indices[startIdx:],
			commitments: commitments[startIdx:],
			cells:       cells[startIdx:],
			proofs:      proofs[startIdx:],
		})
	}

	if anySegmentEmpty {
		return segments, ErrEmptySegment
	}

	// Batch verify that the cells match the corresponding commitments and proofs.
	verified, err := kzg.VerifyCellKZGProofBatch(commitments, indices, cells, proofs)
	if err != nil {
		return segments, stderrors.Join(err, ErrInvalidKZGProof)
	}

	if !verified {
		return segments, ErrInvalidKZGProof
	}

	return nil, nil
}

// verifyKzgCommitmentsInclusionProof is the shared implementation for inclusion proof verification.
func verifyKzgCommitmentsInclusionProof(bodyRoot []byte, kzgCommitments [][]byte, inclusionProof [][]byte) error {
	if len(bodyRoot) != fieldparams.RootLength {
		return ErrBadRootLength
	}

	leaves := blocks.LeavesFromCommitments(kzgCommitments)

	sparse, err := trie.GenerateTrieFromItems(leaves, fieldparams.LogMaxBlobCommitments)
	if err != nil {
		return errors.Wrap(err, "generate trie from items")
	}

	hashTreeRoot, err := sparse.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "hash tree root")
	}

	verified := trie.VerifyMerkleProof(bodyRoot, hashTreeRoot[:], kzgPosition, inclusionProof)
	if !verified {
		return ErrInvalidInclusionProof
	}

	return nil
}

// VerifyDataColumnSidecarInclusionProof verifies if the given KZG commitments included in the given beacon block.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/p2p-interface.md#verify_data_column_sidecar_inclusion_proof
func VerifyDataColumnSidecarInclusionProof(sidecar blocks.RODataColumn) error {
	if sidecar.SignedBlockHeader == nil || sidecar.SignedBlockHeader.Header == nil {
		return ErrNilBlockHeader
	}
	return verifyKzgCommitmentsInclusionProof(
		sidecar.SignedBlockHeader.Header.BodyRoot,
		sidecar.KzgCommitments,
		sidecar.KzgCommitmentsInclusionProof,
	)
}

// VerifyPartialDataColumnHeaderInclusionProof verifies if the KZG commitments are included in the beacon block.
func VerifyPartialDataColumnHeaderInclusionProof(header *ethpb.PartialDataColumnHeader) error {
	if header.SignedBlockHeader == nil || header.SignedBlockHeader.Header == nil {
		return ErrNilBlockHeader
	}
	return verifyKzgCommitmentsInclusionProof(
		header.SignedBlockHeader.Header.BodyRoot,
		header.KzgCommitments,
		header.KzgCommitmentsInclusionProof,
	)
}

// ComputeSubnetForDataColumnSidecar computes the subnet for a data column sidecar.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/p2p-interface.md#compute_subnet_for_data_column_sidecar
func ComputeSubnetForDataColumnSidecar(columnIndex uint64) uint64 {
	dataColumnSidecarSubnetCount := params.BeaconConfig().DataColumnSidecarSubnetCount
	return columnIndex % dataColumnSidecarSubnetCount
}

// DataColumnSubnets computes the subnets for the data columns.
func DataColumnSubnets(dataColumns map[uint64]bool) map[uint64]bool {
	subnets := make(map[uint64]bool, len(dataColumns))

	for column := range dataColumns {
		subnet := ComputeSubnetForDataColumnSidecar(column)
		subnets[subnet] = true
	}

	return subnets
}

// CustodyGroupCountFromRecord extracts the custody group count from an ENR record.
func CustodyGroupCountFromRecord(record *enr.Record) (uint64, error) {
	if record == nil {
		return 0, ErrRecordNil
	}

	// Load the `cgc`
	var cgc Cgc
	if err := record.Load(&cgc); err != nil {
		return 0, ErrCannotLoadCustodyGroupCount
	}

	return uint64(cgc), nil
}
