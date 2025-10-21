package peerdas

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/container/trie"
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
	numberOfColumns := params.BeaconConfig().NumberOfColumns
	if sidecar.Index >= numberOfColumns {
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

// VerifyDataColumnsSidecarKZGProofs verifies if the KZG proofs are correct.
// Note: We are slightly deviating from the specification here:
// The specification verifies the KZG proofs for each sidecar separately,
// while we are verifying all the KZG proofs from multiple sidecars in a batch.
// This is done to improve performance since the internal KZG library is way more
// efficient when verifying in batch.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/p2p-interface.md#verify_data_column_sidecar_kzg_proofs
func VerifyDataColumnsSidecarKZGProofs(sidecars []blocks.RODataColumn) error {
	// Compute the total count.
	count := 0
	for _, sidecar := range sidecars {
		count += len(sidecar.Column)
	}

	commitments := make([]kzg.Bytes48, 0, count)
	indices := make([]uint64, 0, count)
	cells := make([]kzg.Cell, 0, count)
	proofs := make([]kzg.Bytes48, 0, count)

	for _, sidecar := range sidecars {
		for i := range sidecar.Column {
			var (
				commitment kzg.Bytes48
				cell       kzg.Cell
				proof      kzg.Bytes48
			)

			commitmentBytes := sidecar.KzgCommitments[i]
			cellBytes := sidecar.Column[i]
			proofBytes := sidecar.KzgProofs[i]

			if len(commitmentBytes) != len(commitment) ||
				len(cellBytes) != len(cell) ||
				len(proofBytes) != len(proof) {
				return ErrMismatchLength
			}

			copy(commitment[:], commitmentBytes)
			copy(cell[:], cellBytes)
			copy(proof[:], proofBytes)

			commitments = append(commitments, commitment)
			indices = append(indices, sidecar.Index)
			cells = append(cells, cell)
			proofs = append(proofs, proof)
		}
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
	if sidecar.SignedBlockHeader == nil || sidecar.SignedBlockHeader.Header == nil {
		return ErrNilBlockHeader
	}

	root := sidecar.SignedBlockHeader.Header.BodyRoot
	if len(root) != fieldparams.RootLength {
		return ErrBadRootLength
	}

	leaves := blocks.LeavesFromCommitments(sidecar.KzgCommitments)

	sparse, err := trie.GenerateTrieFromItems(leaves, fieldparams.LogMaxBlobCommitments)
	if err != nil {
		return errors.Wrap(err, "generate trie from items")
	}

	hashTreeRoot, err := sparse.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "hash tree root")
	}

	verified := trie.VerifyMerkleProof(root, hashTreeRoot[:], kzgPosition, sidecar.KzgCommitmentsInclusionProof)
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
