package kv

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	bolt "go.etcd.io/bbolt"
)

var migrationFinalizedParent = []byte("parent_bug_32fb183")

func migrateFinalizedParent(ctx context.Context, db *bolt.DB) error {
	if updateErr := db.Update(func(tx *bolt.Tx) error {
		mb := tx.Bucket(migrationsBucket)
		if b := mb.Get(migrationFinalizedParent); bytes.Equal(b, migrationCompleted) {
			return nil // Migration already completed.
		}

		bkt := tx.Bucket(finalizedBlockRootsIndexBucket)
		if bkt == nil {
			return fmt.Errorf("unable to read %s bucket for migration", finalizedBlockRootsIndexBucket)
		}
		bb := tx.Bucket(blocksBucket)
		if bb == nil {
			return fmt.Errorf("unable to read %s bucket for migration", blocksBucket)
		}

		c := bkt.Cursor()
		var slotsWithoutBug primitives.Slot
		maxBugSearch := params.BeaconConfig().SlotsPerEpoch * 10
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			// check if context is cancelled in between
			if ctx.Err() != nil {
				return ctx.Err()
			}

			idxEntry := &ethpb.FinalizedBlockRootContainer{}
			if err := decode(ctx, v, idxEntry); err != nil {
				return errors.Wrapf(err, "unable to decode finalized block root container for root=%#x", k)
			}
			// Not one of the corrupt values
			if !bytes.Equal(idxEntry.ParentRoot, k) {
				slotsWithoutBug += 1
				if slotsWithoutBug > maxBugSearch {
					break
				}
				continue
			}
			slotsWithoutBug = 0
			log.WithField("root", fmt.Sprintf("%#x", k)).Debug("found index entry with incorrect parent root")

			// Look up full block to get the correct parent root.
			encBlk := bb.Get(k)
			if encBlk == nil {
				return errors.Wrapf(ErrNotFound, "could not find block for corrupt finalized index entry %#x", k)
			}
			blk, err := unmarshalBlock(ctx, encBlk)
			if err != nil {
				return errors.Wrapf(err, "unable to decode block for root=%#x", k)
			}
			// Replace parent root in the index with the correct value and write it back.
			pr := blk.Block().ParentRoot()
			idxEntry.ParentRoot = pr[:]
			idxEnc, err := encode(ctx, idxEntry)
			if err != nil {
				return errors.Wrapf(err, "failed to encode finalized index entry for root=%#x", k)
			}
			if err := bkt.Put(k, idxEnc); err != nil {
				return errors.Wrapf(err, "failed to update finalized index entry for root=%#x", k)
			}
			log.WithField("root", fmt.Sprintf("%#x", k)).
				WithField("parentRoot", fmt.Sprintf("%#x", idxEntry.ParentRoot)).
				Debug("updated corrupt index entry with correct parent")
		}
		// Mark migration complete.
		return mb.Put(migrationFinalizedParent, migrationCompleted)
	}); updateErr != nil {
		log.WithError(updateErr).Errorf("could not run finalized parent root index repair migration")
		return updateErr
	}
	return nil
}
