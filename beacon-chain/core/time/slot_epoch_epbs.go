package time

import (
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

// CanUpgradeToEip7732 returns true if the input `slot` can upgrade to EIP-7732(epbs).
// Spec code:
// If state.slot % SLOTS_PER_EPOCH == 0 and compute_epoch_at_slot(state.slot) == EIP7732_FORK_EPOCH
func CanUpgradeToEip7732(slot primitives.Slot) bool {
	epochStart := slots.IsEpochStart(slot)
	return epochStart && slots.ToEpoch(slot) == params.BeaconConfig().EPBSForkEpoch
}
