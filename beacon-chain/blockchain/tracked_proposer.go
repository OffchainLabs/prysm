package blockchain

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// trackedProposer returns the owned-validator entry for the slot's proposer
// if the BN's VC manages that validator. Foreign proposers are filtered
// out: this is strictly an ownership check.
func (s *Service) trackedProposer(st state.ReadOnlyBeaconState, slot primitives.Slot) (cache.ProposerPreference, bool) {
	id, err := helpers.BeaconProposerIndexAtSlot(s.ctx, st, slot)
	if features.Get().PrepareAllPayloads {
		if err != nil {
			return cache.ProposerPreference{}, true
		}
		if val, ok := s.cfg.ProposerPreferencesCache.Validator(id); ok {
			return val, true
		}
		return cache.ProposerPreference{}, true
	}
	if err != nil {
		return cache.ProposerPreference{}, false
	}
	return s.cfg.ProposerPreferencesCache.Validator(id)
}
