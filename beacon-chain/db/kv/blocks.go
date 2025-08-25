package kv

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filters"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/container/slice"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/common"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	bolt "go.etcd.io/bbolt"
)

// Used to represent errors for inconsistent slot ranges.
var errInvalidSlotRange = errors.New("invalid end slot and start slot provided")

// Block retrieval by root. Return nil if block is not found.
func (s *Store) Block(ctx context.Context, blockRoot [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.Block")
	defer span.End()
	blk, err := s.getBlock(ctx, blockRoot, nil)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return blk, err
}

func (s *Store) getBlock(ctx context.Context, blockRoot [32]byte, tx *bolt.Tx) (interfaces.ReadOnlySignedBeaconBlock, error) {
	if v, ok := s.blockCache.Get(string(blockRoot[:])); v != nil && ok {
		return v, nil
	}
	// This method allows the caller to pass in its tx if one is already open.
	// Or if a nil value is used, a transaction will be managed intenally.
	if tx == nil {
		var err error
		tx, err = s.db.Begin(false)
		if err != nil {
			return nil, err
		}
		defer func() {
			if err := tx.Rollback(); err != nil {
				log.WithError(err).Error("could not rollback read-only getBlock transaction")
			}
		}()
	}
	return unmarshalBlock(ctx, tx.Bucket(blocksBucket).Get(blockRoot[:]))
}

// OriginCheckpointBlockRoot returns the value written to the db in SaveOriginCheckpointBlockRoot
// This is the root of a finalized block within the weak subjectivity period
// at the time the chain was started, used to initialize the database and chain
// without syncing from genesis.
func (s *Store) OriginCheckpointBlockRoot(ctx context.Context) ([32]byte, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.OriginCheckpointBlockRoot")
	defer span.End()

	var root [32]byte
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blocksBucket)
		rootSlice := bkt.Get(originCheckpointBlockRootKey)
		if rootSlice == nil {
			return ErrNotFoundOriginBlockRoot
		}
		copy(root[:], rootSlice)
		return nil
	})

	return root, err
}

// HeadBlockRoot returns the latest canonical block root in the Ethereum Beacon Chain.
func (s *Store) HeadBlockRoot() ([32]byte, error) {
	var root [32]byte
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blocksBucket)
		headRoot := bkt.Get(headBlockRootKey)
		if len(headRoot) == 0 {
			return errors.New("no head block root found")
		}
		copy(root[:], headRoot)
		return nil
	})
	return root, err
}

// HeadBlock returns the latest canonical block in the Ethereum Beacon Chain.
func (s *Store) HeadBlock(ctx context.Context) (interfaces.ReadOnlySignedBeaconBlock, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.HeadBlock")
	defer span.End()
	var headBlock interfaces.ReadOnlySignedBeaconBlock
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blocksBucket)
		headRoot := bkt.Get(headBlockRootKey)
		if headRoot == nil {
			return nil
		}
		enc := bkt.Get(headRoot)
		if enc == nil {
			return nil
		}
		var err error
		headBlock, err = unmarshalBlock(ctx, enc)
		return err
	})
	return headBlock, err
}

