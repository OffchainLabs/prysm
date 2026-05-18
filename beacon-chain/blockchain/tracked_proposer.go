package blockchain

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/pkg/errors"
)

// trackedProposer returns the preference for the slot's proposer if the BN's
// VC is attached to that validator (per beacon_committee_subscriptions). A nil
// return with no error means the slot's proposer is not ours (caller should
// skip the payload build). On preference-cache miss the returned pref has an
// empty FeeRecipient and callers fall back to DefaultFeeRecipient.
func (s *Service) trackedProposer(st state.ReadOnlyBeaconState, slot primitives.Slot) (*cache.ProposerPreference, error) {
	id, err := helpers.BeaconProposerIndexAtSlot(s.ctx, st, slot)
	if features.Get().PrepareAllPayloads {
		if err != nil {
			return &cache.ProposerPreference{}, nil
		}
		pref, err := s.preferenceForProposer(st, slot, id)
		if err != nil {
			return nil, err
		}
		return &pref, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "beacon proposer index")
	}
	if !s.cfg.SubscribedValidatorsCache.Has(id) {
		return nil, nil
	}
	pref, err := s.preferenceForProposer(st, slot, id)
	if err != nil {
		return nil, err
	}
	return &pref, nil
}

func (s *Service) preferenceForProposer(st state.ReadOnlyBeaconState, slot primitives.Slot, id primitives.ValidatorIndex) (cache.ProposerPreference, error) {
	dependentRoot, err := helpers.ProposerDependentRootOrGenesis(s.ctx, s.cfg.BeaconDB, st, slot)
	if err != nil {
		return cache.ProposerPreference{ValidatorIndex: id}, errors.Wrap(err, "proposer dependent root")
	}
	if pref, ok := s.cfg.ProposerPreferencesCache.BestFor(dependentRoot, slot, id); ok {
		return pref, nil
	}
	return cache.ProposerPreference{ValidatorIndex: id}, nil
}
