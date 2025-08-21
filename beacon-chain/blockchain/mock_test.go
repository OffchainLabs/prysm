package blockchain

import (
	"testing"

	mock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func testServiceOptsWithDB(t testing.TB) []Option {
	beaconDB := testDB.SetupDB(t)
	fcs := doublylinkedtree.New()
	cs := startup.NewClockSynchronizer()
	return []Option{
		WithDatabase(beaconDB),
		WithStateGen(stategen.New(beaconDB, fcs)),
		WithForkChoiceStore(fcs),
		WithClockSynchronizer(cs),
		WithStateNotifier(&mock.MockStateNotifier{RecordEvents: true}),
		WithSyncChecker(&mock.MockSyncChecker{}),
	}
}

// WARNING: only use these opts when you are certain there are no db calls
// in your code path. this is a lightweight way to satisfy the stategen/beacondb
// initialization requirements w/o the overhead of db init.
func testServiceOptsNoDB() []Option {
	cs := startup.NewClockSynchronizer()
	return []Option{WithClockSynchronizer(cs)}
}

func testServiceNoDB(t testing.TB) *Service {
	s, err := NewService(t.Context(), testServiceOptsNoDB()...)
	require.NoError(t, err)
	return s
}

func testServiceWithDB(t testing.TB) *Service {
	s, err := NewService(t.Context(), testServiceOptsWithDB(t)...)
	require.NoError(t, err)
	return s
}