// blocksAncestryQuery returns all blocks *before* the descendent block;
// that is: inclusive of q.Earliest, exclusive of q.Descendent.Slot.
func (s *Store) blocksAncestryQuery(ctx context.Context, q filters.AncestryQuery) ([]interfaces.ReadOnlySignedBeaconBlock, [][32]byte, error) {
	// Save resources if no blocks will be found by the query.
	if q.Span() < 1 {
		return nil, nil, filters.ErrInvalidQuery
	}

	blocks := make([]interfaces.ReadOnlySignedBeaconBlock, 0, q.Span())
	roots := make([][32]byte, 0, q.Span())
	// Handle edge case where start and end are equal; slotRootsInRange would see end < start and err.
	// So, just grab the descendent in its own tx and stop there.
	if q.Span() == 1 {
		err := s.db.View(func(tx *bolt.Tx) error {
			descendent, err := s.getBlock(ctx, q.Descendent.Root, tx)
			if err != nil {
				return errors.Wrap(err, "descendent block not in db")
			}
			blocks = append(blocks, descendent)
			roots = append(roots, q.Descendent.Root)
			return nil
		})
		return blocks, roots, err
	}

	// stop before the descendent slot since it is determined by the query
	sr, err := s.slotRootsInRange(ctx, q.Earliest, q.Descendent.Slot-1, -1)
	if err != nil {
		return nil, nil, err
	}
	err = s.db.View(func(tx *bolt.Tx) error {
		descendent, err := s.getBlock(ctx, q.Descendent.Root, tx)
		if err != nil {
			return errors.Wrap(err, "descendent block not in db")
		}
		proot := descendent.Block().ParentRoot()
		lowest := descendent.Block().Slot()
		blocks = append(blocks, descendent)
		roots = append(roots, q.Descendent.Root)
		// slotRootsInRange returns the roots in descending order
		for _, prev := range sr {
			if prev.slot < q.Earliest {
				return nil
			}
			if prev.slot >= lowest {
				continue
			}
			if prev.root == proot {
				p, err := s.getBlock(ctx, prev.root, tx)
				if err != nil {
					return err
				}
				roots = append(roots, prev.root)
				blocks = append(blocks, p)
				proot = p.Block().ParentRoot()
				lowest = p.Block().Slot()
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	slices.Reverse(roots)
	slices.Reverse(blocks)

	return blocks, roots, err
}

// Blocks retrieves a list of beacon blocks and its respective roots by filter criteria.
func (s *Store) Blocks(ctx context.Context, f *filters.QueryFilter) ([]interfaces.ReadOnlySignedBeaconBlock, [][32]byte, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.Blocks")
	defer span.End()

	if q, err := f.GetAncestryQuery(); err == nil {
		return s.blocksAncestryQuery(ctx, q)
	} else {
		if !errors.Is(err, filters.ErrNotSet) {
			return nil, nil, err
		}
	}

	blocks := make([]interfaces.ReadOnlySignedBeaconBlock, 0)
	blockRoots := make([][32]byte, 0)

	if start, end, isSimple := f.SimpleSlotRange(); isSimple {
		return s.blocksForSlotRange(ctx, start, end)
	}

	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blocksBucket)

		keys, err := blockRootsByFilter(ctx, tx, f)
		if err != nil {
			return err
		}

		for i := 0; i < len(keys); i++ {
			encoded := bkt.Get(keys[i])
			blk, err := unmarshalBlock(ctx, encoded)
			if err != nil {
				return errors.Wrapf(err, "could not unmarshal block with key %#x", keys[i])
			}
			blocks = append(blocks, blk)
			blockRoots = append(blockRoots, bytesutil.ToBytes32(keys[i]))
		}
		return nil
	})
	return blocks, blockRoots, err
}

// cleanupMissingBlockIndices cleans up the slot->root mapping, and the parent root index pointing
// from each of these blocks to each of their children. Since we don't have the blocks themselves,
// we don't know their parent root to efficiently clean the index going the other direction.
func (s *Store) cleanupMissingBlockIndices(ctx context.Context, badBlocks []slotRoot) {
	errs := make([]error, 0)
	err := s.db.Update(func(tx *bolt.Tx) error {
		for _, sr := range badBlocks {
			log.WithField("root", fmt.Sprintf("%#x", sr.root)).WithField("slot", sr.slot).Warn("Cleaning up indices for missing block")
			if err := s.deleteSlotIndexEntry(tx, sr.slot, sr.root); err != nil {
				errs = append(errs, errors.Wrapf(err, "failed to clean up slot index entry for root %#x and slot %d", sr.root, sr.slot))
			}
			if err := tx.Bucket(blockParentRootIndicesBucket).Delete(sr.root[:]); err != nil {
				errs = append(errs, errors.Wrapf(err, "failed to clean up block parent index for root %#x", sr.root))
			}
		}
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}
	for _, err := range errs {
		log.WithError(err).Error("Failed to clean up indices for missing block")
	}
}

// blocksForSlotRange gets all blocks and roots for a given slot range.
// This function uses the slot->root index, which can contain multiple entries for the same slot
// in case of forks. It will return all blocks for the given slot range, and the roots of those blocks.
// The [i]th element of the blocks slice corresponds to the [i]th element of the roots slice.
// If a block is not found, it will be added to a slice of missing blocks, which will have their indices cleaned
// in a separate Update transaction before the method returns. This is done to compensate for a previous bug where
// block deletions left danging index entries.
func (s *Store) blocksForSlotRange(ctx context.Context, startSlot, endSlot primitives.Slot) ([]interfaces.ReadOnlySignedBeaconBlock, [][32]byte, error) {
	slotRootPairs, err := s.slotRootsInRange(ctx, startSlot, endSlot, -1) // set batch size to zero to retrieve all
	if err != nil {
		return nil, nil, err
	}
	slices.Reverse(slotRootPairs)
	badBlocks := make([]slotRoot, 0)
	defer func() { s.cleanupMissingBlockIndices(ctx, badBlocks) }()
	roots := make([][32]byte, 0, len(slotRootPairs))
	blks := make([]interfaces.ReadOnlySignedBeaconBlock, 0, len(slotRootPairs))
	err = s.db.View(func(tx *bolt.Tx) error {
		for _, sr := range slotRootPairs {
			blk, err := s.getBlock(ctx, sr.root, tx)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					badBlocks = append(badBlocks, sr)
					continue
				}
				return err
			}
			roots = append(roots, sr.root)
			blks = append(blks, blk)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return blks, roots, nil
}

// BlockRoots retrieves a list of beacon block roots by filter criteria. If the caller
// requires both the blocks and the block roots for a certain filter they should instead
// use the Blocks function rather than use BlockRoots. During periods of non finality
// there are potential race conditions which leads to differing roots when calling the db
// multiple times for the same filter.
func (s *Store) BlockRoots(ctx context.Context, f *filters.QueryFilter) ([][32]byte, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.BlockRoots")
	defer span.End()
	blockRoots := make([][32]byte, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		keys, err := blockRootsByFilter(ctx, tx, f)
		if err != nil {
			return err
		}

		for i := 0; i < len(keys); i++ {
			blockRoots = append(blockRoots, bytesutil.ToBytes32(keys[i]))
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not retrieve block roots")
	}
	return blockRoots, nil
}

// HasBlock checks if a block by root exists in the db.
func (s *Store) HasBlock(ctx context.Context, blockRoot [32]byte) bool {
	_, span := trace.StartSpan(ctx, "BeaconDB.HasBlock")
	defer span.End()
	if v, ok := s.blockCache.Get(string(blockRoot[:])); v != nil && ok {
		return true
	}
	exists := false
	if err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blocksBucket)
		exists = bkt.Get(blockRoot[:]) != nil
		return nil
	}); err != nil { // This view never returns an error, but we'll handle anyway for sanity.
		panic(err) // lint:nopanic -- View never returns an error.
	}
	return exists
}

// BlocksBySlot retrieves a list of beacon blocks and its respective roots by slot.
func (s *Store) BlocksBySlot(ctx context.Context, slot primitives.Slot) ([]interfaces.ReadOnlySignedBeaconBlock, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.BlocksBySlot")
	defer span.End()

	blocks := make([]interfaces.ReadOnlySignedBeaconBlock, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blocksBucket)
		roots, err := blockRootsBySlot(ctx, tx, slot)
		if err != nil {
			return errors.Wrap(err, "could not retrieve blocks by slot")
		}
		for _, r := range roots {
			encoded := bkt.Get(r[:])
			blk, err := unmarshalBlock(ctx, encoded)
			if err != nil {
				return err
			}
			blocks = append(blocks, blk)
		}
		return nil
	})
	return blocks, err
}

