package peerdas

import (
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
)

const (
	CustodyGroupCountEnrKey = "cgc"
)

var (
	// Custom errors
	errRecordNil                   = errors.New("record is nil")
	errCannotLoadCustodyGroupCount = errors.New("cannot load the custody group count from peer")
	errIndexTooLarge               = errors.New("column index is larger than the specified columns count")
	errMismatchLength              = errors.New("mismatch in the length of the commitments and proofs")
)

// https://github.com/ethereum/consensus-specs/blob/v1.5.0-alpha.10/specs/fulu/p2p-interface.md#the-discovery-domain-discv5
type Cgc uint64

func (Cgc) ENRKey() string { return CustodyGroupCountEnrKey }

// VerifyDataColumnsSidecarKZGProofs verifies the provided KZG Proofs of data columns.
// https://github.com/ethereum/consensus-specs/blob/dev/specs/fulu/p2p-interface.md#verify_data_column_sidecar_kzg_proofs
func VerifyDataColumnsSidecarKZGProofs(sidecars []blocks.RODataColumn) (bool, error) {
	// Retrieve the number of columns.
	numberOfColumns := params.BeaconConfig().NumberOfColumns

	// Compute the total count.
	count := 0
	for _, sidecar := range sidecars {
		count += len(sidecar.DataColumn)
	}

	commitments := make([]kzg.Bytes48, 0, count)
	indices := make([]uint64, 0, count)
	cells := make([]kzg.Cell, 0, count)
	proofs := make([]kzg.Bytes48, 0, count)

	for _, sidecar := range sidecars {
		// Check if the columns index is not too large
		if sidecar.ColumnIndex >= numberOfColumns {
			return false, errIndexTooLarge
		}

		// Check if the KZG commitments size and data column size match.
		if len(sidecar.DataColumn) != len(sidecar.KzgCommitments) {
			return false, errMismatchLength
		}

		// Check if the KZG proofs size and data column size match.
		if len(sidecar.DataColumn) != len(sidecar.KzgProof) {
			return false, errMismatchLength
		}

		for i := range sidecar.DataColumn {
			commitments = append(commitments, kzg.Bytes48(sidecar.KzgCommitments[i]))
			indices = append(indices, sidecar.ColumnIndex)
			cells = append(cells, kzg.Cell(sidecar.DataColumn[i]))
			proofs = append(proofs, kzg.Bytes48(sidecar.KzgProof[i]))
		}
	}

	// Verify all the batch at once.
	verified, err := kzg.VerifyCellKZGProofBatch(commitments, indices, cells, proofs)
	if err != nil {
		return false, errors.Wrap(err, "verify cell KZG proof batch")
	}

	return verified, nil
}

// ComputeSubnetForDataColumnSidecar computes the subnet for a data column sidecar.
// https://github.com/ethereum/consensus-specs/blob/dev/specs/fulu/p2p-interface.md#compute_subnet_for_data_column_sidecar
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

// ComputeCustodyGroupForColumn computes the custody group for a given column.
// It is the reciprocal function of ComputeColumnsForCustodyGroup.
func ComputeCustodyGroupForColumn(columnIndex uint64) (uint64, error) {
	beaconConfig := params.BeaconConfig()
	numberOfColumns := beaconConfig.NumberOfColumns

	if columnIndex >= numberOfColumns {
		return 0, errIndexTooLarge
	}

	numberOfCustodyGroups := beaconConfig.NumberOfCustodyGroups
	columnsPerGroup := numberOfColumns / numberOfCustodyGroups

	return columnIndex / columnsPerGroup, nil
}

// CustodyGroupCount returns the number of groups we should participate in for custody.
func CustodyGroupCount() uint64 {
	if flags.Get().SubscribeToAllSubnets {
		return params.BeaconConfig().NumberOfCustodyGroups
	}

	return params.BeaconConfig().CustodyRequirement
}

// CustodyGroupCountFromRecord extracts the custody group count from an ENR record.
func CustodyGroupCountFromRecord(record *enr.Record) (uint64, error) {
	if record == nil {
		return 0, errRecordNil
	}

	// Load the `cgc`
	var cgc Cgc
	if cgc := record.Load(&cgc); cgc != nil {
		return 0, errCannotLoadCustodyGroupCount
	}

	return uint64(cgc), nil
}
