package backfill

import (
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// needSpan represents the need for a resource over a span of slots.
type needSpan struct {
	begin primitives.Slot
	end   primitives.Slot
}

// at returns whether blocks/blobs/columns are needed at the given slot.
func (n needSpan) at(slot primitives.Slot) bool {
	return slot >= n.begin && slot < n.end
}

// currentNeeds fields can be used to check whether the given resource type is needed
// at a given slot. The values are based on the current slot, so this value shouldn't
// be retained / reused across slots.
type currentNeeds struct {
	block needSpan
	blob  needSpan
	col   needSpan
}

// syncNeeds holds configuration and state for determining what data is needed
// at any given slot during backfill based on the current slot.
type syncNeeds struct {
	current func() primitives.Slot
	deneb   primitives.Slot
	fulu    primitives.Slot

	oldestSlotFlagPtr  *primitives.Slot
	validOldestSlotPtr *primitives.Slot
	blockRetention     primitives.Epoch

	blobRetentionFlag primitives.Epoch
	blobRetention     primitives.Epoch
	colRetention      primitives.Epoch
}

// initialize cleans up data and performs validation. Since syncNeeds is usable as a value (not a pointer),
// this method allows the backfill service initialization to collect relevant field values into a syncNeeds instance
// before performing validation, which requires access to current slot (which we don't have during flag processing).
func (c syncNeeds) initialize(current func() primitives.Slot, deneb, fulu primitives.Slot) syncNeeds {
	c.current = current
	c.deneb = deneb
	c.fulu = fulu
	// We apply the --blob-retention-epochs flag to both blob and column retention.
	c.blobRetention = max(c.blobRetentionFlag, params.BeaconConfig().MinEpochsForBlobsSidecarsRequest)
	c.colRetention = max(c.blobRetentionFlag, params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest)

	// Override spec minimum block retention with user-provided flag only if it is lower than the spec minimum.
	c.blockRetention = primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests)
	if c.oldestSlotFlagPtr != nil {
		oldestEpoch := slots.ToEpoch(*c.oldestSlotFlagPtr)
		if oldestEpoch < c.blockRetention {
			c.validOldestSlotPtr = c.oldestSlotFlagPtr
		} else {
			log.WithField("backfill-oldest-slot", *c.oldestSlotFlagPtr).
				WithField("specMinSlot", syncEpochOffset(current(), c.blockRetention)).
				Warn("Ignoring user-specified slot > MIN_EPOCHS_FOR_BLOCK_REQUESTS.")
			c.oldestSlotFlagPtr = nil // unset so nothing uses the invalid value
			c.validOldestSlotPtr = nil
		}
	}

	return c
}

// currently is the main callback given to the different parts of backfill to determine
// what resources are needed at a given slot. It assumes the current instance of syncNeeds
// is the result of calling initialize.
func (n syncNeeds) currently() currentNeeds {
	current := n.current()
	c := currentNeeds{
		block: n.blockSpan(current),
		blob:  needSpan{begin: syncEpochOffset(current, n.blobRetention), end: n.fulu},
		col:   needSpan{begin: syncEpochOffset(current, n.colRetention), end: current},
	}
	// Adjust the minimums forward to the slots where the sidecar types were introduced
	c.blob.begin = max(c.blob.begin, n.deneb)
	c.col.begin = max(c.col.begin, n.fulu)

	return c
}
func (n syncNeeds) blockSpan(current primitives.Slot) needSpan {
	if n.validOldestSlotPtr != nil { // assumes validation done in initialize()
		return needSpan{begin: *n.validOldestSlotPtr, end: current}
	}
	return needSpan{begin: syncEpochOffset(current, n.blockRetention), end: current}
}
