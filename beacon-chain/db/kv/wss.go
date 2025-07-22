package kv

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/detect"
	"github.com/OffchainLabs/prysm/v6/proto/dbval"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// SaveOrigin loads an ssz serialized Block & BeaconState from an io.Reader
// (ex: an open file) prepares the database so that the beacon node can begin
// syncing, using the provided values as their point of origin. This is an alternative
// to syncing from genesis, and should only be run on an empty database.
func (s *Store) SaveOrigin(ctx context.Context, serState, serBlock []byte) error {
	cf, err := detect.FromState(serState)
	if err != nil {
		return errors.Wrap(err, "could not sniff config+fork for origin state bytes")
	}
	_, ok := params.BeaconConfig().ForkVersionSchedule[cf.Version]
	if !ok {
		return fmt.Errorf("config mismatch, beacon node configured to connect to %s, detected state is for %s", params.BeaconConfig().ConfigName, cf.Config.ConfigName)
	}

	log.WithFields(logrus.Fields{
		"configName": cf.Config.ConfigName,
		"forkName":   version.String(cf.Fork),
	}).Info("Detected supported config for state & block version")

	state, err := cf.UnmarshalBeaconState(serState)
	if err != nil {
		return errors.Wrap(err, "failed to initialize origin state w/ bytes + config+fork")
	}

	wblk, err := cf.UnmarshalBeaconBlock(serBlock)
	if err != nil {
		return errors.Wrap(err, "failed to initialize origin block w/ bytes + config+fork")
	}
	blk := wblk.Block()
	slot := blk.Slot()

	blockRoot, err := blk.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not compute HashTreeRoot of checkpoint block")
	}

	pr := blk.ParentRoot()
	bf := &dbval.BackfillStatus{
		LowSlot:       uint64(slot),
		LowRoot:       blockRoot[:],
		LowParentRoot: pr[:],
		OriginRoot:    blockRoot[:],
		OriginSlot:    uint64(slot),
	}

	if err = s.SaveBackfillStatus(ctx, bf); err != nil {
		return errors.Wrap(err, "unable to save backfill status data to db for checkpoint sync")
	}

	log.WithField("root", fmt.Sprintf("%#x", blockRoot)).Info("Saving checkpoint data into database")
	if err := s.SaveBlock(ctx, wblk); err != nil {
		return errors.Wrap(err, "save block")
	}

	// save state
	if err = s.SaveState(ctx, state, blockRoot); err != nil {
		return errors.Wrap(err, "save state")
	}

	if err = s.SaveStateSummary(ctx, &ethpb.StateSummary{
		Slot: state.Slot(),
		Root: blockRoot[:],
	}); err != nil {
		return errors.Wrap(err, "save state summary")
	}

	// mark block as head of chain, so that processing will pick up from this point
	if err = s.SaveHeadBlockRoot(ctx, blockRoot); err != nil {
		return errors.Wrap(err, "save head block root")
	}

	// save origin block root in a special key, to be used when the canonical
	// origin (start of chain, ie alternative to genesis) block or state is needed
	if err = s.SaveOriginCheckpointBlockRoot(ctx, blockRoot); err != nil {
		return errors.Wrap(err, "save origin checkpoint block root")
	}

	// rebuild the checkpoint from the block
	// use it to mark the block as justified and finalized
	slotEpoch, err := wblk.Block().Slot().SafeDivSlot(params.BeaconConfig().SlotsPerEpoch)
	if err != nil {
		return err
	}

	chkpt := &ethpb.Checkpoint{
		Epoch: primitives.Epoch(slotEpoch),
		Root:  blockRoot[:],
	}

	if err = s.SaveJustifiedCheckpoint(ctx, chkpt); err != nil {
		return errors.Wrap(err, "save justified checkpoint")
	}

	if err = s.SaveFinalizedCheckpoint(ctx, chkpt); err != nil {
		return errors.Wrap(err, "save finalized checkpoint")
	}

	return nil
}
