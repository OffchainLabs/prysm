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
// at the given slot. Pre-Gloas the source is the validators/prepare_proposer
// endpoint via TrackedValidatorsCache, with proposer preferences acting as an
// override. Post-Gloas the proposer-preferences cache is the only source; the
// TrackedValidatorsCache is bypassed since the validator client no longer
// pushes prepare_proposer entries.
func (s *Service) trackedProposer(st state.ReadOnlyBeaconState, slot primitives.Slot) (cache.TrackedValidator, bool) {
	if features.Get().PrepareAllPayloads {
		if val, ok := s.proposerPreference(slot); ok {
			return val, true
		}
		return cache.TrackedValidator{Active: true}, true
	}
	if slots.ToEpoch(slot) >= params.BeaconConfig().GloasForkEpoch {
		// Post-Gloas: source fee recipient + gas limit exclusively from the
		// proposer-preferences cache. On a cache miss, signal "tracked but
		// empty" so payload-attribute helpers fall back to the burn address
		// (overridden by --suggested-fee-recipient via setFeeRecipientIfBurnAddress).
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
