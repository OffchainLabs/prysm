package blockchain

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// trackedProposer returns the preference for the slot's proposer if the BN's
// VC is attached to that validator (per beacon_committee_subscriptions). On
// preference-cache miss, the returned ProposerPreference has an empty
// FeeRecipient; callers fall back to DefaultFeeRecipient via
// setFeeRecipientIfBurnAddress.
func (s *Service) trackedProposer(st state.ReadOnlyBeaconState, slot primitives.Slot) (cache.ProposerPreference, bool, error) {
	id, err := helpers.BeaconProposerIndexAtSlot(s.ctx, st, slot)
	if features.Get().PrepareAllPayloads {
		if err != nil {
			return cache.ProposerPreference{}, true, nil
		}
		pref, err := s.preferenceForProposer(st, slot, id)
		return pref, true, err
	}
	if err != nil {
		return cache.ProposerPreference{}, false, nil
	}
	if !s.cfg.SubscribedValidatorsCache.Has(id) {
		return cache.ProposerPreference{}, false, nil
	}
	pref, err := s.preferenceForProposer(st, slot, id)
	return pref, true, err
}

func (s *Service) preferenceForProposer(st state.ReadOnlyBeaconState, slot primitives.Slot, id primitives.ValidatorIndex) (cache.ProposerPreference, error) {
	dependentRoot, err := helpers.ProposerDependentRootOrGenesis(s.ctx, s.cfg.BeaconDB, st, slot)
	if err != nil {
		return cache.ProposerPreference{ValidatorIndex: id}, errors.Wrap(err, "proposer dependent root")
	}
	if pref, ok := s.cfg.ProposerPreferencesCache.BestFor(dependentRoot, slot, id); ok {
		return pref, nil
	}
	log.WithFields(logrus.Fields{
		"slot":           slot,
		"dependentRoot":  fmt.Sprintf("%#x", dependentRoot),
		"validatorIndex": id,
	}).Debug("No proposer preference cached; falling through to default fee recipient.")
	return cache.ProposerPreference{ValidatorIndex: id}, nil
}
