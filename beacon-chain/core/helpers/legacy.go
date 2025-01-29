package helpers

import (
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/math"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// IsLegacyDepositProcessPeriod determines if the current state should use the legacy deposit process.
func IsLegacyDepositProcessPeriod(beaconState state.BeaconState, canonicalEth1Data *ethpb.Eth1Data) bool {
	// Before the Electra upgrade, always use the legacy deposit process.
	if beaconState.Version() < version.Electra {
		return true
	}

	// Handle the transition period between the legacy and the new deposit process.
	requestsStartIndex, err := beaconState.DepositRequestsStartIndex()
	if err != nil {
		// If we can't get the deposit requests start index,
		// we should default to the legacy deposit process.
		return true
	}

	//canonicalEth1Data should never be nil
	eth1DepositIndexLimit := math.Min(canonicalEth1Data.DepositCount, requestsStartIndex)
	return beaconState.Eth1DepositIndex() < eth1DepositIndexLimit
}
