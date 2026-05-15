package helpers

import (
	"context"
	"math"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// GenesisBlockRootReader is the minimal beacon DB surface needed to fetch the
// genesis block root for the spec's epoch < 2 fallback.
type GenesisBlockRootReader interface {
	GenesisBlockRoot(ctx context.Context) ([32]byte, error)
}

// ProposerDependentRootOrGenesis wraps state.ProposerDependentRoot with the
// spec-mandated genesis fallback: when proposal epoch < 2 the dependent root
// is the genesis block root.
func ProposerDependentRootOrGenesis(ctx context.Context, db GenesisBlockRootReader, st state.ReadOnlyBeaconState, slot primitives.Slot) ([32]byte, error) {
	if slots.ToEpoch(slot) < 2 {
		root, err := db.GenesisBlockRoot(ctx)
		if err != nil {
			return [32]byte{}, errors.Wrap(err, "genesis block root")
		}
		return root, nil
	}
	return st.ProposerDependentRoot(slot)
}

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
