package blockchain

import (
	"context"
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v6/async/event"
	mock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache/depositsnapshot"
	statefeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/state"
	lightclient "github.com/OffchainLabs/prysm/v6/beacon-chain/core/light-client"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	mockExecution "github.com/OffchainLabs/prysm/v6/beacon-chain/execution/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice"
	doublylinkedtree "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/blstoexec"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"google.golang.org/protobuf/proto"
)

type mockBeaconNode struct {
	stateFeed *event.Feed
	mu        sync.Mutex
}

// StateFeed mocks the same method in the beacon node.
func (mbn *mockBeaconNode) StateFeed() event.SubscriberSender {
	mbn.mu.Lock()
	defer mbn.mu.Unlock()
	if mbn.stateFeed == nil {
		mbn.stateFeed = new(event.Feed)
	}
	return mbn.stateFeed
}

type mockBroadcaster struct {
	broadcastCalled bool
}

func (mb *mockBroadcaster) Broadcast(_ context.Context, _ proto.Message) error {
	mb.broadcastCalled = true
	return nil
}

func (mb *mockBroadcaster) BroadcastAttestation(_ context.Context, _ uint64, _ ethpb.Att) error {
	mb.broadcastCalled = true
	return nil
}

func (mb *mockBroadcaster) BroadcastSyncCommitteeMessage(_ context.Context, _ uint64, _ *ethpb.SyncCommitteeMessage) error {
	mb.broadcastCalled = true
	return nil
}

func (mb *mockBroadcaster) BroadcastBlob(_ context.Context, _ uint64, _ *ethpb.BlobSidecar) error {
	mb.broadcastCalled = true
	return nil
}

func (mb *mockBroadcaster) BroadcastLightClientOptimisticUpdate(_ context.Context, _ interfaces.LightClientOptimisticUpdate) error {
	mb.broadcastCalled = true
	return nil
}

func (mb *mockBroadcaster) BroadcastLightClientFinalityUpdate(_ context.Context, _ interfaces.LightClientFinalityUpdate) error {
	mb.broadcastCalled = true
	return nil
}

func (mb *mockBroadcaster) BroadcastBLSChanges(_ context.Context, _ []*ethpb.SignedBLSToExecutionChange) {
}

var _ p2p.Broadcaster = (*mockBroadcaster)(nil)

type testServiceRequirements struct {
	ctx     context.Context
	db      db.Database
	fcs     forkchoice.ForkChoicer
	sg      *stategen.State
	notif   statefeed.Notifier
	cs      *startup.ClockSynchronizer
	attPool attestations.Pool
	attSrv  *attestations.Service
	blsPool *blstoexec.Pool
	dc      *depositsnapshot.Cache
}

func minimalTestService(t *testing.T, opts ...Option) (*Service, *testServiceRequirements) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	fcs := doublylinkedtree.New()
	sg := stategen.New(beaconDB, fcs)
	notif := &mockBeaconNode{}
	fcs.SetBalancesByRooter(sg.ActiveNonSlashedBalancesByRoot)
	cs := startup.NewClockSynchronizer()
	attPool := attestations.NewPool()
	attSrv, err := attestations.NewService(ctx, &attestations.Config{Pool: attPool})
	require.NoError(t, err)
	blsPool := blstoexec.NewPool()
	dc, err := depositsnapshot.New()
	require.NoError(t, err)
	req := &testServiceRequirements{
		ctx:     ctx,
		db:      beaconDB,
		fcs:     fcs,
		sg:      sg,
		notif:   notif,
		cs:      cs,
		attPool: attPool,
		attSrv:  attSrv,
		blsPool: blsPool,
		dc:      dc,
	}
	defOpts := []Option{WithDatabase(req.db),
		WithStateNotifier(req.notif),
		WithStateGen(req.sg),
		WithForkChoiceStore(req.fcs),
		WithClockSynchronizer(req.cs),
		WithAttestationPool(req.attPool),
		WithAttestationService(req.attSrv),
		WithBLSToExecPool(req.blsPool),
		WithDepositCache(dc),
		WithTrackedValidatorsCache(cache.NewTrackedValidatorsCache()),
		WithBlobStorage(filesystem.NewEphemeralBlobStorage(t)),
		WithSyncChecker(mock.MockChecker{}),
		WithExecutionEngineCaller(&mockExecution.EngineClient{}),
		WithLightClientStore(&lightclient.Store{}),
	}
	// append the variadic opts so they override the defaults by being processed afterwards
	opts = append(defOpts, opts...)
	s, err := NewService(req.ctx, opts...)

	require.NoError(t, err)
	return s, req
}
