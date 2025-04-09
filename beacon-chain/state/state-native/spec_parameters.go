package state_native

import (
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

func (b *BeaconState) ProportionalSlashingMultiplier() (uint64, error) {
	if b.version >= version.Bellatrix {
		return params.BeaconConfig().ProportionalSlashingMultiplierBellatrix, nil
	}

	if b.version >= version.Altair {
		return params.BeaconConfig().ProportionalSlashingMultiplierAltair, nil
	}

	if b.version >= version.Phase0 {
		return params.BeaconConfig().ProportionalSlashingMultiplier, nil
	}

	return 0, errNotSupported("ProportionalSlashingMultiplier", b.version)
}

func (b *BeaconState) InactivityPenaltyQuotient() (uint64, error) {
	if b.version >= version.Bellatrix {
		return params.BeaconConfig().InactivityPenaltyQuotientBellatrix, nil
	}

	if b.version >= version.Altair {
		return params.BeaconConfig().InactivityPenaltyQuotientAltair, nil
	}

	if b.version >= version.Phase0 {
		return params.BeaconConfig().InactivityPenaltyQuotient, nil
	}

	return 0, errNotSupported("InactivityPenaltyQuotient", b.version)
}
