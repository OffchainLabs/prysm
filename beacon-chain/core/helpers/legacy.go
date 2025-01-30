package helpers

import (
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// DepositRequestsHaveStarted determines if the deposit requests have started
func DepositRequestsHaveStarted(beaconState state.BeaconState) bool {
	if beaconState.Version() >= version.Electra {
		requestsStartIndex, err := beaconState.DepositRequestsStartIndex()
		if err == nil {
			// deposit_requests_start_index will only be set once
			// eth1data will be frozen
			if beaconState.Eth1DepositIndex() == requestsStartIndex {
				return true
			}
		}
	}
	return false
}
