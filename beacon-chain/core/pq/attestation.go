package pq

import (
	"bytes"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// Spec: get_latest_block_slot
func LatestBlockSlot(st state.ReadOnlyBeaconState, slot, parentSlot primitives.Slot) (primitives.Slot, error) {
	if parentSlot <= slot {
		return parentSlot, nil
	}
	root, err := helpers.BlockRootAtSlot(st, slot)
	if err != nil {
		return 0, err
	}
	for slot > 0 && slot+params.BeaconConfig().SlotsPerHistoricalRoot > st.Slot() {
		prev, err := helpers.BlockRootAtSlot(st, slot-1)
		if err != nil {
			return 0, err
		}
		if !bytes.Equal(prev, root) {
			break
		}
		slot--
	}
	return slot, nil
}
