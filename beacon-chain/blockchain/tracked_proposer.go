package blockchain

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
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

// trackedProposer returns whether the beacon node was informed, via the
// validators/prepare_proposer endpoint, of the proposer at the given slot.
// It only returns true if the tracked proposer is present and active.
//
// When PrepareAllPayloads is enabled, the node prepares payloads for every
// slot. After the Gloas fork, proposers broadcast their preferences (fee
// recipient, gas limit) via gossip into the ProposerPreferencesCache. When
// available, these preferences supply the fee recipient; otherwise the
// default (burn address) is used.
func (s *Service) trackedProposer(st state.ReadOnlyBeaconState, slot primitives.Slot) (cache.TrackedValidator, bool) {
	if features.Get().PrepareAllPayloads {
		if val, ok := s.proposerPreference(slot); ok {
			return val, true
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
