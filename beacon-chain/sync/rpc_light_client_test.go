package sync

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/abool"
	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	lightClient "github.com/OffchainLabs/prysm/v6/beacon-chain/core/light-client"
	db "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	leakybucket "github.com/OffchainLabs/prysm/v6/container/leaky-bucket"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
)

func TestRPC_LightClientBootstrap_Altair(t *testing.T) {
	origNC := params.BeaconConfig()
	// restore network config after test completes
	defer func() {
		params.OverrideBeaconConfig(origNC)
	}()

	params.SetupTestConfigCleanup(t)
	ctx := context.Background()
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = 1
	cfg.ForkVersionSchedule[[4]byte{1, 0, 0, 0}] = 1
	params.OverrideBeaconConfig(cfg)

	p2pService := p2ptest.NewTestP2P(t)
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	assert.Equal(t, 1, len(p1.BHost.Network().Peers()), "Expected peers to be connected")

	secondsPerSlot := int(params.BeaconConfig().SecondsPerSlot)
	slotIntervals := int(params.BeaconConfig().IntervalsPerSlot)
	slotsPerEpoch := int(params.BeaconConfig().SlotsPerEpoch)

	genesisDrift := slotsPerEpoch*secondsPerSlot + 2*secondsPerSlot + secondsPerSlot/slotIntervals
	chainService := &mockChain.ChainService{
		ValidatorsRoot: [32]byte{'A'},
		Genesis:        time.Unix(time.Now().Unix()-int64(genesisDrift), 0),
	}
	d := db.SetupDB(t)
	r := Service{
		ctx: ctx,
		cfg: &config{
			p2p:           p2pService,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			beaconDB:      d,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			stateNotifier: &mockChain.MockStateNotifier{},
		},
		chainStarted: abool.New(),
		lcStore:      &lightClient.Store{},
		subHandler:   newSubTopicHandler(),
		rateLimiter:  newRateLimiter(p1),
	}
	pcl := protocol.ID(p2p.RPCLightClientBootstrapTopicV1)
	topic := string(pcl)
	r.rateLimiter.limiterMap[topic] = leakybucket.NewCollector(10000, 10000, time.Second, false)

	l := util.NewTestLightClient(t, 1)
	bootstrap, err := lightClient.NewLightClientBootstrapFromBeaconState(ctx, l.State.Slot(), l.State, l.Block)
	require.NoError(t, err)
	blockRoot, err := l.Block.Block().HashTreeRoot()
	require.NoError(t, err)

	require.NoError(t, r.cfg.beaconDB.SaveLightClientBootstrap(ctx, blockRoot[:], bootstrap))

	var wg sync.WaitGroup
	wg.Add(1)
	p2.BHost.SetStreamHandler(pcl, func(stream network.Stream) {
		defer wg.Done()
		expectSuccess(t, stream)
		var res pb.LightClientBootstrapAltair
		assert.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
	})

	stream1, err := p1.BHost.NewStream(context.Background(), p2.BHost.ID(), pcl)
	require.NoError(t, err)
	err = r.lightClientBootstrapRPCHandler(ctx, &blockRoot, stream1)
	require.NoError(t, err)

	if util.WaitTimeout(&wg, 100*time.Second) {
		t.Fatal("Did not receive stream within 1 sec")
	}
}

func TestRPC_LightClientOptimisticUpdate_Altair(t *testing.T) {
	origNC := params.BeaconConfig()
	// restore network config after test completes
	defer func() {
		params.OverrideBeaconConfig(origNC)
	}()

	params.SetupTestConfigCleanup(t)
	ctx := context.Background()
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = 1
	cfg.ForkVersionSchedule[[4]byte{1, 0, 0, 0}] = 1
	params.OverrideBeaconConfig(cfg)

	p2pService := p2ptest.NewTestP2P(t)
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	assert.Equal(t, 1, len(p1.BHost.Network().Peers()), "Expected peers to be connected")

	secondsPerSlot := int(params.BeaconConfig().SecondsPerSlot)
	slotIntervals := int(params.BeaconConfig().IntervalsPerSlot)
	slotsPerEpoch := int(params.BeaconConfig().SlotsPerEpoch)

	genesisDrift := slotsPerEpoch*secondsPerSlot + 2*secondsPerSlot + secondsPerSlot/slotIntervals
	chainService := &mockChain.ChainService{
		ValidatorsRoot: [32]byte{'A'},
		Genesis:        time.Unix(time.Now().Unix()-int64(genesisDrift), 0),
	}
	d := db.SetupDB(t)
	r := Service{
		ctx: ctx,
		cfg: &config{
			p2p:           p2pService,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			beaconDB:      d,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			stateNotifier: &mockChain.MockStateNotifier{},
		},
		chainStarted: abool.New(),
		lcStore:      &lightClient.Store{},
		subHandler:   newSubTopicHandler(),
		rateLimiter:  newRateLimiter(p1),
	}
	pcl := protocol.ID(p2p.RPCLightClientOptimisticUpdateTopicV1)
	topic := string(pcl)
	r.rateLimiter.limiterMap[topic] = leakybucket.NewCollector(10000, 10000, time.Second, false)

	l := util.NewTestLightClient(t, 1)

	update, err := lightClient.NewLightClientOptimisticUpdateFromBeaconState(ctx, l.State.Slot(), l.State, l.Block, l.AttestedState, l.AttestedBlock)
	require.NoError(t, err)

	r.lcStore.SetLastOptimisticUpdate(update)

	var wg sync.WaitGroup
	wg.Add(1)
	p2.BHost.SetStreamHandler(pcl, func(stream network.Stream) {
		defer wg.Done()
		expectSuccess(t, stream)
		var res pb.LightClientOptimisticUpdateAltair
		assert.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
	})

	stream1, err := p1.BHost.NewStream(context.Background(), p2.BHost.ID(), pcl)
	require.NoError(t, err)
	err = r.lightClientOptimisticUpdateRPCHandler(ctx, nil, stream1)
	require.NoError(t, err)

	if util.WaitTimeout(&wg, 1*time.Second) {
		t.Fatal("Did not receive stream within 1 sec")
	}
}
