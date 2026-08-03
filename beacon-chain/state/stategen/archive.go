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

// SetArchivePending marks whether an archive node is still regenerating historical states. While pending,
// cold-state migration is suppressed and full states are periodically snapshotted by root so restarts do not
// have to replay from the sync origin.
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

// CompleteArchiveRegeneration hands cold-state migration back to the normal finalization-driven path. It
// reports whether the handoff happened; the caller keeps walking when it did not.
//
// nextUnwrittenBoundary is the lowest tree boundary the walk has not written yet. The handoff only completes
// once that is above the finalized checkpoint slot, meaning every anchor a live write below finalization
// could resolve to already exists. It is deliberately not a comparison against the walk's frontier: boundary
// spacing need not divide the epoch, so the frontier can never equal the checkpoint slot exactly.
func (s *State) CompleteArchiveRegeneration(ctx context.Context, nextUnwrittenBoundary primitives.Slot) (bool, error) {
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
	s.SaveFinalizedState(fState.Slot(), fRoot, fState)

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

// saveArchiveResumeSnapshot persists a full state by root at a coarse interval so that a restart during
// regeneration has a nearby replay base. It keeps only the newest snapshot; the bucket-wide
// ClearHotStateSnapshots cannot be used here because it would also drop the checkpoint origin state.
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
