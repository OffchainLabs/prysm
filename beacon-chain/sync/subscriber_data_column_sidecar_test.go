package sync

import (
	"context"
	"iter"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/partialdatacolumnbroadcaster"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestAllDataColumnSubnets(t *testing.T) {
	t.Run("returns nil when no validators tracked", func(t *testing.T) {
		// Service with no tracked validators
		svc := &Service{
			ctx: t.Context(),
		}

		result := svc.allDataColumnSubnets(primitives.Slot(0))
		assert.Equal(t, true, len(result) == 0, "Expected nil or empty map when no validators are tracked")
	})

	t.Run("returns all subnets logic test", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		ctx := t.Context()

		beaconDB := dbtest.SetupDB(t)

		// Create and save genesis state
		genesisState, _ := util.DeterministicGenesisState(t, 64)
		require.NoError(t, beaconDB.SaveGenesisData(ctx, genesisState))

		// Create stategen and initialize with genesis state
		stateGen := stategen.New(beaconDB, doublylinkedtree.New())
		_, err := stateGen.Resume(ctx, genesisState)
		require.NoError(t, err)

		// At least one attached validator.
		svc := cache.NewSubscribedValidatorsCache()
		svc.Add(1)

		s := &Service{
			ctx:                       ctx,
			subscribedValidatorsCache: svc,
			cfg: &config{
				stateGen: stateGen,
				beaconDB: beaconDB,
			},
		}

		dataColumnSidecarSubnetCount := params.BeaconConfig().DataColumnSidecarSubnetCount
		result := s.allDataColumnSubnets(0)
		assert.Equal(t, dataColumnSidecarSubnetCount, uint64(len(result)))

		for i := range dataColumnSidecarSubnetCount {
			assert.Equal(t, true, result[i])
		}
	})
}

// TestProcessDataColumnSidecarsFromReconstruction_GloasSkipsProposerIndex is a regression test:
// Gloas sidecars don't expose a proposer index, so reconstruction must not call ProposerIndex().
// With no stored columns, reconstruction is unnecessary and the call returns nil instead of erroring.
func TestProcessDataColumnSidecarsFromReconstruction_GloasSkipsProposerIndex(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	s := &Service{
		ctx: t.Context(),
		cfg: &config{
			p2p:               mockp2p.NewTestP2P(t),
			clock:             startup.NewClock(time.Now(), [32]byte{}),
			dataColumnStorage: filesystem.NewEphemeralDataColumnStorage(t),
		},
		seenDataColumnCache: newSlotAwareCache(seenDataColumnSize),
	}

	var root [fieldparams.RootLength]byte
	root[0] = 0xEE
	gdc, err := blocks.NewRODataColumnGloasWithRoot(&ethpb.DataColumnSidecarGloas{
		Index:           0,
		Slot:            1,
		BeaconBlockRoot: root[:],
	}, root)
	require.NoError(t, err)
	v := blocks.NewVerifiedRODataColumn(gdc)

	require.NoError(t, s.processDataColumnSidecarsFromReconstruction(t.Context(), v))
}

// blockingPartialBroadcaster holds every Publish call until released (or the context is done),
// standing in for a saturated partial column broadcaster.
type blockingPartialBroadcaster struct {
	partialdatacolumnbroadcaster.Broadcaster
	release     chan struct{}
	releaseOnce sync.Once

	mu    sync.Mutex
	calls int
}

func newBlockingPartialBroadcaster() *blockingPartialBroadcaster {
	return &blockingPartialBroadcaster{release: make(chan struct{})}
}

func (b *blockingPartialBroadcaster) Publish(ctx context.Context, topicsAndColumns iter.Seq2[string, blocks.PartialDataColumn]) error {
	for range topicsAndColumns {
	}
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *blockingPartialBroadcaster) Release() {
	b.releaseOnce.Do(func() { close(b.release) })
}

func (b *blockingPartialBroadcaster) Calls() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

// signallingChainService reports the number of data columns received on each ReceiveDataColumns call.
type signallingChainService struct {
	*chainMock.ChainService
	received chan int
}

func newSignallingChainService(chain *chainMock.ChainService) *signallingChainService {
	return &signallingChainService{ChainService: chain, received: make(chan int, 1024)}
}

func (c *signallingChainService) ReceiveDataColumns(dcs []blocks.VerifiedRODataColumn) error {
	err := c.ChainService.ReceiveDataColumns(dcs)
	c.received <- len(dcs)
	return err
}

// waitReceived returns the number of data columns received by the next ReceiveDataColumns call, failing
// if none happens while the partial publish is still held back.
func (c *signallingChainService) waitReceived(t *testing.T) int {
	select {
	case n := <-c.received:
		return n
	case <-time.After(10 * time.Second):
		t.Fatal("data columns were not received while the partial publish was blocked")
		return 0
	}
}

func TestDataColumnSubscriber_ReceiveDoesNotWaitOnPartialPublish(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.CapellaForkEpoch = 0
	cfg.DenebForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.FuluForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	require.NoError(t, kzg.Start())

	_, _, sidecars := util.GenerateTestFuluBlockWithSidecars(t, 1)
	chainService := newSignallingChainService(&chainMock.ChainService{})
	p2p := mockp2p.NewTestP2P(t)
	broadcaster := newBlockingPartialBroadcaster()
	p2p.SetPartialColumnBroadcaster(broadcaster)
	_, _, err := p2p.UpdateCustodyInfo(0, params.BeaconConfig().CustodyRequirement)
	require.NoError(t, err)

	s := &Service{
		cfg: &config{
			p2p:                    p2p,
			chain:                  chainService,
			clock:                  startup.NewClock(time.Now(), [32]byte{}),
			dataColumnStorage:      filesystem.NewEphemeralDataColumnStorage(t),
			executionReconstructor: &mockExecution.EngineClient{},
			operationNotifier:      &chainMock.MockOperationNotifier{},
		},
		seenDataColumnCache: newSlotAwareCache(1),
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- s.dataColumnSubscriber(ctx, sidecars[0].DataColumnSidecar())
	}()

	// The column is received while its partial publish is still held back.
	require.Equal(t, 1, chainService.waitReceived(t))

	// Release the publish, and stop the execution path, which retries until every sampled column is seen.
	broadcaster.Release()
	cancel()
	require.NoError(t, <-done)
	// At least one: the execution path may also have published partials before it was stopped.
	require.NotEqual(t, 0, broadcaster.Calls(), "column was not published as partial")
}
