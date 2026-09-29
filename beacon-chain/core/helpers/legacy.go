package helpers

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// DepositRequestsStarted determines if the deposit requests have started.
func DepositRequestsStarted(beaconState state.BeaconState) bool {
	// The former deposit mechanism is removed, so deposit requests are always started.
	if beaconState.Version() >= version.Fulu {
		return true
	}

	if beaconState.Version() < version.Electra {
		return false
	}

	// Only Electra checks deposit requests start index.
	requestsStartIndex, err := beaconState.DepositRequestsStartIndex()
	if err != nil {
		return false
	}

	return beaconState.Eth1DepositIndex() == requestsStartIndex
}
