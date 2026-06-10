package peerdas

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/pkg/errors"
)

const kzgPosition = 11 // The index of the KZG commitment list in the Body

var (
	ErrIndexTooLarge               = errors.New("column index is larger than the specified columns count")
	ErrNoKzgCommitments            = errors.New("no KZG commitments found")
	ErrMismatchLength              = errors.New("mismatch in the length of the column, commitments or proofs")
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
	index := sidecar.Index()
	if index >= fieldparams.NumberOfColumns {
		return ErrIndexTooLarge
	}

	// A sidecar for zero blobs is invalid.
	kzgCommitments, err := sidecar.KzgCommitments()
	if err != nil {
		return errors.Wrap(err, "kzg commitments")
	}
	if len(kzgCommitments) == 0 {
		return ErrNoKzgCommitments
	}

	// A sidecar with more commitments than the max blob count for this block is invalid.
	slot := sidecar.Slot()
	maxBlobsPerBlock := params.BeaconConfig().MaxBlobsPerBlock(slot)
	if len(kzgCommitments) > maxBlobsPerBlock {
		return ErrTooManyCommitments
	}

	// The column length must be equal to the number of commitments/proofs.
	column := sidecar.Column()
	kzgProofs := sidecar.KzgProofs()
	if len(column) != len(kzgCommitments) || len(column) != len(kzgProofs) {
		return ErrMismatchLength
	}

	return nil
}

// VerifyDataColumnsCellsKZGProofs verifies that the given cell/proof bundles are correct.
// Note: We are slightly deviating from the specification here:
// The specification verifies the KZG proofs for each sidecar separately,
// while we verify all the KZG proofs in a single batch.
// This is done to improve performance since the internal KZG library is way more
// efficient when verifying in batch.
//
// https://github.com/ethereum/consensus-specs/blob/master/specs/gloas/p2p-interface.md#modified-verify_data_column_sidecar_kzg_proofs
func VerifyDataColumnsCellsKZGProofs(cellProofs []blocks.CellProofBundle) error {
	commitments := make([]kzg.Bytes48, 0, len(cellProofs))
	indices := make([]uint64, 0, len(cellProofs))
	cells := make([]kzg.Cell, 0, len(cellProofs))
	proofs := make([]kzg.Bytes48, 0, len(cellProofs))

	for _, bundle := range cellProofs {
		var (
			commitment kzg.Bytes48
			cell       kzg.Cell
			proof      kzg.Bytes48
		)

		if len(bundle.Commitment) != len(commitment) ||
			len(bundle.Cell) != len(cell) ||
			len(bundle.Proof) != len(proof) {
			return ErrMismatchLength
		}

		copy(commitment[:], bundle.Commitment)
		copy(cell[:], bundle.Cell)
		copy(proof[:], bundle.Proof)

		commitments = append(commitments, commitment)
		indices = append(indices, bundle.ColumnIndex)
		cells = append(cells, cell)
		proofs = append(proofs, proof)
	}

	// Batch verify that the cells match the corresponding commitments and proofs.
	verified, err := kzg.VerifyCellKZGProofBatch(commitments, indices, cells, proofs)
	if err != nil {
		return errors.Wrap(err, "verify cell KZG proof batch")
	}

	if !verified {
		return ErrInvalidKZGProof
	}

	return nil
}

// VerifyDataColumnSidecarInclusionProof verifies if the given KZG commitments included in the given beacon block.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/p2p-interface.md#verify_data_column_sidecar_inclusion_proof
func VerifyDataColumnSidecarInclusionProof(sidecar blocks.RODataColumn) error {
	if sidecar.IsGloas() {
		return nil
	}
	signedBlockHeader, err := sidecar.SignedBlockHeader()
	if err != nil {
		return errors.Wrap(err, "signed block header")
	}
	if signedBlockHeader == nil || signedBlockHeader.Header == nil {
		return ErrNilBlockHeader
	}

	root := signedBlockHeader.Header.BodyRoot
	if len(root) != fieldparams.RootLength {
		return ErrBadRootLength
	}

	kzgCommitments, err := sidecar.KzgCommitments()
	if err != nil {
		return errors.Wrap(err, "kzg commitments")
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

	kzgInclusionProof, err := sidecar.KzgCommitmentsInclusionProof()
	if err != nil {
		return errors.Wrap(err, "kzg commitments inclusion proof")
	}
	verified := trie.VerifyMerkleProof(root, hashTreeRoot[:], kzgPosition, kzgInclusionProof)
	if !verified {
		return ErrInvalidInclusionProof
	}

	return nil
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
