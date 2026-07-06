package sync

import (
	"testing"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

func saveBlockAtSlot(t *testing.T, database db.NoHeadAccessDatabase, slot primitives.Slot) [32]byte {
	t.Helper()
	b := util.NewBeaconBlock()
	b.Block.Slot = slot
	util.SaveBlock(t, t.Context(), database, b)
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	return root
}

func newGloasGroupCallbacks(t *testing.T, chain blockchainService, database db.NoHeadAccessDatabase) *partialColumnCallbacks {
	return &partialColumnCallbacks{service: &Service{
		cfg: &config{chain: chain, beaconDB: database},
		ctx: t.Context(),
	}}
}

func TestValidateGloasGroupID(t *testing.T) {
	t.Run("ignored when the chain service is unavailable", func(t *testing.T) {
		c := newGloasGroupCallbacks(t, nil, dbtest.SetupDB(t))
		require.Equal(t, pubsub.ValidationIgnore, c.ValidateGloasGroupID(5, [32]byte{'r', 'o', 'o', 't'}))
	})

	t.Run("ignored when the block for the root has not been seen", func(t *testing.T) {
		// A mock chain with no DB reports HasBlock == false for every root.
		c := newGloasGroupCallbacks(t, &mock.ChainService{}, dbtest.SetupDB(t))
		require.Equal(t, pubsub.ValidationIgnore, c.ValidateGloasGroupID(5, [32]byte{'u', 'n', 's', 'e', 'e', 'n'}))
	})

	t.Run("ignored when the block is absent from the database", func(t *testing.T) {
		database := dbtest.SetupDB(t)
		root := [32]byte{'m', 'i', 's', 's', 'i', 'n', 'g'}
		// HasBlock reports true via InitSyncBlockRoots, but the block was never saved, so the DB
		// lookup yields nil and we ignore rather than reject.
		chain := &mock.ChainService{DB: database, InitSyncBlockRoots: map[[32]byte]bool{root: true}}
		c := newGloasGroupCallbacks(t, chain, database)
		require.Equal(t, pubsub.ValidationIgnore, c.ValidateGloasGroupID(5, root))
	})

	t.Run("rejected when the block slot does not match the group slot", func(t *testing.T) {
		database := dbtest.SetupDB(t)
		root := saveBlockAtSlot(t, database, 5)
		c := newGloasGroupCallbacks(t, &mock.ChainService{DB: database}, database)
		// The group claims slot 6, but the seen block at this root is at slot 5.
		require.Equal(t, pubsub.ValidationReject, c.ValidateGloasGroupID(6, root))
	})

	t.Run("accepted when the block slot matches the group slot", func(t *testing.T) {
		database := dbtest.SetupDB(t)
		root := saveBlockAtSlot(t, database, 5)
		c := newGloasGroupCallbacks(t, &mock.ChainService{DB: database}, database)
		require.Equal(t, pubsub.ValidationAccept, c.ValidateGloasGroupID(5, root))
	})
}
