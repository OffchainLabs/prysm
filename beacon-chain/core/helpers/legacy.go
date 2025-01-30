package helpers

import (
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// HasDepositRequestStarted determines if the deposit requests have started.
func HasDepositRequestStarted(beaconState state.BeaconState) bool {
    if beaconState.Version() < version.Electra {
        return false
    }

    requestsStartIndex, err := beaconState.DepositRequestsStartIndex()
    if err != nil {
        return false
    }

    return beaconState.Eth1DepositIndex() == requestsStartIndex
}
