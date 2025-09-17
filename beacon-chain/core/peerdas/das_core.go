package peerdas

import (
	"encoding/binary"
	"math"
	"slices"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/crypto/hash"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/holiman/uint256"
	"github.com/pkg/errors"
)

var (
	// Custom errors
	ErrCustodyGroupTooLarge           = errors.New("custody group too large")
	ErrCustodyGroupCountTooLarge      = errors.New("custody group count too large")
	errWrongComputedCustodyGroupCount = errors.New("wrong computed custody group count, should never happen")

	// maxUint256 is the maximum value of an uint256.
	maxUint256 = &uint256.Int{math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64}
)

// CustodyGroups computes the custody groups the node should participate in for custody.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/das-core.md#get_custody_groups
func CustodyGroups(nodeId enode.ID, custodyGroupCount uint64) ([]uint64, error) {
	numberOfCustodyGroups := params.BeaconConfig().NumberOfCustodyGroups

	// Check if the custody group count is larger than the number of custody groups.
	if custodyGroupCount > numberOfCustodyGroups {
		return nil, ErrCustodyGroupCountTooLarge
	}

	// Shortcut if all custody groups are needed.
	if custodyGroupCount == numberOfCustodyGroups {
		custodyGroups := make([]uint64, 0, numberOfCustodyGroups)
		for i := range numberOfCustodyGroups {
			custodyGroups = append(custodyGroups, i)
		}

		return custodyGroups, nil
	}

	one := uint256.NewInt(1)

	custodyGroupsMap := make(map[uint64]bool, custodyGroupCount)
	custodyGroups := make([]uint64, 0, custodyGroupCount)
	for currentId := new(uint256.Int).SetBytes(nodeId.Bytes()); uint64(len(custodyGroups)) < custodyGroupCount; {
		// Convert to big endian bytes.
		currentIdBytesBigEndian := currentId.Bytes32()

		// Convert to little endian.
		currentIdBytesLittleEndian := bytesutil.ReverseByteOrder(currentIdBytesBigEndian[:])

		// Hash the result.
		hashedCurrentId := hash.Hash(currentIdBytesLittleEndian)

		// Get the custody group ID.
		custodyGroup := binary.LittleEndian.Uint64(hashedCurrentId[:8]) % numberOfCustodyGroups

		// Add the custody group to the map.
		if !custodyGroupsMap[custodyGroup] {
			custodyGroupsMap[custodyGroup] = true
			custodyGroups = append(custodyGroups, custodyGroup)
		}

		if currentId.Cmp(maxUint256) == 0 {
			// Overflow prevention.
			currentId = uint256.NewInt(0)
		} else {
			// Increment the current ID.
			currentId.Add(currentId, one)
		}
	}

	// Final check.
	if uint64(len(custodyGroups)) != custodyGroupCount {
		return nil, errWrongComputedCustodyGroupCount
	}

	// Sort the custody groups.
	slices.Sort[[]uint64](custodyGroups)

	return custodyGroups, nil
}

// ComputeColumnsForCustodyGroup computes the columns for a given custody group.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/das-core.md#compute_columns_for_custody_group
func ComputeColumnsForCustodyGroup(custodyGroup uint64) ([]uint64, error) {
	beaconConfig := params.BeaconConfig()
	numberOfCustodyGroups := beaconConfig.NumberOfCustodyGroups

	if custodyGroup >= numberOfCustodyGroups {
		return nil, ErrCustodyGroupTooLarge
	}

	numberOfColumns := beaconConfig.NumberOfColumns

	columnsPerGroup := numberOfColumns / numberOfCustodyGroups

	columns := make([]uint64, 0, columnsPerGroup)
	for i := range columnsPerGroup {
		column := numberOfCustodyGroups*i + custodyGroup
		columns = append(columns, column)
	}

	return columns, nil
}

// ComputeCustodyGroupForColumn computes the custody group for a given column.
// It is the reciprocal function of ComputeColumnsForCustodyGroup.
func ComputeCustodyGroupForColumn(columnIndex uint64) (uint64, error) {
	beaconConfig := params.BeaconConfig()
	numberOfColumns := beaconConfig.NumberOfColumns
	numberOfCustodyGroups := beaconConfig.NumberOfCustodyGroups

	if columnIndex >= numberOfColumns {
		return 0, ErrIndexTooLarge
	}

	return columnIndex % numberOfCustodyGroups, nil
}

// CustodyColumns computes the custody columns from the custody groups.
func CustodyColumns(custodyGroups []uint64) (map[uint64]bool, error) {
	numberOfCustodyGroups := params.BeaconConfig().NumberOfCustodyGroups

	custodyGroupCount := len(custodyGroups)

	// Compute the columns for each custody group.
	columns := make(map[uint64]bool, custodyGroupCount)
	for _, group := range custodyGroups {
		if group >= numberOfCustodyGroups {
			return nil, ErrCustodyGroupTooLarge
		}

		groupColumns, err := ComputeColumnsForCustodyGroup(group)
		if err != nil {
			return nil, errors.Wrap(err, "compute columns for custody group")
		}

		for _, column := range groupColumns {
			columns[column] = true
		}
	}

	return columns, nil
}
