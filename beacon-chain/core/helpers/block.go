package helpers

import (
	"math"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// ErrProposerDependentRootUnderflow is returned by ProposerDependentRoot when
// the proposal epoch is less than 2, in which case the spec falls back to the
// genesis block root — callers must supply that themselves.
var ErrProposerDependentRootUnderflow = errors.New("proposer dependent root: epoch < 2")

// BlockRootAtSlot returns the block root stored in the BeaconState for a recent slot.
// It returns an error if the requested block root is not within the slot range.
//
// Spec pseudocode definition:
//
//	def get_block_root_at_slot(state: BeaconState, slot: Slot) -> Root:
//	  """
//	  Return the block root at a recent ``slot``.
//	  """
//	  assert slot < state.slot <= slot + SLOTS_PER_HISTORICAL_ROOT
//	  return state.block_roots[slot % SLOTS_PER_HISTORICAL_ROOT]
func BlockRootAtSlot(state state.ReadOnlyBeaconState, slot primitives.Slot) ([]byte, error) {
	if math.MaxUint64-slot < params.BeaconConfig().SlotsPerHistoricalRoot {
		return []byte{}, errors.New("slot overflows uint64")
	}
	if slot >= state.Slot() || state.Slot() > slot+params.BeaconConfig().SlotsPerHistoricalRoot {
		return []byte{}, errors.Errorf("slot %d out of bounds", slot)
	}
	return state.BlockRootAtIndex(uint64(slot % params.BeaconConfig().SlotsPerHistoricalRoot))
}

// StateRootAtSlot returns the cached state root at that particular slot. If no state
// root has been cached it will return a zero-hash.
func StateRootAtSlot(state state.ReadOnlyBeaconState, slot primitives.Slot) ([]byte, error) {
	if slot >= state.Slot() || state.Slot() > slot+params.BeaconConfig().SlotsPerHistoricalRoot {
		return []byte{}, errors.Errorf("slot %d out of bounds", slot)
	}
	return state.StateRootAtIndex(uint64(slot % params.BeaconConfig().SlotsPerHistoricalRoot))
}

// BlockRoot returns the block root stored in the BeaconState for epoch start slot.
//
// Spec pseudocode definition:
//
//	def get_block_root(state: BeaconState, epoch: Epoch) -> Root:
//	  """
//	  Return the block root at the start of a recent ``epoch``.
//	  """
//	  return get_block_root_at_slot(state, compute_start_slot_at_epoch(epoch))
func BlockRoot(state state.ReadOnlyBeaconState, epoch primitives.Epoch) ([]byte, error) {
	s, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, err
	}
	return BlockRootAtSlot(state, s)
}

// ProposerDependentRoot is the spec's get_proposer_dependent_root(state, epoch(slot)) =
// state.block_roots[start_slot(epoch(slot)-1) - 1]. Returns
// ErrProposerDependentRootUnderflow when the proposal epoch is < 2; the spec's
// fallback to the genesis block root is the caller's responsibility.
func ProposerDependentRoot(st state.ReadOnlyBeaconState, slot primitives.Slot) ([32]byte, error) {
	epoch := slots.ToEpoch(slot)
	if epoch < 2 {
		return [32]byte{}, ErrProposerDependentRootUnderflow
	}
	boundary, err := slots.EpochStart(epoch - 1)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "epoch start")
	}
	rootBytes, err := BlockRootAtSlot(st, boundary-1)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "block root at slot")
	}
	return bytesutil.ToBytes32(rootBytes), nil
}
