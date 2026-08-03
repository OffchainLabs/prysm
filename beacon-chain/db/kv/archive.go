package kv

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

// archiveStatusLen is the encoded size of an ArchiveStatus: two slots, a state root and the complete flag.
const archiveStatusLen = 8 + 8 + 32 + 1

// ArchiveStatus tracks the progress of an archive node's historical state regeneration. There is only one
// ArchiveStatus value in the database. It is stored next to the state-diff offset because it describes the
// same tree: OriginSlot is the tree offset, and RegeneratedThroughSlot is the highest tree boundary that has
// been written by the forward walk.
type ArchiveStatus struct {
	OriginSlot             primitives.Slot
	RegeneratedThroughSlot primitives.Slot
	OriginStateRoot        [32]byte
	// Complete is set once the walk has caught up with the live finalized checkpoint and normal cold-state
	// migration has taken over.
	Complete bool
}

func (a *ArchiveStatus) encode() []byte {
	enc := make([]byte, archiveStatusLen)
	binary.LittleEndian.PutUint64(enc[0:8], uint64(a.OriginSlot))
	binary.LittleEndian.PutUint64(enc[8:16], uint64(a.RegeneratedThroughSlot))
	copy(enc[16:48], a.OriginStateRoot[:])
	if a.Complete {
		enc[48] = 1
	}
	return enc
}

func decodeArchiveStatus(enc []byte) (*ArchiveStatus, error) {
	if len(enc) != archiveStatusLen {
		return nil, fmt.Errorf("archive status has invalid length %d, expected %d", len(enc), archiveStatusLen)
	}
	a := &ArchiveStatus{
		OriginSlot:             primitives.Slot(binary.LittleEndian.Uint64(enc[0:8])),
		RegeneratedThroughSlot: primitives.Slot(binary.LittleEndian.Uint64(enc[8:16])),
		Complete:               enc[48] == 1,
	}
	copy(a.OriginStateRoot[:], enc[16:48])
	return a, nil
}

// SaveArchiveStatus writes the archive status to the db and updates the copy the store keeps in memory.
func (s *Store) SaveArchiveStatus(ctx context.Context, as *ArchiveStatus) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveArchiveStatus")
	defer span.End()
	if as == nil {
		return errors.New("nil archive status")
	}
	enc := as.encode()
	if err := s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		return bucket.Put(archiveStatusKey, enc)
	}); err != nil {
		return err
	}
	s.setArchiveStatus(as)
	return nil
}

// ArchiveStatus retrieves the archive status, or a wrapped ErrNotFound if this is not an archive node.
func (s *Store) ArchiveStatus(ctx context.Context) (*ArchiveStatus, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.ArchiveStatus")
	defer span.End()
	var enc []byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		raw := bucket.Get(archiveStatusKey)
		if len(raw) == 0 {
			return errors.Wrap(ErrNotFound, "ArchiveStatus not found")
		}
		enc = append(enc, raw...)
		return nil
	}); err != nil {
		return nil, err
	}
	return decodeArchiveStatus(enc)
}

func (s *Store) setArchiveStatus(as *ArchiveStatus) {
	s.archiveLock.Lock()
	defer s.archiveLock.Unlock()
	cp := *as
	s.archiveStatus = &cp
}

// archivePending reports whether an archive walk is in progress, along with the highest boundary slot it has
// written. While pending, the live chain must not write into the diff tree: the tree levels above the walk's
// frontier have no anchors yet, and a stray write becomes the level maximum that startStateDiff validates
// against on the next boot.
func (s *Store) archivePending() (primitives.Slot, bool) {
	s.archiveLock.RLock()
	defer s.archiveLock.RUnlock()
	if s.archiveStatus == nil || s.archiveStatus.Complete {
		return 0, false
	}
	return s.archiveStatus.RegeneratedThroughSlot, true
}

// InitializeArchiveOrigin anchors the state-diff tree at the given archive origin state and records the
// resulting ArchiveStatus. It must run before any other caller can set the offset, and refuses to move the
// offset of a tree that already has one.
func (s *Store) InitializeArchiveOrigin(ctx context.Context, st state.BeaconState) error {
	existing, err := s.ArchiveStatus(ctx)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	root, err := st.HashTreeRoot(ctx)
	if err != nil {
		return errors.Wrap(err, "could not compute archive origin state root")
	}
	if existing != nil {
		if existing.OriginSlot != st.Slot() || existing.OriginStateRoot != root {
			return fmt.Errorf(
				"archive origin state changed: database was initialized at slot %d (root %#x), got slot %d (root %#x). "+
					"Use the original state or delete the database and re-sync",
				existing.OriginSlot, existing.OriginStateRoot, st.Slot(), root)
		}
		return nil
	}

	hasOffset, err := s.hasStateDiffOffset()
	if err != nil {
		return err
	}
	if hasOffset {
		offset, err := s.loadOffset()
		if err != nil {
			return err
		}
		if offset != uint64(st.Slot()) {
			return fmt.Errorf(
				"state-diff tree is already anchored at slot %d, cannot re-anchor an archive at slot %d; "+
					"delete the database and re-sync", offset, st.Slot())
		}
	}

	if err := s.initializeStateDiff(st.Slot(), st); err != nil {
		return errors.Wrap(err, "could not anchor state-diff tree at the archive origin")
	}
	return s.SaveArchiveStatus(ctx, &ArchiveStatus{
		OriginSlot:             st.Slot(),
		RegeneratedThroughSlot: st.Slot(),
		OriginStateRoot:        root,
	})
}
