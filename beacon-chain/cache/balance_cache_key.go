package cache

import (
	"encoding/binary"
	"fmt"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

// Given input state `st`, balance key is constructed as:
// (block_root in `st` at epoch_start_slot - 1) + current_epoch + validator_count
func balanceCacheKey(st state.ReadOnlyBeaconState) (string, error) {
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	currentEpoch := st.Slot().DivSlot(slotsPerEpoch)
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
