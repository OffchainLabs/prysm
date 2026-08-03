package node

import (
	"bytes"
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/archive"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync/backfill"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz/detect"
	"github.com/OffchainLabs/prysm/v7/genesis"
	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// stateDiffTreeEpochMultiple is the slot alignment the state-diff tree requires of its offset.
const stateDiffTreeEpochMultiple = 32

type archiveOriginInitializer interface {
	InitializeArchiveOrigin(ctx context.Context, st state.BeaconState) error
	ArchiveStatus(ctx context.Context) (*kv.ArchiveStatus, error)
}

// initArchiveOrigin anchors the state-diff tree at the archive origin state. It must run before genesis or
// checkpoint data is written to the database, because whichever caller sets the tree offset first wins and
// everything below that offset is unrepresentable.
//
// It has to run after genesis.Initialize and the fork schedule are set up, hence its position at the top of
// startBaseServices rather than right after the database is opened.
func (b *BeaconNode) initArchiveOrigin(cliCtx *cli.Context) error {
	if !features.Get().EnableArchive {
		return nil
	}
	// openDB downgrades to the legacy state layout when an existing database predates state-diff. Archive
	// mode is meaningless without the tree, so refuse to run in that configuration.
	if !features.Get().EnableStateDiff {
		return errors.New("--enable-archive requires the state-diff database layout, but it was disabled " +
			"because this database was created without it; use a fresh data directory")
	}
	if cliCtx.Bool(flags.BeaconDBPruning.Name) {
		return fmt.Errorf("--enable-archive cannot be combined with --%s: pruning deletes the history the "+
			"archive is built from", flags.BeaconDBPruning.Name)
	}
	initializer, ok := b.db.(archiveOriginInitializer)
	if !ok {
		return errors.New("database does not support archive mode")
	}

	st, err := archiveOriginState(cliCtx)
	if err != nil {
		return err
	}
	if err := validateArchiveOrigin(st); err != nil {
		return err
	}
	if err := initializer.InitializeArchiveOrigin(b.ctx, st); err != nil {
		return err
	}
	as, err := initializer.ArchiveStatus(b.ctx)
	if err != nil {
		return errors.Wrap(err, "could not read the archive status just written")
	}
	slot := st.Slot()
	b.ArchiveOriginSlot = &slot
	b.archiveRegenPending = !as.Complete
	log.WithFields(logrus.Fields{
		"originSlot":             slot,
		"originEpoch":            slots.ToEpoch(slot),
		"regeneratedThroughSlot": as.RegeneratedThroughSlot,
		"regenerationComplete":   as.Complete,
	}).Info("Archive mode enabled; state-diff tree anchored at the archive origin")
	return nil
}

// checkArchiveOriginBelowOrigin verifies the archive origin sits below the sync origin. It runs after startDB
// because the sync origin only exists once checkpoint data has been written.
func (b *BeaconNode) checkArchiveOriginBelowOrigin(ctx context.Context) error {
	if b.ArchiveOriginSlot == nil {
		return nil
	}
	bf, err := b.db.BackfillStatus(ctx)
	if errors.Is(err, kv.ErrNotFound) {
		// A genesis-synced node has no backfill status; there is nothing below to reach.
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "could not read backfill status")
	}
	if uint64(*b.ArchiveOriginSlot) > bf.OriginSlot {
		return fmt.Errorf("archive origin state slot %d is above the sync origin slot %d; the archive origin "+
			"must be a state the node can backfill down to", *b.ArchiveOriginSlot, bf.OriginSlot)
	}
	return nil
}

// registerArchiveService registers the historical state regeneration service. It must be registered after
// backfill, whose completion it waits on.
func (b *BeaconNode) registerArchiveService() error {
	var backfillService *backfill.Service
	if err := b.services.FetchService(&backfillService); err != nil {
		return err
	}
	d, ok := b.db.(archive.Database)
	if !ok {
		return errors.New("database does not support archive state regeneration")
	}
	svc := archive.New(b.ctx, d, b.stateGen, b.ClockWaiter, backfillService.WaitForCompletion)
	return b.services.RegisterService(svc)
}

// archiveOriginState loads the state named by --archive-origin-state, defaulting to genesis.
func archiveOriginState(cliCtx *cli.Context) (state.BeaconState, error) {
	p := cliCtx.Path(flags.ArchiveOriginState.Name)
	if p == "" {
		st, err := genesis.State()
		if err != nil {
			return nil, errors.Wrap(err, "could not load the genesis state as the archive origin")
		}
		return st, nil
	}
	enc, err := file.ReadFileAsBytes(p)
	if err != nil {
		return nil, errors.Wrapf(err, "could not read archive origin state from %s", p)
	}
	st, err := detect.UnmarshalState(enc)
	if err != nil {
		return nil, errors.Wrapf(err, "could not unmarshal archive origin state from %s", p)
	}
	return st, nil
}

// validateArchiveOrigin runs the checks that are possible before any blocks have been backfilled. The state
// contents themselves are operator-provided trust input, exactly like --checkpoint-state; they are verified
// cryptographically once the forward walk replays the first block on top of them.
func validateArchiveOrigin(st state.BeaconState) error {
	if st == nil || st.IsNil() {
		return errors.New("archive origin state is nil")
	}
	if st.Slot()%params.BeaconConfig().SlotsPerEpoch != 0 || st.Slot()%stateDiffTreeEpochMultiple != 0 {
		return fmt.Errorf("archive origin state slot %d must be an epoch boundary and a multiple of %d",
			st.Slot(), stateDiffTreeEpochMultiple)
	}
	gvr := genesis.ValidatorsRoot()
	if !bytes.Equal(st.GenesisValidatorsRoot(), gvr[:]) {
		return fmt.Errorf("archive origin state is for a different network: genesis_validators_root %#x, expected %#x",
			st.GenesisValidatorsRoot(), gvr)
	}
	return nil
}
