package blockchain

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/sirupsen/logrus"
)

// trackedProposer returns the preference for the slot's proposer if the BN's
// VC is attached to that validator (per beacon_committee_subscriptions). On
// preference-cache miss, the returned ProposerPreference has an empty
// FeeRecipient; callers fall back to DefaultFeeRecipient via
// setFeeRecipientIfBurnAddress.
func (s *Service) trackedProposer(st state.ReadOnlyBeaconState, slot primitives.Slot) (cache.ProposerPreference, bool) {
	id, err := helpers.BeaconProposerIndexAtSlot(s.ctx, st, slot)
	if features.Get().PrepareAllPayloads {
		if err != nil {
			return cache.ProposerPreference{}, true
		}
		return s.preferenceForProposer(st, slot, id), true
	}
	if err != nil {
		return cache.ProposerPreference{}, false
	}
	if !s.cfg.SubscribedValidatorsCache.Has(id) {
		return cache.ProposerPreference{}, false
	}
	return s.preferenceForProposer(st, slot, id), true
}

// preferenceForProposer returns the signed preference for (slot, dep_root) if
// present, then falls back to the per-validator default written via
// PrepareBeaconProposer (pre-Gloas), then to an empty preference (caller
// resolves to --suggested-fee-recipient).
func (s *Service) preferenceForProposer(st state.ReadOnlyBeaconState, slot primitives.Slot, id primitives.ValidatorIndex) cache.ProposerPreference {
	dependentRoot, err := helpers.ProposerDependentRoot(st, slot)
	if err == nil {
		if pref, ok := s.cfg.ProposerPreferencesCache.Get(dependentRoot, slot); ok && pref.ValidatorIndex == id {
			return pref
		}
	}
	if def, ok := s.cfg.ProposerPreferencesCache.Default(id); ok {
		return def
	}
	log.WithFields(logrus.Fields{
		"slot":           slot,
		"dependentRoot":  fmt.Sprintf("%#x", dependentRoot),
		"validatorIndex": id,
	}).Debug("No proposer preference cached; falling through to default fee recipient.")
	return cache.ProposerPreference{ValidatorIndex: id}
}