// BlockRootsBySlot retrieves a list of beacon block roots by slot
func (s *Store) BlockRootsBySlot(ctx context.Context, slot primitives.Slot) (bool, [][32]byte, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.BlockRootsBySlot")
	defer span.End()
	blockRoots := make([][32]byte, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		blockRoots, err = blockRootsBySlot(ctx, tx, slot)
		return err
	})
	if err != nil {
		return false, nil, errors.Wrap(err, "could not retrieve block roots by slot")
	}
	return len(blockRoots) > 0, blockRoots, nil
}

// DeleteBlock from the db
// This deletes the root entry from all buckets in the blocks DB
// If the block is finalized this function returns an error
func (s *Store) DeleteBlock(ctx context.Context, root [32]byte) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.DeleteBlock")
	defer span.End()

	if err := s.DeleteState(ctx, root); err != nil {
		return err
	}

	if err := s.deleteStateSummary(root); err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(finalizedBlockRootsIndexBucket)
		if b := bkt.Get(root[:]); b != nil {
			return ErrDeleteJustifiedAndFinalized
		}

		// Look up the block to find its slot; needed to remove the slot index entry.
		blk, err := s.getBlock(ctx, root, tx)
		if err != nil {
			// getBlock can return ErrNotFound, in which case we won't even try to delete it.
			if errors.Is(err, ErrNotFound) {
				return nil
			}
			return err
		}
		if err := s.deleteSlotIndexEntry(tx, blk.Block().Slot(), root); err != nil {
			return err
		}
		if err := s.deleteMatchingParentIndex(tx, blk.Block().ParentRoot(), root); err != nil {
			return err
		}
		if err := s.deleteBlock(tx, root[:]); err != nil {
			return err
		}
		s.blockCache.Del(string(root[:]))
		return nil
	})
}

// DeleteHistoricalDataBeforeSlot deletes all blocks and states before the given slot.
// This function deletes data from the following buckets:
// - blocksBucket
// - blockParentRootIndicesBucket
// - finalizedBlockRootsIndexBucket
// - stateBucket
// - stateSummaryBucket
// - blockRootValidatorHashesBucket
// - blockSlotIndicesBucket
// - stateSlotIndicesBucket
func (s *Store) DeleteHistoricalDataBeforeSlot(ctx context.Context, cutoffSlot primitives.Slot, batchSize int) (int, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.DeleteHistoricalDataBeforeSlot")
	defer span.End()

	// Collect slot/root pairs to perform deletions in a separate read only transaction.
	slotRoots, err := s.slotRootsInRange(ctx, primitives.Slot(0), cutoffSlot, batchSize)
	if err != nil {
		return 0, err
	}

	// Return early if there's nothing to delete.
	if len(slotRoots) == 0 {
		return 0, nil
	}

	// Perform all deletions in a single transaction for atomicity
	var numSlotsDeleted int
	err = s.db.Update(func(tx *bolt.Tx) error {
		for _, sr := range slotRoots {
			// Return if context is cancelled or deadline is exceeded.
			if ctx.Err() != nil {
				//nolint:nilerr
				return nil
			}

			// Delete block
			if err = s.deleteBlock(tx, sr.root[:]); err != nil {
				return err
			}

			// Delete finalized block roots index
			if err = tx.Bucket(finalizedBlockRootsIndexBucket).Delete(sr.root[:]); err != nil {
				return errors.Wrap(err, "could not delete finalized block root index")
			}

			// Delete state
			if err = tx.Bucket(stateBucket).Delete(sr.root[:]); err != nil {
				return errors.Wrap(err, "could not delete state")
			}

			// Delete state summary
			if err = tx.Bucket(stateSummaryBucket).Delete(sr.root[:]); err != nil {
				return errors.Wrap(err, "could not delete state summary")
			}

			// Delete validator entries
			if err = s.deleteValidatorHashes(tx, sr.root[:]); err != nil {
				return errors.Wrap(err, "could not delete validators")
			}

			numSlotsDeleted++
		}

		for _, sr := range slotRoots {
			// Delete slot indices
			if err = tx.Bucket(blockSlotIndicesBucket).Delete(bytesutil.SlotToBytesBigEndian(sr.slot)); err != nil {
				return errors.Wrap(err, "could not delete block slot index")
			}
			if err = tx.Bucket(stateSlotIndicesBucket).Delete(bytesutil.SlotToBytesBigEndian(sr.slot)); err != nil {
				return errors.Wrap(err, "could not delete state slot index")
			}
		}

		// Delete all caches after we have deleted everything from buckets.
		// This is done after the buckets are deleted to avoid any issues in case of transaction rollback.
		for _, sr := range slotRoots {
			// Delete block from cache
			s.blockCache.Del(string(sr.root[:]))
			// Delete state summary from cache
			s.stateSummaryCache.delete(sr.root)
		}

		return nil
	})

	return numSlotsDeleted, err
}

// SaveBlock to the db.
func (s *Store) SaveBlock(ctx context.Context, signed interfaces.ReadOnlySignedBeaconBlock) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.SaveBlock")
	defer span.End()
	blockRoot, err := signed.Block().HashTreeRoot()
	if err != nil {
		return err
	}
	if v, ok := s.blockCache.Get(string(blockRoot[:])); v != nil && ok {
		return nil
	}
	return s.SaveBlocks(ctx, []interfaces.ReadOnlySignedBeaconBlock{signed})
}

// This function determines if we should save beacon blocks in the DB in blinded format by checking
// if a `saveBlindedBeaconBlocks` key exists in the database. Otherwise, we check if the last
// blocked stored to check if it is blinded, and then write that `saveBlindedBeaconBlocks` key
// to the DB for future checks.
func (s *Store) shouldSaveBlinded() (bool, error) {
	var saveBlinded bool
	if err := s.db.View(func(tx *bolt.Tx) error {
		metadataBkt := tx.Bucket(chainMetadataBucket)
		saveBlinded = len(metadataBkt.Get(saveBlindedBeaconBlocksKey)) > 0
		return nil
	}); err != nil {
		return false, err
	}
	return saveBlinded, nil
}

