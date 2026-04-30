package cache

import (
	"encoding/binary"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// Given input state `st`, balance key is constructed as:
// (block_root in `st` at epoch_start_slot - 1) + current_epoch + validator_count
func balanceCacheKey(st state.ReadOnlyBeaconState) (string, error) {
	currentEpoch := primitives.Epoch(st.Slot().DivSlot(params.BeaconConfig().SlotsPerEpoch))
	return computeBalanceCacheKey(st, currentEpoch)
}

// nextEpochBalanceCacheKey computes the balance cache key as if state.Slot were
// at the start of the upcoming epoch. It is used during epoch processing
// (when state.Slot is still the last slot of the current epoch) to prime the
// cache for the first block of the next epoch.
func nextEpochBalanceCacheKey(st state.ReadOnlyBeaconState) (string, error) {
	currentEpoch := primitives.Epoch(st.Slot().DivSlot(params.BeaconConfig().SlotsPerEpoch))
	return computeBalanceCacheKey(st, currentEpoch+1)
}

func computeBalanceCacheKey(st state.ReadOnlyBeaconState, currentEpoch primitives.Epoch) (string, error) {
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	epochStartSlot, err := slotsPerEpoch.SafeMul(uint64(currentEpoch))
	if err != nil {
		// impossible condition due to early division
		return "", fmt.Errorf("start slot calculation overflows: %w", err)
	}
	prevSlot := primitives.Slot(0)
	if epochStartSlot > 1 {
		prevSlot = epochStartSlot - 1
	}
	r, err := st.BlockRootAtIndex(uint64(prevSlot % params.BeaconConfig().SlotsPerHistoricalRoot))
	if err != nil {
		// impossible condition because index is always constrained within state
		return "", err
	}

	// Mix in current epoch
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(currentEpoch))
	key := append(r, b...)

	// Mix in validator count
	b = make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(st.NumValidators()))
	key = append(key, b...)

	return string(key), nil
}
