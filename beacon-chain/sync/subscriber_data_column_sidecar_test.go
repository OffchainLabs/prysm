package sync

import (
	"context"
	"iter"
	"sync"
	"testing"

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
	"github.com/OffchainLabs/prysm/v7/time"
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

// recordingPartialBroadcaster records how many data columns the chain had received at each Publish call.
type recordingPartialBroadcaster struct {
	partialdatacolumnbroadcaster.Broadcaster
	received  func() int
	published chan struct{}

	mu                sync.Mutex
	receivedAtPublish []int
}

func newRecordingPartialBroadcaster(received func() int) *recordingPartialBroadcaster {
	return &recordingPartialBroadcaster{received: received, published: make(chan struct{}, 1024)}
}

func (b *recordingPartialBroadcaster) Publish(_ context.Context, topicsAndColumns iter.Seq2[string, blocks.PartialDataColumn]) error {
	for range topicsAndColumns {
	}
	b.mu.Lock()
	b.receivedAtPublish = append(b.receivedAtPublish, b.received())
	b.mu.Unlock()
	b.published <- struct{}{}
	return nil
}

func (b *recordingPartialBroadcaster) counts() []int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]int(nil), b.receivedAtPublish...)
}

func TestDataColumnSubscriber_ReceivesBeforePartialPublish(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.CapellaForkEpoch = 0
	cfg.DenebForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.FuluForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	require.NoError(t, kzg.Start())

	_, _, sidecars := util.GenerateTestFuluBlockWithSidecars(t, 1)
	chainService := &chainMock.ChainService{}
	p2p := mockp2p.NewTestP2P(t)
	broadcaster := newRecordingPartialBroadcaster(func() int { return len(chainService.DataColumns) })
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

	// The execution path retries until every sampled column is seen, so stop it once a partial is published.
	<-broadcaster.published
	cancel()
	require.NoError(t, <-done)

	counts := broadcaster.counts()
	require.NotEqual(t, 0, len(counts))
	for _, received := range counts {
		require.Equal(t, 1, received, "partial published before the column was received")
	}
}