// SaveBlocks via bulk updates to the db.
func (s *Store) SaveBlocks(ctx context.Context, blks []interfaces.ReadOnlySignedBeaconBlock) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.SaveBlocks")
	defer span.End()

	robs := make([]blocks.ROBlock, len(blks))
	for i := range blks {
		rb, err := blocks.NewROBlock(blks[i])
		if err != nil {
			return errors.Wrapf(err, "failed to make an ROBlock for a block in SaveBlocks")
		}
		robs[i] = rb
	}
	return s.SaveROBlocks(ctx, robs, true)
}

type blockBatchEntry struct {
	root    []byte
	block   interfaces.ReadOnlySignedBeaconBlock
	enc     []byte
	updated bool
	indices map[string][]byte
}

func prepareBlockBatch(blks []blocks.ROBlock, shouldBlind bool) ([]blockBatchEntry, error) {
	batch := make([]blockBatchEntry, len(blks))
	for i := range blks {
		batch[i].root, batch[i].block = blks[i].RootSlice(), blks[i].ReadOnlySignedBeaconBlock
		batch[i].indices = blockIndices(batch[i].block.Block().Slot(), batch[i].block.Block().ParentRoot())
		if shouldBlind {
			blinded, err := batch[i].block.ToBlinded()
			if err != nil {
				if !errors.Is(err, blocks.ErrUnsupportedVersion) {
					return nil, errors.Wrapf(err, "could not convert block to blinded format for root %#x", batch[i].root)
				}
				// Pre-deneb blocks give ErrUnsupportedVersion; use the full block already in the batch entry.
			} else {
				batch[i].block = blinded
			}
		}
		enc, err := encodeBlock(batch[i].block)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to encode block for root %#x", batch[i].root)
		}
		batch[i].enc = enc
	}
	return batch, nil
}

func (s *Store) SaveROBlocks(ctx context.Context, blks []blocks.ROBlock, cache bool) error {
	shouldBlind, err := s.shouldSaveBlinded()
	if err != nil {
		return err
	}
	// Precompute expensive values outside the db transaction.
	batch, err := prepareBlockBatch(blks, shouldBlind)
	if err != nil {
		return errors.Wrap(err, "failed to encode all blocks in batch for saving to the db")
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blocksBucket)
		for i := range batch {
			if exists := bkt.Get(batch[i].root); exists != nil {
				continue
			}
			if err := bkt.Put(batch[i].root, batch[i].enc); err != nil {
				return errors.Wrapf(err, "could write block to db with root %#x", batch[i].root)
			}
			if err := updateValueForIndices(ctx, batch[i].indices, batch[i].root, tx); err != nil {
				return errors.Wrapf(err, "could not update DB indices for root %#x", batch[i].root)
			}
			batch[i].updated = true
		}
		return nil
	})
	if !cache {
		return err
	}
	for i := range batch {
		if batch[i].updated {
			s.blockCache.Set(string(batch[i].root), batch[i].block, int64(len(batch[i].enc)))
		}
	}
	return err
}

// blockIndices takes in a beacon block and returns
// a map of bolt DB index buckets corresponding to each particular key for indices for
// data, such as (shard indices bucket -> shard 5).
func blockIndices(slot primitives.Slot, parentRoot [32]byte) map[string][]byte {
	return map[string][]byte{
		string(blockSlotIndicesBucket):       bytesutil.SlotToBytesBigEndian(slot),
		string(blockParentRootIndicesBucket): parentRoot[:],
	}
}

// SaveHeadBlockRoot to the db.
func (s *Store) SaveHeadBlockRoot(ctx context.Context, blockRoot [32]byte) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.SaveHeadBlockRoot")
	defer span.End()
	hasStateSummary := s.HasStateSummary(ctx, blockRoot)
	return s.db.Update(func(tx *bolt.Tx) error {
		hasStateInDB := tx.Bucket(stateBucket).Get(blockRoot[:]) != nil
		if !(hasStateInDB || hasStateSummary) {
			return errors.New("no state or state summary found with head block root")
		}

		bucket := tx.Bucket(blocksBucket)
		return bucket.Put(headBlockRootKey, blockRoot[:])
	})
}

// GenesisBlock retrieves the genesis block of the beacon chain.
func (s *Store) GenesisBlock(ctx context.Context) (interfaces.ReadOnlySignedBeaconBlock, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.GenesisBlock")
	defer span.End()
	var blk interfaces.ReadOnlySignedBeaconBlock
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blocksBucket)
		root := bkt.Get(genesisBlockRootKey)
		enc := bkt.Get(root)
		if enc == nil {
			return nil
		}
		var err error
		blk, err = unmarshalBlock(ctx, enc)
		return err
	})
	return blk, err
}

func (s *Store) GenesisBlockRoot(ctx context.Context) ([32]byte, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.GenesisBlockRoot")
	defer span.End()
	var root [32]byte
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blocksBucket)
		r := bkt.Get(genesisBlockRootKey)
		if len(r) == 0 {
			return ErrNotFoundGenesisBlockRoot
		}
		root = bytesutil.ToBytes32(r)
		return nil
	})
	return root, err
}

// SaveGenesisBlockRoot to the db.
func (s *Store) SaveGenesisBlockRoot(ctx context.Context, blockRoot [32]byte) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveGenesisBlockRoot")
	defer span.End()
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(blocksBucket)
		return bucket.Put(genesisBlockRootKey, blockRoot[:])
	})
}

