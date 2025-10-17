package kv

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
)

// UpdateCustodyInfo atomically updates the custody group count if it is greater than the stored one,
// and updates the earliest available slot.
// The earliest available slot can decrease (for backfill scenarios) or increase (for pruning scenarios).
// It returns the (potentially updated) custody group count and earliest available slot.
func (s *Store) UpdateCustodyInfo(ctx context.Context, earliestAvailableSlot primitives.Slot, custodyGroupCount uint64) (primitives.Slot, uint64, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.UpdateCustodyInfo")
	defer span.End()

	storedGroupCount, storedEarliestAvailableSlot := uint64(0), primitives.Slot(0)
	if err := s.db.Update(func(tx *bolt.Tx) error {
		// Retrieve the custody bucket.
		bucket, err := tx.CreateBucketIfNotExists(custodyBucket)
		if err != nil {
			return errors.Wrap(err, "create custody bucket")
		}

		// Retrieve the stored custody group count.
		storedGroupCountBytes := bucket.Get(groupCountKey)
		if len(storedGroupCountBytes) != 0 {
			storedGroupCount = bytesutil.BytesToUint64BigEndian(storedGroupCountBytes)
		}

		// Retrieve the stored earliest available slot.
		storedEarliestAvailableSlotBytes := bucket.Get(earliestAvailableSlotKey)
		if len(storedEarliestAvailableSlotBytes) != 0 {
			storedEarliestAvailableSlot = primitives.Slot(bytesutil.BytesToUint64BigEndian(storedEarliestAvailableSlotBytes))
		}

		originalStoredGroupCount := storedGroupCount

		// Update custody group count only if it increased.
		if custodyGroupCount > storedGroupCount {
			storedGroupCount = custodyGroupCount
			bytes := bytesutil.Uint64ToBytesBigEndian(custodyGroupCount)
			if err := bucket.Put(groupCountKey, bytes); err != nil {
				return errors.Wrap(err, "put custody group count")
			}
		}

		// Allow decrease (for backfill scenarios)
		// When backfilling blocks, we're discovering earlier data, so earliestAvailableSlot
		// should decrease to reflect the newly available earlier blocks.
		if earliestAvailableSlot <= storedEarliestAvailableSlot {
			storedEarliestAvailableSlot = earliestAvailableSlot
			bytes := bytesutil.Uint64ToBytesBigEndian(uint64(earliestAvailableSlot))
			if err := bucket.Put(earliestAvailableSlotKey, bytes); err != nil {
				return errors.Wrap(err, "put earliest available slot")
			}
			return nil
		}

		// Update earliest available slot if it advanced.
		// This is for pruning to work correctly as blocks are pruned,
		// the earliest available slot moves forward independently of custody group changes.
		//
		// IMPORTANT: If we're increasing earliestAvailableSlot without also increasing
		// custodyGroupCount, we must ensure the new slot doesn't exceed the minimum
		// required slot (based on MIN_EPOCHS_FOR_BLOCK_REQUESTS from current time).
		// This prevents nodes from arbitrarily refusing to serve mandatory historical data.
		// If custody group count is NOT increasing, validate the increase is allowed
		// Use originalStoredGroupCount to check if custody is actually increasing in this call
		if custodyGroupCount <= originalStoredGroupCount {
			genesisTime := time.Unix(int64(params.BeaconConfig().MinGenesisTime+params.BeaconConfig().GenesisDelay), 0)
			currentSlot := slots.CurrentSlot(genesisTime)
			currentEpoch := slots.ToEpoch(currentSlot)
			minEpochsForBlocks := primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests)

			// Calculate the minimum required epoch (or 0 if we're early in the chain)
			minRequiredEpoch := primitives.Epoch(0)
			if currentEpoch > minEpochsForBlocks {
				minRequiredEpoch = currentEpoch - minEpochsForBlocks
			}

			// Convert to slot to ensure we compare at slot-level granularity
			minRequiredSlot, err := slots.EpochStart(minRequiredEpoch)
			if err != nil {
				return errors.Wrap(err, "calculate minimum required slot")
			}

			// Prevent any increase that would put earliest available slot beyond the minimum required slot
			// when custody group count is not increasing
			if earliestAvailableSlot > minRequiredSlot {
				return errors.Errorf(
					"cannot increase earliest available slot to %d (epoch %d) without increasing custody group count, "+
						"as it exceeds minimum required slot %d (epoch %d). This would prevent serving mandatory historical data.",
					earliestAvailableSlot, slots.ToEpoch(earliestAvailableSlot),
					minRequiredSlot, minRequiredEpoch,
				)
			}
		}

		storedEarliestAvailableSlot = earliestAvailableSlot
		bytes := bytesutil.Uint64ToBytesBigEndian(uint64(earliestAvailableSlot))
		if err := bucket.Put(earliestAvailableSlotKey, bytes); err != nil {
			return errors.Wrap(err, "put earliest available slot")
		}

		return nil
	}); err != nil {
		return 0, 0, err
	}

	log.WithFields(logrus.Fields{
		"earliestAvailableSlot": storedEarliestAvailableSlot,
		"groupCount":            storedGroupCount,
	}).Debug("Custody info")

	return storedEarliestAvailableSlot, storedGroupCount, nil
}

// UpdateSubscribedToAllDataSubnets updates the "subscribed to all data subnets" status in the database
// only if `subscribed` is `true`.
// It returns the previous subscription status.
func (s *Store) UpdateSubscribedToAllDataSubnets(ctx context.Context, subscribed bool) (bool, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.UpdateSubscribedToAllDataSubnets")
	defer span.End()

	result := false
	if !subscribed {
		if err := s.db.View(func(tx *bolt.Tx) error {
			// Retrieve the custody bucket.
			bucket := tx.Bucket(custodyBucket)
			if bucket == nil {
				return nil
			}

			// Retrieve the subscribe all data subnets flag.
			bytes := bucket.Get(subscribeAllDataSubnetsKey)
			if len(bytes) == 0 {
				return nil
			}

			if bytes[0] == 1 {
				result = true
			}

			return nil
		}); err != nil {
			return false, err
		}

		return result, nil
	}

	if err := s.db.Update(func(tx *bolt.Tx) error {
		// Retrieve the custody bucket.
		bucket, err := tx.CreateBucketIfNotExists(custodyBucket)
		if err != nil {
			return errors.Wrap(err, "create custody bucket")
		}

		bytes := bucket.Get(subscribeAllDataSubnetsKey)
		if len(bytes) != 0 && bytes[0] == 1 {
			result = true
		}

		if err := bucket.Put(subscribeAllDataSubnetsKey, []byte{1}); err != nil {
			return errors.Wrap(err, "put subscribe all data subnets")
		}

		return nil
	}); err != nil {
		return false, err
	}

	return result, nil
}
