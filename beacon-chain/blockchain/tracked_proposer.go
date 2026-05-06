package blockchain

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// proposerPreference returns a TrackedValidator from the ProposerPreferencesCache
// if a preference exists for the given slot.
func (s *Service) proposerPreference(slot primitives.Slot) (cache.TrackedValidator, bool) {
	if s.cfg.ProposerPreferencesCache == nil {
		return cache.TrackedValidator{}, false
	}
	pref, ok := s.cfg.ProposerPreferencesCache.Get(slot)
	if !ok {
		return cache.TrackedValidator{}, false
	}
	var feeRecipient primitives.ExecutionAddress
	copy(feeRecipient[:], pref.FeeRecipient)
	return cache.TrackedValidator{Active: true, FeeRecipient: feeRecipient, GasLimit: pref.GasLimit}, true
}

// trackedProposer returns whether the beacon node was informed of the proposer
// at the given slot. Post-Gloas the proposer-preferences cache is the sole
// source; pre-Gloas it falls back to TrackedValidatorsCache.
func (s *Service) trackedProposer(st state.ReadOnlyBeaconState, slot primitives.Slot) (cache.TrackedValidator, bool) {
	if features.Get().PrepareAllPayloads {
		if val, ok := s.proposerPreference(slot); ok {
			return val, true
		}
		return cache.TrackedValidator{Active: true}, true
	}
	if slots.ToEpoch(slot) >= params.BeaconConfig().GloasForkEpoch {
		if pref, ok := s.proposerPreference(slot); ok {
			return pref, true
		}
		return cache.TrackedValidator{Active: true}, true
	}
	id, err := helpers.BeaconProposerIndexAtSlot(s.ctx, st, slot)
	if err != nil {
		return cache.TrackedValidator{}, false
	}
	val, ok := s.cfg.TrackedValidatorsCache.Validator(id)
	if !ok {
		return cache.TrackedValidator{}, false
	}
	if pref, ok := s.proposerPreference(slot); ok {
		return pref, true
	}
	return val, val.Active
}
