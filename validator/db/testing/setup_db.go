package testing

import (
	"context"
	"testing"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/validator/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/validator/db/iface"
	"github.com/prysmaticlabs/prysm/v5/validator/db/kv"
)

// SetupDB instantiates and returns a DB instance for the validator client.
// The `minimal` flag indicates whether the DB should be instantiated with minimal, filesystem
// slashing protection database.
func SetupDB(t testing.TB, pubkeys [][fieldparams.BLSPubkeyLength]byte, minimal bool) iface.ValidatorDB {
	var (
		db  iface.ValidatorDB
		err error
	)

	// Create a new DB instance.
	if minimal {
		config := &filesystem.Config{PubKeys: pubkeys}
		db, err = filesystem.NewStore(t.TempDir(), config)
	} else {
		config := &kv.Config{PubKeys: pubkeys}
		db, err = kv.NewKVStore(context.Background(), t.TempDir(), config)
	}

	if err != nil {
		t.Fatalf("Failed to instantiate DB: %v", err)
	}

	// Cleanup the DB after the test.
	t.Cleanup(func() {
		if err := db.ClearDB(); err != nil {
			t.Fatalf("Failed to clear database: %v", err)
		}
	})

	return db
}
