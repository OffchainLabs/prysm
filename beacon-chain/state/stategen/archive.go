package stategen

import (
	"context"
	"fmt"

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

	s.archive.lock.Lock()
	s.archive.pending = false
	roots := s.archive.resumeSnapshotRoots
	s.archive.resumeSnapshotRoots = nil
	s.archive.lock.Unlock()

	if len(roots) > 0 {
		if deleter, ok := s.beaconDB.(hotStateSnapshotDeleter); ok {
			if err := deleter.DeleteHotStateSnapshots(ctx, roots); err != nil {
				log.WithError(err).Warn("Could not delete archive resume snapshots")
			}
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
	saver, ok := s.beaconDB.(hotStateSnapshotSaver)
	if !ok {
		return nil
	}
	if err := saver.SaveHotStateSnapshot(ctx, st, blockRoot); err != nil {
		return err
	}

	s.archive.lock.Lock()
	stale := s.archive.resumeSnapshotRoots
	s.archive.resumeSnapshotRoots = [][32]byte{blockRoot}
	s.archive.lock.Unlock()

	log.WithFields(logrus.Fields{
		"slot": st.Slot(),
		"root": fmt.Sprintf("%#x", blockRoot),
	}).Info("Saved archive restart snapshot")

	if len(stale) > 0 {
		if deleter, ok := s.beaconDB.(hotStateSnapshotDeleter); ok {
			if err := deleter.DeleteHotStateSnapshots(ctx, stale); err != nil {
				log.WithError(err).Warn("Could not delete superseded archive resume snapshot")
			}
		}
	}
	return nil
}

type hotStateSnapshotDeleter interface {
	DeleteHotStateSnapshots(ctx context.Context, blockRoots [][32]byte) error
}