// SaveOriginCheckpointBlockRoot is used to keep track of the block root used for syncing from a checkpoint origin.
// This should be a finalized block from within the current weak subjectivity period.
// This value is used by a running beacon chain node to locate the state at the beginning
// of the chain history, in places where genesis would typically be used.
func (s *Store) SaveOriginCheckpointBlockRoot(ctx context.Context, blockRoot [32]byte) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveOriginCheckpointBlockRoot")
	defer span.End()
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(blocksBucket)
		return bucket.Put(originCheckpointBlockRootKey, blockRoot[:])
	})
}

// HighestRootsBelowSlot returns roots from the database slot index from the highest slot below the input slot.
// The slot value at the beginning of the return list is the slot where the roots were found. This is helpful so that
// calling code can make decisions based on the slot without resolving the blocks to discover their slot (for instance
// checking which root is canonical in fork choice, which operates purely on roots,
// then if no canonical block is found, continuing to search through lower slots).
func (s *Store) HighestRootsBelowSlot(ctx context.Context, slot primitives.Slot) (fs primitives.Slot, roots [][32]byte, err error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.HighestRootsBelowSlot")
	defer span.End()

	sk := bytesutil.Uint64ToBytesBigEndian(uint64(slot))
	err = s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blockSlotIndicesBucket)
		c := bkt.Cursor()
		// The documentation for Seek says:
		// "If the key does not exist then the next key is used. If no keys follow, a nil key is returned."
		seekPast := func(ic *bolt.Cursor, k []byte) ([]byte, []byte) {
			ik, iv := ic.Seek(k)
			// So if there are slots in the index higher than the requested slot, sl will be equal to the key that is
			// one higher than the value we want. If the slot argument is higher than the highest value in the index,
			// we'll get a nil value for `sl`. In that case we'll go backwards from Cursor.Last().
			if ik == nil {
				return ic.Last()
			}
			return ik, iv
		}
		// re loop condition: when .Prev() rewinds past the beginning off the collection, the loop will terminate,
		// because `sl` will be nil. If we don't find a value for `root` before iteration ends,
		// `root` will be the zero value, in which case this function will return the genesis block.
		for sl, r := seekPast(c, sk); sl != nil; sl, r = c.Prev() {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if r == nil {
				continue
			}
			fs = bytesutil.BytesToSlotBigEndian(sl)
			// Iterating through the index using .Prev will move from higher to lower, so the first key we find behind
			// the requested slot must be the highest block below that slot.
			if slot > fs {
				roots, err = splitRoots(r)
				if err != nil {
					return errors.Wrapf(err, "error parsing packed roots %#x", r)
				}
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return 0, nil, err
	}
	if len(roots) == 0 || (len(roots) == 1 && roots[0] == params.BeaconConfig().ZeroHash) {
		gr, err := s.GenesisBlockRoot(ctx)
		return 0, [][32]byte{gr}, err
	}

	return fs, roots, nil
}

// FeeRecipientByValidatorID returns the fee recipient for a validator id.
// `ErrNotFoundFeeRecipient` is returned if the validator id is not found.
func (s *Store) FeeRecipientByValidatorID(ctx context.Context, id primitives.ValidatorIndex) (common.Address, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.FeeRecipientByValidatorID")
	defer span.End()
	var addr []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(feeRecipientBucket)
		addr = bkt.Get(bytesutil.Uint64ToBytesBigEndian(uint64(id)))
		// IF the fee recipient is not found in the standard fee recipient bucket, then
		// check the registration bucket. The fee recipient may be there.
		// This is to resolve imcompatility until we fully migrate to the registration bucket.
		if addr == nil {
			bkt = tx.Bucket(registrationBucket)
			enc := bkt.Get(bytesutil.Uint64ToBytesBigEndian(uint64(id)))
			if enc == nil {
				return errors.Wrapf(ErrNotFoundFeeRecipient, "validator id %d", id)
			}
			reg := &ethpb.ValidatorRegistrationV1{}
			if err := decode(ctx, enc, reg); err != nil {
				return err
			}
			addr = reg.FeeRecipient
		}
		return nil
	})
	return common.BytesToAddress(addr), err
}

// SaveFeeRecipientsByValidatorIDs saves the fee recipients for validator ids.
// Error is returned if `ids` and `recipients` are not the same length.
func (s *Store) SaveFeeRecipientsByValidatorIDs(ctx context.Context, ids []primitives.ValidatorIndex, feeRecipients []common.Address) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveFeeRecipientByValidatorID")
	defer span.End()

	if len(ids) != len(feeRecipients) {
		return errors.New("validatorIDs and feeRecipients must be the same length")
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(feeRecipientBucket)
		for i, id := range ids {
			if err := bkt.Put(bytesutil.Uint64ToBytesBigEndian(uint64(id)), feeRecipients[i].Bytes()); err != nil {
				return err
			}
		}
		return nil
	})
}

// RegistrationByValidatorID returns the validator registration object for a validator id.
// `ErrNotFoundFeeRecipient` is returned if the validator id is not found.
func (s *Store) RegistrationByValidatorID(ctx context.Context, id primitives.ValidatorIndex) (*ethpb.ValidatorRegistrationV1, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.RegistrationByValidatorID")
	defer span.End()
	reg := &ethpb.ValidatorRegistrationV1{}
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(registrationBucket)
		enc := bkt.Get(bytesutil.Uint64ToBytesBigEndian(uint64(id)))
		if enc == nil {
			return errors.Wrapf(ErrNotFoundFeeRecipient, "validator id %d", id)
		}
		return decode(ctx, enc, reg)
	})
	return reg, err
}

// SaveRegistrationsByValidatorIDs saves the validator registrations for validator ids.
// Error is returned if `ids` and `registrations` are not the same length.
func (s *Store) SaveRegistrationsByValidatorIDs(ctx context.Context, ids []primitives.ValidatorIndex, regs []*ethpb.ValidatorRegistrationV1) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.SaveRegistrationsByValidatorIDs")
	defer span.End()

	if len(ids) != len(regs) {
		return errors.New("ids and registrations must be the same length")
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(registrationBucket)
		for i, id := range ids {
			enc, err := encode(ctx, regs[i])
			if err != nil {
				return err
			}
			if err := bkt.Put(bytesutil.Uint64ToBytesBigEndian(uint64(id)), enc); err != nil {
				return err
			}
		}
		return nil
	})
}

