package das

import (
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// NeedSpan represents the need for a resource over a span of slots.
type NeedSpan struct {
	Begin primitives.Slot
	End   primitives.Slot
}

// At returns whether blocks/blobs/columns are needed At the given slot.
func (n NeedSpan) At(slot primitives.Slot) bool {
	return slot >= n.Begin && slot < n.End
}

// CurrentNeeds fields can be used to check whether the given resource type is needed
// at a given slot. The values are based on the current slot, so this value shouldn't
// be retained / reused across slots.
type CurrentNeeds struct {
	Block NeedSpan
	Blob  NeedSpan
	Col   NeedSpan
}

// SyncNeeds holds configuration and state for determining what data is needed
// at any given slot during backfill based on the current slot.
type SyncNeeds struct {
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

type CurrentSlotter func() primitives.Slot

func NewSyncNeeds(current CurrentSlotter, oldestSlotFlagPtr *primitives.Slot, blobRetentionFlag primitives.Epoch) (SyncNeeds, error) {
	deneb, err := slots.EpochStart(params.BeaconConfig().DenebForkEpoch)
	if err != nil {
		return SyncNeeds{}, errors.Wrap(err, "deneb fork slot")
	}
	fuluBoundary := min(params.BeaconConfig().FuluForkEpoch, slots.MaxSafeEpoch())
	fulu, err := slots.EpochStart(fuluBoundary)
	if err != nil {
		return SyncNeeds{}, errors.Wrap(err, "fulu fork slot")
	}
	sn := SyncNeeds{
		current:           func() primitives.Slot { return current() },
		deneb:             deneb,
		fulu:              fulu,
		blobRetentionFlag: blobRetentionFlag,
	}
	// We apply the --blob-retention-epochs flag to both blob and column retention.
	sn.blobRetention = max(sn.blobRetentionFlag, params.BeaconConfig().MinEpochsForBlobsSidecarsRequest)
	sn.colRetention = max(sn.blobRetentionFlag, params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest)

	// Override spec minimum block retention with user-provided flag only if it is lower than the spec minimum.
	sn.blockRetention = primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests)

	if oldestSlotFlagPtr != nil {
		if *oldestSlotFlagPtr <= syncEpochOffset(current(), sn.blockRetention) {
			sn.validOldestSlotPtr = oldestSlotFlagPtr
		} else {
			log.WithField("backfill-oldest-slot", *oldestSlotFlagPtr).
				WithField("specMinSlot", syncEpochOffset(current(), sn.blockRetention)).
				Warn("Ignoring user-specified slot > MIN_EPOCHS_FOR_BLOCK_REQUESTS.")
		}
	}

	return sn, nil
}

// Currently is the main callback given to the different parts of backfill to determine
// what resources are needed at a given slot. It assumes the current instance of SyncNeeds
// is the result of calling initialize.
func (n SyncNeeds) Currently() CurrentNeeds {
	current := n.current()
	c := CurrentNeeds{
		Block: n.blockSpan(current),
		Blob:  NeedSpan{Begin: syncEpochOffset(current, n.blobRetention), End: n.fulu},
		Col:   NeedSpan{Begin: syncEpochOffset(current, n.colRetention), End: current},
	}
	// Adjust the minimums forward to the slots where the sidecar types were introduced
	c.Blob.Begin = max(c.Blob.Begin, n.deneb)
	c.Col.Begin = max(c.Col.Begin, n.fulu)

	return c
}

func (n SyncNeeds) blockSpan(current primitives.Slot) NeedSpan {
	if n.validOldestSlotPtr != nil { // assumes validation done in initialize()
		return NeedSpan{Begin: *n.validOldestSlotPtr, End: current}
	}
	return NeedSpan{Begin: syncEpochOffset(current, n.blockRetention), End: current}
}

func (n SyncNeeds) BlobRetentionChecker() RetentionChecker {
	return func(slot primitives.Slot) bool {
		current := n.Currently()
		return current.Blob.At(slot)
	}
}

func (n SyncNeeds) DataColumnRetentionChecker() RetentionChecker {
	return func(slot primitives.Slot) bool {
		current := n.Currently()
		return current.Col.At(slot)
	}
}

// syncEpochOffset subtracts a number of epochs as slots from the current slot, with underflow checks.
// It returns slot 1 if the result would be 0 or underflow. It doesn't return slot 0 because the
// genesis block needs to be specially synced (it doesn't have a valid signature).
func syncEpochOffset(current primitives.Slot, subtract primitives.Epoch) primitives.Slot {
	minEpoch := min(subtract, slots.MaxSafeEpoch())
	// compute slot offset - offset is a number of slots to go back from current (not an absolute slot).
	offset := slots.UnsafeEpochStart(minEpoch)
	// Undeflow protection: slot 0 is the genesis block, therefore the signature in it is invalid.
	// To prevent us from rejecting a batch, we restrict the minimum backfill batch till only slot 1
	if offset >= current {
		return 1
	}
	return current - offset
}
