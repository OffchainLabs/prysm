package blockchain

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// proposerPreference looks up the cached preference for (slot, valIdx) anchored
// to the dependent_root derived from the given state.
func (s *Service) proposerPreference(st state.ReadOnlyBeaconState, slot primitives.Slot, valIdx primitives.ValidatorIndex) (cache.TrackedValidator, bool) {
	if s.cfg.ProposerPreferencesCache == nil {
		return cache.TrackedValidator{}, false
	}
	dependentRoot, ok := s.proposerDependentRoot(st, slot)
	if !ok {
		return cache.TrackedValidator{}, false
	}
	pref, ok := s.cfg.ProposerPreferencesCache.Get(dependentRoot, slot)
	if !ok {
		return cache.TrackedValidator{}, false
	}
	if pref.ValidatorIndex != valIdx {
		return cache.TrackedValidator{}, false
	}
	var feeRecipient primitives.ExecutionAddress
	copy(feeRecipient[:], pref.FeeRecipient)
	return cache.TrackedValidator{Active: true, FeeRecipient: feeRecipient, GasLimit: pref.GasLimit}, true
}

// proposerDependentRoot is the spec's get_proposer_dependent_root(state, epoch(slot)).
// Returns false on slot underflow (proposal epoch < 2) — the caller treats that
// as a cache miss; the spec's "use genesis" fallback only matters for genesis-
// adjacent slots which never carry Gloas preferences.
func (s *Service) proposerDependentRoot(st state.ReadOnlyBeaconState, slot primitives.Slot) ([32]byte, bool) {
	proposalEpoch := slots.ToEpoch(slot)
	if proposalEpoch < 2 {
		return [32]byte{}, false
	}
	boundary, err := slots.EpochStart(proposalEpoch - 1)
	if err != nil {
		return [32]byte{}, false
	}
	rootBytes, err := helpers.BlockRootAtSlot(st, boundary-1)
	if err != nil {
		return [32]byte{}, false
	}
	return bytesutil.ToBytes32(rootBytes), true
}

// trackedProposer returns whether the beacon node was informed, via the
// validators/prepare_proposer endpoint, of the proposer at the given slot.
// Post-Gloas, a cached ProposerPreference (keyed by the dependent_root derived
// from `st`) overrides the tracked validator when present.
func (s *Service) trackedProposer(st state.ReadOnlyBeaconState, slot primitives.Slot) (cache.TrackedValidator, bool) {
	if features.Get().PrepareAllPayloads {
		id, err := helpers.BeaconProposerIndexAtSlot(s.ctx, st, slot)
		if err != nil {
			return cache.TrackedValidator{Active: true}, true
		}
		if val, ok := s.proposerPreference(st, slot, id); ok {
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
	if pref, ok := s.proposerPreference(st, slot, id); ok {
		return pref, true
	}
	return val, val.Active
}
