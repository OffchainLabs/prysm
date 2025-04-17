package peerdas

import (
	beaconState "github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/pkg/errors"
)

// ValidatorsCustodyRequirement returns the number of custody groups regarding the validator indices attached to the beacon node.
// https://github.com/ethereum/consensus-specs/blob/v1.5.0-beta.5/specs/fulu/validator.md#validator-custody
func ValidatorsCustodyRequirement(state beaconState.ReadOnlyBeaconState, validatorsIndex map[primitives.ValidatorIndex]bool) (uint64, error) {
	totalNodeBalance := uint64(0)
	for index := range validatorsIndex {
		balance, err := state.BalanceAtIndex(index)
		if err != nil {
			return 0, errors.Wrapf(err, "balance at index for validator index %v", index)
		}

		totalNodeBalance += balance
	}

	beaconConfig := params.BeaconConfig()
	numberOfCustodyGroup := beaconConfig.NumberOfCustodyGroups
	validatorCustodyRequirement := beaconConfig.ValidatorCustodyRequirement
	balancePerAdditionalCustodyGroup := beaconConfig.BalancePerAdditionalCustodyGroup

	count := totalNodeBalance / balancePerAdditionalCustodyGroup
	return min(max(count, validatorCustodyRequirement), numberOfCustodyGroup), nil
}