// EarliestStoredSlot returns the earliest slot in the database.
func (s *Store) EarliestSlot(ctx context.Context) (primitives.Slot, error) {
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	_, span := trace.StartSpan(ctx, "BeaconDB.EarliestSlot")
	defer span.End()

	earliestAvailableSlot := primitives.Slot(0)
	err := s.db.View(func(tx *bolt.Tx) error {
		// Retrieve the root corresponding to the earliest available block.
		c := tx.Bucket(blockSlotIndicesBucket).Cursor()
		k, v := c.First()
		if k == nil || v == nil {
			return ErrNotFound
		}
		slot := bytesutil.BytesToSlotBigEndian(k)

		// The genesis block may be indexed in this bucket, even if we started from a checkpoint.
		// Because of this, we check the next block. If the next block is still in the genesis epoch,
		// then we consider we have the whole chain.
		if slot != 0 {
			earliestAvailableSlot = slot
		}

		k, v = c.Next()
		if k == nil || v == nil {
			// Only the genesis block is available.
			return nil
		}
		slot = bytesutil.BytesToSlotBigEndian(k)
		if slot < slotsPerEpoch {
			// We are still in the genesis epoch, so we consider we have the whole chain.
			return nil
		}

		earliestAvailableSlot = slot
		return nil
	})

	return earliestAvailableSlot, err
}

type slotRoot struct {
	slot primitives.Slot
	root [32]byte
}

// slotRootsInRange returns slot and block root pairs of length min(batchSize, end-slot)
// If batchSize < 0, the limit check will be skipped entirely.
func (s *Store) slotRootsInRange(ctx context.Context, start, end primitives.Slot, batchSize int) ([]slotRoot, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.slotRootsInRange")
	defer span.End()
	if end < start {
		return nil, errInvalidSlotRange
	}

	var pairs []slotRoot
	key := bytesutil.SlotToBytesBigEndian(end)

	edge := false // used to detect whether we are at the very beginning or end of the index
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blockSlotIndicesBucket)
		c := bkt.Cursor()
		for k, v := c.Seek(key); ; /* rely on internal checks to exit */ k, v = c.Prev() {
			if len(k) == 0 && len(v) == 0 {
				// The `edge` variable and this `if` deal with 2 edge cases:
				// - Seeking past the end of the bucket (the `end` param is higher than the highest slot).
				// - Seeking before the beginning of the bucket (the `start` param is lower than the lowest slot).
				// In both of these cases k,v will be nil and we can handle the same way using `edge` to
				// - continue to the next iteration. If the following Prev() key/value is nil, Prev has gone past the beginning.
				// - Otherwise, iterate as usual.
				if edge {
					return nil
				}
				edge = true
				continue
			}
			edge = false
			slot := bytesutil.BytesToSlotBigEndian(k)
			if slot > end {
				continue // Seek will seek to the next key *after* the given one if not present
			}
			if slot < start {
				return nil
			}
			roots, err := splitRoots(v)
			if err != nil {
				return errors.Wrapf(err, "corrupt value %v in block slot index for slot=%d", v, slot)
			}
			for _, r := range roots {
				pairs = append(pairs, slotRoot{slot: slot, root: r})
			}
			if batchSize < 0 {
				continue
			}
			if len(pairs) >= batchSize {
				return nil // allows code to easily cap the number of items pruned
			}
		}
	})

	return pairs, err
}

// blockRootsByFilter retrieves the block roots given the filter criteria.
func blockRootsByFilter(ctx context.Context, tx *bolt.Tx, f *filters.QueryFilter) ([][]byte, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.blockRootsByFilter")
	defer span.End()

	// If no filter criteria are specified, return an error.
	if f == nil {
		return nil, errors.New("must specify a filter criteria for retrieving blocks")
	}

	// Creates a list of indices from the passed in filter values, such as:
	// []byte("0x2093923") in the parent root indices bucket to be used for looking up
	// block roots that were stored under each of those indices for O(1) lookup.
	indicesByBucket, err := createBlockIndicesFromFilters(ctx, f)
	if err != nil {
		return nil, errors.Wrap(err, "could not determine lookup indices")
	}

	// We retrieve block roots that match a filter criteria of slot ranges, if specified.
	filtersMap := f.Filters()
	rootsBySlotRange, err := blockRootsBySlotRange(
		ctx,
		tx.Bucket(blockSlotIndicesBucket),
		filtersMap[filters.StartSlot],
		filtersMap[filters.EndSlot],
		filtersMap[filters.StartEpoch],
		filtersMap[filters.EndEpoch],
		filtersMap[filters.SlotStep],
	)
	if err != nil {
		return nil, err
	}

	// Once we have a list of block roots that correspond to each
	// lookup index, we find the intersection across all of them and use
	// that list of roots to lookup the block. These block will
	// meet the filter criteria.
	indices := lookupValuesForIndices(ctx, indicesByBucket, tx)

	keys := rootsBySlotRange
	if len(indices) > 0 {
		// If we have found indices that meet the filter criteria, and there are also
		// block roots that meet the slot range filter criteria, we find the intersection
		// between these two sets of roots.
		if len(rootsBySlotRange) > 0 {
			joined := append([][][]byte{keys}, indices...)
			keys = slice.IntersectionByteSlices(joined...)
		} else {
			// If we have found indices that meet the filter criteria, but there are no block roots
			// that meet the slot range filter criteria, we find the intersection
			// of the regular filter indices.
			keys = slice.IntersectionByteSlices(indices...)
		}
	}

	return keys, nil
}

