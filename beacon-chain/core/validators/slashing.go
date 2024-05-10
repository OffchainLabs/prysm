package validators

import (
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// SlashingParamsPerVersion returns the slashing parameters for the given state version.
func SlashingParamsPerVersion(v int) (slashingQuotient, proposerRewardQuotient, whistleblowerRewardQuotient uint64, err error) {
	cfg := params.BeaconConfig()
	switch v {
	case version.Phase0:
		slashingQuotient = cfg.MinSlashingPenaltyQuotient
		proposerRewardQuotient = cfg.ProposerRewardQuotient
		whistleblowerRewardQuotient = cfg.WhistleBlowerRewardQuotient
	case version.Altair:
		slashingQuotient = cfg.MinSlashingPenaltyQuotientAltair
		proposerRewardQuotient = cfg.ProposerRewardQuotient
		whistleblowerRewardQuotient = cfg.WhistleBlowerRewardQuotient
	case version.Bellatrix, version.Capella, version.Deneb:
		slashingQuotient = cfg.MinSlashingPenaltyQuotientBellatrix
		proposerRewardQuotient = cfg.ProposerRewardQuotient
		whistleblowerRewardQuotient = cfg.WhistleBlowerRewardQuotient
	case version.Electra, version.EPBS:
		slashingQuotient = cfg.MinSlashingPenaltyQuotientElectra
		proposerRewardQuotient = cfg.ProposerRewardQuotient
		whistleblowerRewardQuotient = cfg.WhistleBlowerRewardQuotientElectra
	default:
		err = errors.New("unknown state version")
	}
	return
}
