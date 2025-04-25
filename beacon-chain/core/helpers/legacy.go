package helpers

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
)

// DepositRequestsStarted determines if the deposit requests have started.
func DepositRequestsStarted(beaconState state.BeaconState) bool {
	if beaconState.Version() < version.Electra {
		return false
	}

	requestsStartIndex, err := beaconState.DepositRequestsStartIndex()
	if err != nil {
		return false
	}

	return beaconState.Eth1DepositIndex() == requestsStartIndex
}
