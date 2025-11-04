package node

import (
	"context"
	"os"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/kv"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/slasherkv"
	"github.com/OffchainLabs/prysm/v6/cmd"
	"github.com/OffchainLabs/prysm/v6/genesis"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

type dbClearer struct {
	shouldClear bool
	force       bool
	confirmed   bool
}

const (
	clearConfirmation = "This will delete your beacon chain database stored in your data directory. " +
		"Your database backups will not be removed - do you want to proceed? (Y/N)"

	clearDeclined = "Database will not be deleted. No changes have been made."
)

func (c *dbClearer) clearKV(ctx context.Context, db *kv.Store) (*kv.Store, error) {
	if !c.shouldProceed() {
		return db, nil
	}

	log.Warning("Removing database")
	if err := db.ClearDB(); err != nil {
		return nil, errors.Wrap(err, "could not clear database")
	}
	return kv.NewKVStore(ctx, db.DatabasePath())
}

func (c *dbClearer) clearGenesis(dir string) error {
	if !c.shouldProceed() {
		return nil
	}

	gfile, err := genesis.FindStateFile(dir)
	if err != nil {
		return nil
	}

	if err := os.Remove(gfile.FilePath()); err != nil {
		return errors.Wrapf(err, "genesis state file not removed: %s", gfile.FilePath())
	}
	return nil
}

func (c *dbClearer) clearBlobs(bs *filesystem.BlobStorage) error {
	if !c.shouldProceed() {
		return nil
	}

	log.Warning("Removing blob storage")
	if err := bs.Clear(); err != nil {
		return errors.Wrap(err, "could not clear blob storage")
	}

	return nil
}

func (c *dbClearer) clearColumns(cs *filesystem.DataColumnStorage) error {
	if !c.shouldProceed() {
		return nil
	}

	log.Warning("Removing data columns storage")
	if err := cs.Clear(); err != nil {
		return errors.Wrap(err, "could not clear data columns storage")
	}

	return nil
}

func (c *dbClearer) clearSlasher(ctx context.Context, db *slasherkv.Store) (*slasherkv.Store, error) {
	if !c.shouldProceed() {
		return db, nil
	}

	log.Warning("Removing slasher database")
	if err := db.ClearDB(); err != nil {
		return nil, errors.Wrap(err, "could not clear slasher database")
	}
	return slasherkv.NewKVStore(ctx, db.DatabasePath())
}

func (c *dbClearer) shouldProceed() bool {
	if !c.shouldClear {
		return false
	}
	if c.force {
		return true
	}
	if !c.confirmed {
		confirmed, err := cmd.ConfirmAction(clearConfirmation, clearDeclined)
		if err != nil {
			log.WithError(err).Error("Not clearing db due to confirmation error")
			return false
		}
		c.confirmed = confirmed
	}
	return c.confirmed
}

func newDbClearer(cliCtx *cli.Context) *dbClearer {
	force := cliCtx.Bool(cmd.ForceClearDB.Name)
	return &dbClearer{
		shouldClear: cliCtx.Bool(cmd.ClearDB.Name) || force,
		force:       force,
	}
}
