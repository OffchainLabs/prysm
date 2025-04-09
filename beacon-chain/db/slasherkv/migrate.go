package slasherkv

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
)

// Migrate , its corresponding usage and tests can be totally removed once Electra is on mainnet.
// Previously, the first 8 bytes of keys of `attestation-data-roots` and `proposal-records` buckets
// were stored as little-endian respectively epoch and slots. It was the source of
// https://github.com/prysmaticlabs/prysm/issues/14142 and potentially
// https://github.com/prysmaticlabs/prysm/issues/13658.
// To solve this (or these) issue(s), we decided to store the first 8 bytes of keys as big-endian.
// See https://github.com/prysmaticlabs/prysm/pull/14151.
// However, not to break the backward compatibility, we need to migrate the existing data.
// The strategy is quite simple: If, for these bucket keys in the store, we detect
// a slot (resp. epoch) higher, than the current slot (resp. epoch), then we consider that the data
// is stored in little-endian. We create a new entry with the same value, but with the slot (resp. epoch)
// part in the key stored as a big-endian.
// We start the iterate by the highest key and iterate down until we reach the current slot (resp. epoch).
func (s *Store) Migrate(ctx context.Context, headEpoch, maxPruningEpoch primitives.Epoch, batchSize int) error {
	// Migrate attestations.
	log.Info("Starting migration of attestations. This may take a while.")
	start := time.Now()

	if err := s.migrateAttestations(ctx, headEpoch, maxPruningEpoch, batchSize); err != nil {
		return errors.Wrap(err, "migrate attestations")
	}

	log.WithField("duration", time.Since(start)).Info("Migration of attestations completed successfully")

	// Migrate proposals.
	log.Info("Starting migration of proposals. This may take a while.")
	start = time.Now()

	if err := s.migrateProposals(ctx, headEpoch, maxPruningEpoch, batchSize); err != nil {
		return errors.Wrap(err, "migrate proposals")
	}

	log.WithField("duration", time.Since(start)).Info("Migration of proposals completed successfully")

	return nil
}

func (s *Store) migrateAttestations(ctx context.Context, headEpoch, maxPruningEpoch primitives.Epoch, batchSize int) error {
	done := false
	var epochLittleEndian uint64

	for !done {
		count := 0

		if err := s.db.Update(func(tx *bolt.Tx) error {
			signingRootsBkt := tx.Bucket(attestationDataRootsBucket)
			attRecordsBkt := tx.Bucket(attestationRecordsBucket)

			// We begin a migrating iteration starting from the last item in the bucket.
			c := signingRootsBkt.Cursor()
			for k, v := c.Last(); k != nil; k, v = c.Prev() {
				if count >= batchSize {
					log.WithField("epoch", epochLittleEndian).Info("Migrated attestations")

					return nil
				}

				// Check if the context is done.
				if ctx.Err() != nil {
					return ctx.Err()
				}

				// Extract the epoch encoded in the first 8 bytes of the key.
				encodedEpoch := k[:8]

				// Convert it to an uint64, considering it is stored as big-endian.
				epochBigEndian := binary.BigEndian.Uint64(encodedEpoch)

				// If the epoch is smaller or equal to the current epoch, we are done.
				if epochBigEndian <= uint64(headEpoch) {
					break
				}

				// Otherwise, we consider that the epoch is stored as little-endian.
				epochLittleEndian = binary.LittleEndian.Uint64(encodedEpoch)

				// Increment the count of migrated items.
				count++

				// If the epoch is still higher than the current epoch, then it is an issue.
				// This should never happen.
				if epochLittleEndian > uint64(headEpoch) {
					log.WithFields(logrus.Fields{
						"epochLittleEndian": epochLittleEndian,
						"epochBigEndian":    epochBigEndian,
						"headEpoch":         headEpoch,
					}).Error("Epoch is higher than the current epoch both if stored as little-endian or as big-endian")

					continue
				}

				epoch := primitives.Epoch(epochLittleEndian)
				if err := signingRootsBkt.Delete(k); err != nil {
					return err
				}

				// We don't bother migrating data that is going to be pruned by the pruning routine.
				if epoch <= maxPruningEpoch {
					if err := attRecordsBkt.Delete(v); err != nil {
						return err
					}

					continue
				}

				// Create a new key with the epoch stored as big-endian.
				newK := make([]byte, 8)
				binary.BigEndian.PutUint64(newK, uint64(epoch))
				newK = append(newK, k[8:]...)

				// Store the same value with the new key.
				if err := signingRootsBkt.Put(newK, v); err != nil {
					return err
				}
			}

			done = true

			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) migrateProposals(ctx context.Context, headEpoch, maxPruningEpoch primitives.Epoch, batchSize int) error {
	done := false

	if !done {
		count := 0

		// Compute the max pruning slot.
		maxPruningSlot, err := slots.EpochEnd(maxPruningEpoch)
		if err != nil {
			return errors.Wrap(err, "compute max pruning slot")
		}

		// Compute the head slot.
		headSlot, err := slots.EpochEnd(headEpoch)
		if err != nil {
			return errors.Wrap(err, "compute head slot")
		}

		if err := s.db.Update(func(tx *bolt.Tx) error {
			proposalBkt := tx.Bucket(proposalRecordsBucket)

			// We begin a migrating iteration starting from the last item in the bucket.
			c := proposalBkt.Cursor()
			for k, v := c.Last(); k != nil; k, v = c.Prev() {
				if count >= batchSize {
					return nil
				}

				// Check if the context is done.
				if ctx.Err() != nil {
					return ctx.Err()
				}

				// Extract the slot encoded in the first 8 bytes of the key.
				encodedSlot := k[:8]

				// Convert it to an uint64, considering it is stored as big-endian.
				slotBigEndian := binary.BigEndian.Uint64(encodedSlot)

				// If the epoch is smaller or equal to the current epoch, we are done.
				if slotBigEndian <= uint64(headSlot) {
					break
				}

				// Otherwise, we consider that the epoch is stored as little-endian.
				slotLittleEndian := binary.LittleEndian.Uint64(encodedSlot)

				// If the slot is still higher than the current slot, then it is an issue.
				// This should never happen.
				if slotLittleEndian > uint64(headSlot) {
					log.WithFields(logrus.Fields{
						"slotLittleEndian": slotLittleEndian,
						"slotBigEndian":    slotBigEndian,
						"headSlot":         headSlot,
					}).Error("Slot is higher than the current slot both if stored as little-endian or as big-endian")

					continue
				}

				slot := primitives.Slot(slotLittleEndian)
				if err := proposalBkt.Delete(k); err != nil {
					return err
				}

				// We don't bother migrating data that is going to be pruned by the pruning routine.
				if slot <= maxPruningSlot {
					continue
				}

				// Create a new key with the epoch stored as big-endian.
				newK := make([]byte, 8)
				binary.BigEndian.PutUint64(newK, uint64(slot))
				newK = append(newK, k[8:]...)

				// Store the same value with the new key.
				if err := proposalBkt.Put(newK, v); err != nil {
					return err
				}
			}

			done = true

			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}
