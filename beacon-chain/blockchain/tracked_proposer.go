package blockchain

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

// trackedProposer returns the preference for the slot's proposer if the
// BN's VC manages that validator. Foreign proposers are filtered out via
// the owned-cache ownership gate. The branch-specific external entry
// (keyed by (slot, dependent_root)) wins for spec alignment.
//
// Pre-Gloas: external is empty for our validators, so we fall back to the
// owned entry (populated by prepare_beacon_proposer).
//
// Post-Gloas: a missing branch-specific entry indicates dependent_root
// drift between the Submit and the FCU — we surface this with a warning
// and a metric, and return an empty-fee preference so the caller falls
// through to the default fee path (which itself logs the deprecated
// --suggested-fee-recipient warning).
func (s *Service) trackedProposer(st state.ReadOnlyBeaconState, slot primitives.Slot) (cache.ProposerPreference, bool) {
	id, err := helpers.BeaconProposerIndexAtSlot(s.ctx, st, slot)
	if features.Get().PrepareAllPayloads {
		if err != nil {
			return cache.ProposerPreference{}, true
		}
		if val, ok := s.preferenceForOwnedProposer(st, slot, id); ok {
			return val, true
		}
		return cache.ProposerPreference{}, true
	}
	if err != nil {
		return cache.ProposerPreference{}, false
	}
	return s.preferenceForOwnedProposer(st, slot, id)
}

func (s *Service) preferenceForOwnedProposer(st state.ReadOnlyBeaconState, slot primitives.Slot, id primitives.ValidatorIndex) (cache.ProposerPreference, bool) {
	val, ok := s.cfg.ProposerPreferencesCache.Validator(id)
	if !ok {
		return cache.ProposerPreference{}, false
	}
	dependentRoot, err := helpers.ProposerDependentRoot(st, slot)
	if err != nil {
		return val, true
	}
	if pref, ok := s.cfg.ProposerPreferencesCache.Get(dependentRoot, slot); ok && pref.ValidatorIndex == id {
		return pref, true
	}
	if slots.ToEpoch(slot) >= params.BeaconConfig().GloasForkEpoch {
		log.WithFields(logrus.Fields{
			"slot":           slot,
			"dependentRoot":  fmt.Sprintf("%#x", dependentRoot),
			"validatorIndex": id,
		}).Warn("No proposer preference for current branch (dependent_root drift); falling through to default fee recipient. Validator client should re-Submit.")
		return cache.ProposerPreference{ValidatorIndex: id}, true
	}
	return val, true
}
