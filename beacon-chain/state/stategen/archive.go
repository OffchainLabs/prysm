package stategen

import (
	"context"
	"fmt"
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// SetArchivePending marks whether an archive node is still regenerating historical states.
func (s *State) SetArchivePending(pending bool) {
	s.archive.lock.Lock()
	defer s.archive.lock.Unlock()
	s.archive.pending = pending
}

// ArchivePending reports whether archive regeneration is still in progress.
func (s *State) ArchivePending() bool {
	s.archive.lock.RLock()
	defer s.archive.lock.RUnlock()
	return s.archive.pending
}

// CompleteArchiveRegeneration hands migration back to the finalization-driven path, reporting if it happened.
func (s *State) CompleteArchiveRegeneration(
	ctx context.Context,
	nextUnwrittenBoundary primitives.Slot,
	markComplete func(context.Context) error,
) (bool, error) {
	s.migrationLock.Lock()
	defer s.migrationLock.Unlock()

	cp, err := s.beaconDB.FinalizedCheckpoint(ctx)
	if err != nil {
		return false, errors.Wrap(err, "could not read the finalized checkpoint")
	}
	cpSlot, err := slots.EpochStart(cp.Epoch)
	if err != nil {
		return false, errors.Wrap(err, "could not compute the finalized checkpoint slot")
	}
	if nextUnwrittenBoundary <= cpSlot {
		return false, nil
	}

	fRoot := bytesutil.ToBytes32(cp.Root)
	fState, err := s.StateByRoot(ctx, fRoot)
	if err != nil {
		return false, errors.Wrapf(err, "could not load the finalized state at root %#x", fRoot)
	}
	s.SaveFinalizedState(fRoot, fState)

	if err := markComplete(ctx); err != nil {
		return false, errors.Wrap(err, "could not record archive regeneration as complete")
	}

	s.SetArchivePending(false)

	if store, ok := s.beaconDB.(archiveResumeSnapshotStore); ok {
		roots, err := store.ArchiveResumeSnapshotRoots(ctx)
		if err == nil {
			err = store.DeleteArchiveResumeSnapshots(ctx, roots)
		}
		if err != nil {
			log.WithError(err).Warn("Could not delete archive resume snapshots")
		}
	}

	log.WithFields(logrus.Fields{
		"nextUnwrittenBoundary": nextUnwrittenBoundary,
		"finalizedSlot":         fState.Slot(),
		"finalizedEpoch":        cp.Epoch,
	}).Info("Archive regeneration complete; resuming cold state migration")
	return true, nil
}

// saveArchiveResumeSnapshot persists a full state by root at a coarse interval, keeping only the newest.
func (s *State) saveArchiveResumeSnapshot(ctx context.Context, blockRoot [32]byte, st state.BeaconState) error {
	if st.Slot()%archiveResumeSnapshotInterval != 0 {
		return nil
	}
	store, ok := s.beaconDB.(archiveResumeSnapshotStore)
	if !ok {
		return nil
	}
	if err := store.SaveArchiveResumeSnapshot(ctx, st, blockRoot); err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		"slot": st.Slot(),
		"root": fmt.Sprintf("%#x", blockRoot),
	}).Info("Saved archive restart snapshot")

	roots, err := store.ArchiveResumeSnapshotRoots(ctx)
	if err == nil {
		err = store.DeleteArchiveResumeSnapshots(ctx, slices.DeleteFunc(roots, func(r [32]byte) bool {
			return r == blockRoot
		}))
	}
	if err != nil {
		log.WithError(err).Warn("Could not delete superseded archive resume snapshots")
	}
	return nil
}

type archiveResumeSnapshotStore interface {
	SaveArchiveResumeSnapshot(ctx context.Context, st state.ReadOnlyBeaconState, blockRoot [32]byte) error
	ArchiveResumeSnapshotRoots(ctx context.Context) ([][32]byte, error)
	DeleteArchiveResumeSnapshots(ctx context.Context, blockRoots [][32]byte) error
}
