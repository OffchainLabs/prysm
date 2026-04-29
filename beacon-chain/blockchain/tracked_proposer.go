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
// to the checkpoint at epoch(slot)-1 reachable from headRoot.
func (s *Service) proposerPreference(slot primitives.Slot, valIdx primitives.ValidatorIndex, headRoot [32]byte) (cache.TrackedValidator, bool) {
	if s.cfg.ProposerPreferencesCache == nil {
		return cache.TrackedValidator{}, false
	}
	checkpointRoot, ok := s.proposerCheckpointRoot(slot, headRoot)
	if !ok {
		return cache.TrackedValidator{}, false
	}
	pref, ok := s.cfg.ProposerPreferencesCache.Get(checkpointRoot, slot)
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

// proposerCheckpointRoot is the spec's get_checkpoint_block(store, headRoot, epoch(slot)-1).
func (s *Service) proposerCheckpointRoot(slot primitives.Slot, headRoot [32]byte) ([32]byte, bool) {
	proposalEpoch := slots.ToEpoch(slot)
	if proposalEpoch == 0 {
		return [32]byte{}, false
	}
	boundarySlot, err := slots.EpochStart(proposalEpoch - 1)
	if err != nil {
		return [32]byte{}, false
	}
	ar, err := s.Ancestor(s.ctx, headRoot[:], boundarySlot)
	if err != nil {
		return [32]byte{}, false
	}
	return bytesutil.ToBytes32(ar), true
}

// trackedProposer returns whether the beacon node was informed, via the
// validators/prepare_proposer endpoint, of the proposer at the given slot.
// Post-Gloas, a cached ProposerPreference (keyed by the checkpoint at
// epoch(slot)-1 reached from headRoot) overrides the tracked validator when
// present.
func (s *Service) trackedProposer(st state.ReadOnlyBeaconState, slot primitives.Slot, headRoot [32]byte) (cache.TrackedValidator, bool) {
	if features.Get().PrepareAllPayloads {
		id, err := helpers.BeaconProposerIndexAtSlot(s.ctx, st, slot)
		if err != nil {
			return cache.TrackedValidator{Active: true}, true
		}
		if val, ok := s.proposerPreference(slot, id, headRoot); ok {
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
	if pref, ok := s.proposerPreference(slot, id, headRoot); ok {
		return pref, true
	}
	return val, val.Active
}