// blockRootsBySlotRange looks into a boltDB bucket and performs a binary search
// range scan using sorted left-padded byte keys using a start slot and an end slot.
// However, if step is one, the implemented logic won’t skip half of the slots in the range.
func blockRootsBySlotRange(
	ctx context.Context,
	bkt *bolt.Bucket,
	startSlotEncoded, endSlotEncoded, startEpochEncoded, endEpochEncoded, slotStepEncoded interface{},
) ([][]byte, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.blockRootsBySlotRange")
	defer span.End()

	// Return nothing when all slot parameters are missing
	if startSlotEncoded == nil && endSlotEncoded == nil && startEpochEncoded == nil && endEpochEncoded == nil {
		return [][]byte{}, nil
	}

	var startSlot, endSlot primitives.Slot
	var step uint64
	var ok bool
	if startSlot, ok = startSlotEncoded.(primitives.Slot); !ok {
		startSlot = 0
	}
	if endSlot, ok = endSlotEncoded.(primitives.Slot); !ok {
		endSlot = 0
	}
	if step, ok = slotStepEncoded.(uint64); !ok || step == 0 {
		step = 1
	}
	startEpoch, startEpochOk := startEpochEncoded.(primitives.Epoch)
	endEpoch, endEpochOk := endEpochEncoded.(primitives.Epoch)
	var err error
	if startEpochOk && endEpochOk {
		startSlot, err = slots.EpochStart(startEpoch)
		if err != nil {
			return nil, err
		}
		endSlot, err = slots.EpochStart(endEpoch)
		if err != nil {
			return nil, err
		}
		endSlot = endSlot + params.BeaconConfig().SlotsPerEpoch - 1
	}
	min := bytesutil.SlotToBytesBigEndian(startSlot)
	max := bytesutil.SlotToBytesBigEndian(endSlot)

	conditional := func(key, max []byte) bool {
		return key != nil && bytes.Compare(key, max) <= 0
	}
	if endSlot < startSlot {
		return nil, errInvalidSlotRange
	}
	rootsRange := endSlot.SubSlot(startSlot).Div(step)
	roots := make([][]byte, 0, rootsRange)
	c := bkt.Cursor()
	for k, v := c.Seek(min); conditional(k, max); k, v = c.Next() {
		slot := bytesutil.BytesToSlotBigEndian(k)
		if step > 1 {
			if slot.SubSlot(startSlot).Mod(step) != 0 {
				continue
			}
		}
		numOfRoots := len(v) / 32
		splitRoots := make([][]byte, 0, numOfRoots)
		for i := 0; i < len(v); i += 32 {
			splitRoots = append(splitRoots, v[i:i+32])
		}
		roots = append(roots, splitRoots...)
	}
	return roots, nil
}

// blockRootsBySlot retrieves the block roots by slot
func blockRootsBySlot(ctx context.Context, tx *bolt.Tx, slot primitives.Slot) ([][32]byte, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.blockRootsBySlot")
	defer span.End()

	bkt := tx.Bucket(blockSlotIndicesBucket)
	key := bytesutil.SlotToBytesBigEndian(slot)
	c := bkt.Cursor()
	k, v := c.Seek(key)
	if k != nil && bytes.Equal(k, key) {
		r, err := splitRoots(v)
		if err != nil {
			return nil, errors.Wrapf(err, "corrupt value in block slot index for slot=%d", slot)
		}
		return r, nil
	}
	return [][32]byte{}, nil
}

// createBlockFiltersFromIndices takes in filter criteria and returns
// a map with a single key-value pair: "block-parent-root-indices” -> parentRoot (array of bytes).
//
// For blocks, these are list of signing roots of block
// objects. If a certain filter criterion does not apply to
// blocks, an appropriate error is returned.
func createBlockIndicesFromFilters(ctx context.Context, f *filters.QueryFilter) (map[string][]byte, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.createBlockIndicesFromFilters")
	defer span.End()
	indicesByBucket := make(map[string][]byte)
	for k, v := range f.Filters() {
		switch k {
		case filters.ParentRoot:
			parentRoot, ok := v.([]byte)
			if !ok {
				return nil, errors.New("parent root is not []byte")
			}
			indicesByBucket[string(blockParentRootIndicesBucket)] = parentRoot
		// The following cases are passthroughs for blocks, as they are not used
		// for filtering indices.
		case filters.StartSlot:
		case filters.EndSlot:
		case filters.StartEpoch:
		case filters.EndEpoch:
		case filters.SlotStep:
		default:
			return nil, fmt.Errorf("filter criterion %v not supported for blocks", k)
		}
	}
	return indicesByBucket, nil
}

