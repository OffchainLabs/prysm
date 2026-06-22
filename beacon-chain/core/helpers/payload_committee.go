package helpers

import (
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// PayloadCommitteeAvailable reports whether a beacon state at stateSlot can resolve the payload
// timeliness committee for slot, i.e. slot falls within the state's cached PTC window of
// [previous epoch, current epoch + MIN_SEED_LOOKAHEAD].
func PayloadCommitteeAvailable(stateSlot, slot primitives.Slot) bool {
	stateEpoch := slots.ToEpoch(stateSlot)
	slotEpoch := slots.ToEpoch(slot)
	return slotEpoch+1 >= stateEpoch && slotEpoch <= stateEpoch+params.BeaconConfig().MinSeedLookahead
}
