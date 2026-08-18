package kv

import (
	"context"
	"encoding/binary"
	"errors"
	"strconv"
	"sync"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/golang/snappy"
	pkgerrors "github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

// anchorMemoSize is how many deserialized anchors are kept alongside the compressed ones. Anchors are stored
// compressed precisely to bound memory, so memoizing every level would defeat that: with the default
// exponents it would pin six full states.
//
// Two is enough to capture the reuse that matters. Writes advance in slot order, so the deepest level's
// anchor serves every boundary inside its span (15 of every 16 with the default exponents), and the one
// request that interrupts the run is the deepest level's own write asking for the level above it. Holding
// those two means no misses in steady state; anything shallower is requested rarely enough that
// re-deserializing it is cheaper than keeping it resident.
const anchorMemoSize = 2

// anchorMemoEntry is a deserialized anchor. A nil state marks an unused slot.
type anchorMemoEntry struct {
	level int
	state state.ReadOnlyBeaconState
}

type stateDiffCache struct {
	sync.RWMutex
	anchors        [][]byte
	levelsWithData []bool
	offset         uint64
	// memo is an LRU of already deserialized anchors. getAnchor is called once per diff written and an
	// anchor is reused for every slot in its span, so without it a long forward walk pays a full state
	// deserialization per boundary. Entries are dropped whenever the underlying bytes change.
	memo [anchorMemoSize]anchorMemoEntry
}

// memoGet returns the memoized anchor for the level, promoting it to most recently used. Callers must hold
// the write lock.
func (c *stateDiffCache) memoGet(level int) state.ReadOnlyBeaconState {
	for i, e := range c.memo {
		if e.state == nil || e.level != level {
			continue
		}
		if i > 0 {
			copy(c.memo[1:i+1], c.memo[:i])
			c.memo[0] = e
		}
		return e.state
	}
	return nil
}

// memoPut inserts an anchor as most recently used, evicting the least recently used entry. Callers must hold
// the write lock.
func (c *stateDiffCache) memoPut(level int, st state.ReadOnlyBeaconState) {
	copy(c.memo[1:], c.memo[:len(c.memo)-1])
	c.memo[0] = anchorMemoEntry{level: level, state: st}
}

// memoDrop invalidates the memoized anchor for the level. Callers must hold the write lock.
func (c *stateDiffCache) memoDrop(level int) {
	for i, e := range c.memo {
		if e.state != nil && e.level == level {
			c.memo[i] = anchorMemoEntry{}
		}
	}
}

func populateStateDiffCacheFromDB(s *Store, offset uint64) (*stateDiffCache, error) {
	cache := &stateDiffCache{
		anchors:        make([][]byte, len(flags.Get().StateDiffExponents)-1),
		levelsWithData: make([]bool, len(flags.Get().StateDiffExponents)),
		offset:         offset,
	}

	if err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		for level := range cache.levelsWithData {
			if level == 0 {
				if bucket.Get(makeKeyForStateDiffTree(0, offset)) != nil {
					cache.levelsWithData[level] = true
				}
				continue
			}
			cursor := bucket.Cursor()
			prefix := []byte{byte(level)}
			key, _ := cursor.Seek(prefix)
			if key != nil && key[0] == byte(level) {
				slot, ok := slotFromStateDiffKey(key)
				if !ok {
					return ErrStateDiffCorrupted
				}
				if slot < offset {
					return ErrStateDiffCorrupted
				}
				if computeLevel(offset, primitives.Slot(slot)) != level {
					return ErrStateDiffCorrupted
				}
				if !hasCompleteDiffAtLevelSlot(bucket, level, slot) {
					return ErrStateDiffCorrupted
				}
				cache.levelsWithData[level] = true
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// The offset snapshot must always exist; its absence means the tree is unreadable.
	if _, err := s.getFullSnapshot(offset); err != nil {
		if errors.Is(err, errSnapshotNotFound) {
			return nil, pkgerrors.Wrapf(ErrStateDiffMissingSnapshot, "offset snapshot at slot %d", offset)
		}
		return nil, pkgerrors.Wrapf(ErrStateDiffCorrupted, "failed to load offset snapshot at slot %d: %v", offset, err)
	}
	// Only cache anchor if there are higher levels that need it.
	// With a single exponent, len(anchors)==0 and no caching is needed.
	if len(cache.anchors) > 0 {
		// Cache the newest level-0 snapshot, not the one at the offset: once the tree spans more than
		// 2^exponents[0] slots the offset snapshot is no longer the anchor any live write resolves to.
		anchorSlot := latestLevelZeroSlot(s, offset)
		anchor0, err := s.getFullSnapshot(anchorSlot)
		if err != nil {
			return nil, pkgerrors.Wrapf(ErrStateDiffCorrupted, "failed to load level 0 snapshot at slot %d: %v", anchorSlot, err)
		}
		if err := cache.setAnchor(0, anchor0); err != nil {
			return nil, err
		}
	}
	cache.levelsWithData[0] = true

	return cache, nil
}

// latestLevelZeroSlot returns the highest slot holding a full snapshot that is a valid level 0 boundary for
// the given offset, falling back to the offset itself.
func latestLevelZeroSlot(s *Store, offset uint64) uint64 {
	maxSlot, err := latestSlotForLevel(s, 0)
	if err != nil {
		return offset
	}
	if maxSlot <= offset || computeLevel(offset, primitives.Slot(maxSlot)) != 0 {
		return offset
	}
	return maxSlot
}

func validateStateDiffCache(ctx context.Context, s *Store, cache *stateDiffCache) error {
	// Copy level flags under lock, then release before validation work.
	// stateByDiff may consult cache metadata and should never be called while holding cache locks.
	cache.RLock()
	levels := make([]bool, len(cache.levelsWithData))
	copy(levels, cache.levelsWithData)
	cache.RUnlock()

	for level, hasData := range levels {
		if !hasData || level == 0 {
			continue
		}
		maxSlot, err := latestSlotForLevel(s, level)
		if err != nil {
			return err
		}
		if _, err := s.stateByDiff(ctx, primitives.Slot(maxSlot)); err != nil {
			return pkgerrors.Wrapf(ErrStateDiffCorrupted, "state diff validation failed for level %d slot %d: %v", level, maxSlot, err)
		}
	}
	return nil
}

func latestSlotForLevel(s *Store, level int) (uint64, error) {
	var maxSlot uint64
	found := false
	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		cursor := bucket.Cursor()
		prefix := []byte{byte(level)}
		for key, _ := cursor.Seek(prefix); key != nil && key[0] == byte(level); key, _ = cursor.Next() {
			slot, ok := slotFromStateDiffKey(key)
			if !ok {
				return ErrStateDiffCorrupted
			}
			if !found || slot > maxSlot {
				maxSlot = slot
				found = true
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, ErrStateDiffCorrupted
	}
	return maxSlot, nil
}

func slotFromStateDiffKey(key []byte) (uint64, bool) {
	if len(key) < 9 {
		return 0, false
	}
	return binary.LittleEndian.Uint64(key[1:9]), true
}

func hasCompleteDiffAtLevelSlot(bucket *bbolt.Bucket, level int, slot uint64) bool {
	key := makeKeyForStateDiffTree(level, slot)
	stateKey := append(append([]byte{}, key...), stateSuffix...)
	validatorKey := append(append([]byte{}, key...), validatorSuffix...)
	balancesKey := append(append([]byte{}, key...), balancesSuffix...)
	return bucket.Get(stateKey) != nil && bucket.Get(validatorKey) != nil && bucket.Get(balancesKey) != nil
}

func newStateDiffCache(s *Store) (*stateDiffCache, error) {
	var offset uint64

	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}

		offsetBytes := bucket.Get(offsetKey)
		if offsetBytes == nil {
			return errors.New("state diff cache: offset not found")
		}
		offset = binary.LittleEndian.Uint64(offsetBytes)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &stateDiffCache{
		anchors:        make([][]byte, len(flags.Get().StateDiffExponents)-1), // -1 because last level doesn't need to be cached
		levelsWithData: make([]bool, len(flags.Get().StateDiffExponents)),
		offset:         offset,
	}, nil
}

// getAnchor returns the anchor state for the given level. The result must be treated as read-only: it is
// shared with every caller until the anchor is replaced. It is only ever consumed by hdiff.Diff, which does
// not mutate its inputs.
func (c *stateDiffCache) getAnchor(level int) state.ReadOnlyBeaconState {
	c.Lock()
	defer c.Unlock()

	if level < 0 || level >= len(c.anchors) {
		return nil
	}
	if st := c.memoGet(level); st != nil {
		return st
	}

	compressed := c.anchors[level]

	if len(compressed) == 0 {
		return nil
	}

	uncompressed, err := snappy.Decode(nil, compressed)
	if err != nil {
		return nil
	}

	st, err := decodeStateSnapshot(uncompressed)
	if err != nil {
		return nil
	}

	c.memoPut(level, st)
	return st
}

func (c *stateDiffCache) setAnchor(level int, anchor state.ReadOnlyBeaconState) error {
	c.Lock()
	defer c.Unlock()
	if level >= len(c.anchors) || level < 0 {
		return errors.New("state diff cache: anchor level out of range")
	}
	if anchor == nil {
		return errors.New("state diff cache: anchor cannot be nil")
	}

	anchorSSZ, err := anchor.MarshalSSZ()
	if err != nil {
		return err
	}
	versionedAnchorBytes, err := addKey(anchor.Version(), anchorSSZ)
	if err != nil {
		return err
	}
	compressed := snappy.Encode(nil, versionedAnchorBytes)

	c.anchors[level] = compressed
	c.memoDrop(level)
	stateDiffAnchorCacheBytes.WithLabelValues(strconv.Itoa(level)).Set(float64(len(compressed)))
	return nil
}

func (c *stateDiffCache) levelHasData(level int) bool {
	c.RLock()
	defer c.RUnlock()
	if level < 0 || level >= len(c.levelsWithData) {
		return false
	}
	return c.levelsWithData[level]
}

func (c *stateDiffCache) setLevelHasData(level int) error {
	c.Lock()
	defer c.Unlock()
	if level < 0 || level >= len(c.levelsWithData) {
		return errors.New("state diff cache: level data index out of range")
	}
	c.levelsWithData[level] = true
	return nil
}

func (c *stateDiffCache) getOffset() uint64 {
	c.RLock()
	defer c.RUnlock()
	return c.offset
}

func (c *stateDiffCache) setOffset(offset uint64) {
	c.Lock()
	defer c.Unlock()
	c.offset = offset
}

func (c *stateDiffCache) clearAnchors() {
	c.Lock()
	defer c.Unlock()
	c.anchors = make([][]byte, len(flags.Get().StateDiffExponents)-1) // -1 because last level doesn't need to be cached
	c.memo = [anchorMemoSize]anchorMemoEntry{}
	for level := range len(c.anchors) {
		stateDiffAnchorCacheBytes.WithLabelValues(strconv.Itoa(level)).Set(0)
	}
}
