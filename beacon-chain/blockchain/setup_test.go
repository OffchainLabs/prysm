package blockchain

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/event"
	mock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache/depositsnapshot"
	statefeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	mockExecution "github.com/OffchainLabs/prysm/v6/beacon-chain/execution/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice"
	doublylinkedtree "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/doubly-linked-tree"
	lightclient "github.com/OffchainLabs/prysm/v6/beacon-chain/light-client"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/blstoexec"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2pTesting "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/libp2p/go-libp2p/core/peer"
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

type mockAccessor struct {
	mockBroadcaster
	mockCustodyManager
	p2pTesting.MockPeerManager
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

func (mb *mockBroadcaster) BroadcastDataColumnSidecars(_ context.Context, _ []blocks.VerifiedRODataColumn) error {
	mb.broadcastCalled = true
	return nil
}

func (mb *mockBroadcaster) BroadcastBLSChanges(_ context.Context, _ []*ethpb.SignedBLSToExecutionChange) {
}

var _ p2p.Broadcaster = (*mockBroadcaster)(nil)

// mockCustodyManager is a mock implementation of p2p.CustodyManager
type mockCustodyManager struct {
	mut                   sync.RWMutex
	earliestAvailableSlot primitives.Slot
	custodyGroupCount     uint64
}

func (dch *mockCustodyManager) EarliestAvailableSlot(context.Context) (primitives.Slot, error) {
	dch.mut.RLock()
	defer dch.mut.RUnlock()

	return dch.earliestAvailableSlot, nil
}

func (dch *mockCustodyManager) CustodyGroupCount(context.Context) (uint64, error) {
	dch.mut.RLock()
	defer dch.mut.RUnlock()

	return dch.custodyGroupCount, nil
}

func (dch *mockCustodyManager) UpdateCustodyInfo(earliestAvailableSlot primitives.Slot, custodyGroupCount uint64) (primitives.Slot, uint64, error) {
	dch.mut.Lock()
	defer dch.mut.Unlock()

	dch.earliestAvailableSlot = earliestAvailableSlot
	dch.custodyGroupCount = custodyGroupCount

	return earliestAvailableSlot, custodyGroupCount, nil
}

func (dch *mockCustodyManager) UpdateEarliestAvailableSlot(earliestAvailableSlot primitives.Slot) error {
	dch.mut.Lock()
	defer dch.mut.Unlock()

	dch.earliestAvailableSlot = earliestAvailableSlot
	return nil
}

func (dch *mockCustodyManager) CustodyGroupCountFromPeer(peer.ID) uint64 {
	return 0
}

var _ p2p.CustodyManager = (*mockCustodyManager)(nil)

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
	ctx := t.Context()
	genesis := time.Now().Add(-1 * 4 * time.Duration(params.BeaconConfig().SlotsPerEpoch*primitives.Slot(params.BeaconConfig().SecondsPerSlot)) * time.Second) // Genesis was 4 epochs ago.
	beaconDB := testDB.SetupDB(t)
	fcs := doublylinkedtree.New()
	fcs.SetGenesisTime(genesis)
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
		WithDataColumnStorage(filesystem.NewEphemeralDataColumnStorage(t)),
		WithSyncChecker(mock.MockChecker{}),
		WithExecutionEngineCaller(&mockExecution.EngineClient{}),
		WithP2PBroadcaster(&mockAccessor{}),
		WithLightClientStore(&lightclient.Store{}),
		WithGenesisTime(genesis),
	}
	// append the variadic opts so they override the defaults by being processed afterwards
	opts = append(defOpts, opts...)
	s, err := NewService(req.ctx, opts...)

	require.NoError(t, err)
	return s, req
}
