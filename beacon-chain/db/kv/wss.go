package kv

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz/detect"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
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

	if err := verifyOriginBlockRoot(ctx, state, blockRoot); err != nil {
		return err
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

	if features.Get().EnableStateDiff {
		// initializeStateDiff will save the state, so we don't need to call SaveState here
		if err := s.initializeStateDiff(state.Slot(), state); err != nil {
			return errors.Wrap(err, "failed to initialize state diff")
		}
	} else {
		// save state
		if err = s.SaveState(ctx, state, blockRoot); err != nil {
			return errors.Wrap(err, "save state")
		}
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

	// The origin is treated as finalized at the epoch of its state, not of its block.
	slotEpoch, err := state.Slot().SafeDivSlot(params.BeaconConfig().SlotsPerEpoch)
	if err != nil {
		return err
	}
	originEpoch := primitives.Epoch(slotEpoch)

	if state.Slot()%params.BeaconConfig().SlotsPerEpoch != 0 {
		log.WithFields(logrus.Fields{
			"slot":  state.Slot(),
			"epoch": originEpoch,
		}).Warn("Origin state is not at an epoch boundary")
	}

	logOriginFinality(state, originEpoch)

	// The justified epoch stays truthful so imported blocks match forkchoice's voting source.
	justifiedEpoch := originEpoch
	if jc := state.CurrentJustifiedCheckpoint(); jc != nil && jc.Epoch < originEpoch {
		justifiedEpoch = jc.Epoch
	}

	if err = s.SaveJustifiedCheckpoint(ctx, &ethpb.Checkpoint{Epoch: justifiedEpoch, Root: blockRoot[:]}); err != nil {
		return errors.Wrap(err, "save justified checkpoint")
	}

	if err = s.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{Epoch: originEpoch, Root: blockRoot[:]}); err != nil {
		return errors.Wrap(err, "save finalized checkpoint")
	}
	return nil
}

func verifyOriginBlockRoot(ctx context.Context, st state.BeaconState, blockRoot [32]byte) error {
	header := st.LatestBlockHeader()
	if header == nil {
		return errors.New("origin state has no latest block header")
	}
	if bytesutil.ToBytes32(header.StateRoot) == params.BeaconConfig().ZeroHash {
		stateRoot, err := st.HashTreeRoot(ctx)
		if err != nil {
			return errors.Wrap(err, "could not compute HashTreeRoot of origin state")
		}
		header.StateRoot = stateRoot[:]
	}
	headerRoot, err := header.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not compute HashTreeRoot of origin state latest block header")
	}
	if headerRoot != blockRoot {
		return errors.Wrapf(errOriginBlockMismatch, "state latest block header root = %#x, block root = %#x", headerRoot, blockRoot)
	}
	return nil
}

func logOriginFinality(st state.BeaconState, originEpoch primitives.Epoch) {
	fc := st.FinalizedCheckpoint()
	if fc == nil || fc.Epoch >= originEpoch {
		return
	}
	log.WithFields(logrus.Fields{
		"originEpoch":    originEpoch,
		"finalizedEpoch": fc.Epoch,
	}).Warn("Syncing from an unfinalized checkpoint. If this block is reorged out the node cannot recover and the database must be deleted")
}