// unmarshal block from marshaled proto beacon block bytes to versioned beacon block struct type.
func unmarshalBlock(_ context.Context, enc []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	if len(enc) == 0 {
		return nil, errors.Wrap(ErrNotFound, "empty block bytes in db")
	}
	var err error
	enc, err = snappy.Decode(nil, enc)
	if err != nil {
		return nil, errors.Wrap(err, "could not snappy decode block")
	}
	var rawBlock ssz.Unmarshaler
	switch {
	case hasAltairKey(enc):
		// Marshal block bytes to altair beacon block.
		rawBlock = &ethpb.SignedBeaconBlockAltair{}
		if err := rawBlock.UnmarshalSSZ(enc[len(altairKey):]); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal Altair block")
		}
	case hasBellatrixKey(enc):
		rawBlock = &ethpb.SignedBeaconBlockBellatrix{}
		if err := rawBlock.UnmarshalSSZ(enc[len(bellatrixKey):]); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal Bellatrix block")
		}
	case hasBellatrixBlindKey(enc):
		rawBlock = &ethpb.SignedBlindedBeaconBlockBellatrix{}
		if err := rawBlock.UnmarshalSSZ(enc[len(bellatrixBlindKey):]); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal blinded Bellatrix block")
		}
	case hasCapellaKey(enc):
		rawBlock = &ethpb.SignedBeaconBlockCapella{}
		if err := rawBlock.UnmarshalSSZ(enc[len(capellaKey):]); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal Capella block")
		}
	case hasCapellaBlindKey(enc):
		rawBlock = &ethpb.SignedBlindedBeaconBlockCapella{}
		if err := rawBlock.UnmarshalSSZ(enc[len(capellaBlindKey):]); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal blinded Capella block")
		}
	case hasDenebKey(enc):
		rawBlock = &ethpb.SignedBeaconBlockDeneb{}
		if err := rawBlock.UnmarshalSSZ(enc[len(denebKey):]); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal Deneb block")
		}
	case hasDenebBlindKey(enc):
		rawBlock = &ethpb.SignedBlindedBeaconBlockDeneb{}
		if err := rawBlock.UnmarshalSSZ(enc[len(denebBlindKey):]); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal blinded Deneb block")
		}
	case HasElectraKey(enc):
		rawBlock = &ethpb.SignedBeaconBlockElectra{}
		if err := rawBlock.UnmarshalSSZ(enc[len(ElectraKey):]); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal Electra block")
		}
	case hasElectraBlindKey(enc):
		rawBlock = &ethpb.SignedBlindedBeaconBlockElectra{}
		if err := rawBlock.UnmarshalSSZ(enc[len(electraBlindKey):]); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal blinded Electra block")
		}
	case hasFuluKey(enc):
		rawBlock = &ethpb.SignedBeaconBlockFulu{}
		if err := rawBlock.UnmarshalSSZ(enc[len(fuluKey):]); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal Fulu block")
		}
	case hasFuluBlindKey(enc):
		rawBlock = &ethpb.SignedBlindedBeaconBlockFulu{}
		if err := rawBlock.UnmarshalSSZ(enc[len(fuluBlindKey):]); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal blinded Fulu block")
		}
	default:
		// Marshal block bytes to phase 0 beacon block.
		rawBlock = &ethpb.SignedBeaconBlock{}
		if err := rawBlock.UnmarshalSSZ(enc); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal Phase0 block")
		}
	}
	return blocks.NewSignedBeaconBlock(rawBlock)
}

func encodeBlock(blk interfaces.ReadOnlySignedBeaconBlock) ([]byte, error) {
	key, err := keyForBlock(blk)
	if err != nil {
		return nil, errors.Wrap(err, "could not determine version encoding key for block")
	}
	enc, err := blk.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	dbfmt := make([]byte, len(key)+len(enc))
	if len(key) > 0 {
		copy(dbfmt, key)
	}
	copy(dbfmt[len(key):], enc)
	return snappy.Encode(nil, dbfmt), nil
}

func keyForBlock(blk interfaces.ReadOnlySignedBeaconBlock) ([]byte, error) {
	v := blk.Version()

	if v >= version.Fulu {
		if blk.IsBlinded() {
			return fuluBlindKey, nil
		}
		return fuluKey, nil
	}

	if v >= version.Electra {
		if blk.IsBlinded() {
			return electraBlindKey, nil
		}
		return ElectraKey, nil
	}

	if v >= version.Deneb {
		if blk.IsBlinded() {
			return denebBlindKey, nil
		}
		return denebKey, nil
	}

	if v >= version.Capella {
		if blk.IsBlinded() {
			return capellaBlindKey, nil
		}
		return capellaKey, nil
	}

	if v >= version.Bellatrix {
		if blk.IsBlinded() {
			return bellatrixBlindKey, nil
		}
		return bellatrixKey, nil
	}

	if v >= version.Altair {
		return altairKey, nil
	}

	if v >= version.Phase0 {
		return nil, nil
	}

	return nil, fmt.Errorf("unsupported block version: %v", blk.Version())
}

func (s *Store) deleteBlock(tx *bolt.Tx, root []byte) error {
	if err := tx.Bucket(blocksBucket).Delete(root); err != nil {
		return errors.Wrap(err, "could not delete block")
	}

	if err := tx.Bucket(blockParentRootIndicesBucket).Delete(root); err != nil {
		return errors.Wrap(err, "could not delete block parent indices")
	}

	return nil
}

func (s *Store) deleteMatchingParentIndex(tx *bolt.Tx, parent, child [32]byte) error {
	bkt := tx.Bucket(blockParentRootIndicesBucket)
	if err := deleteRootIndexEntry(bkt, parent[:], child); err != nil {
		return errors.Wrap(err, "could not delete parent root index entry")
	}
	return nil
}

func (s *Store) deleteSlotIndexEntry(tx *bolt.Tx, slot primitives.Slot, root [32]byte) error {
	key := bytesutil.SlotToBytesBigEndian(slot)
	bkt := tx.Bucket(blockSlotIndicesBucket)
	if err := deleteRootIndexEntry(bkt, key, root); err != nil {
		return errors.Wrap(err, "could not delete slot index entry")
	}
	return nil
}

func deleteRootIndexEntry(bkt *bolt.Bucket, key []byte, root [32]byte) error {
	packed := bkt.Get(key)
	if len(packed) == 0 {
		return nil
	}
	updated, err := removeRoot(packed, root)
	if err != nil {
		return err
	}
	// Don't update the value if the root was not found.
	if bytes.Equal(updated, packed) {
		return nil
	}
	// If there are no other roots in the key, just delete it.
	if len(updated) == 0 {
		if err := bkt.Delete(key); err != nil {
			return err
		}
		return nil
	}
	// Update the key with the root removed.
	return bkt.Put(key, updated)
}

func (s *Store) deleteValidatorHashes(tx *bolt.Tx, root []byte) error {
	ok, err := s.isStateValidatorMigrationOver()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	// Delete the validator hash index
	if err = tx.Bucket(blockRootValidatorHashesBucket).Delete(root); err != nil {
		return errors.Wrap(err, "could not delete validator index")
	}

	return nil
}
