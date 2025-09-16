package sync

import (
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/abool"
	"github.com/OffchainLabs/prysm/v6/async/event"
	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	db "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	lightClient "github.com/OffchainLabs/prysm/v6/beacon-chain/light-client"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	leakybucket "github.com/OffchainLabs/prysm/v6/container/leaky-bucket"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
)

func TestRPC_LightClientBootstrap(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	ctx := t.Context()
	p2pService := p2ptest.NewTestP2P(t)
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	require.Equal(t, 1, len(p1.BHost.Network().Peers()), "Expected peers to be connected")

	chainService := &mockChain.ChainService{
		ValidatorsRoot: [32]byte{'A'},
		Genesis:        time.Unix(time.Now().Unix(), 0),
	}
	d := db.SetupDB(t)
	lcStore := lightClient.NewLightClientStore(&p2ptest.FakeP2P{}, new(event.Feed), d)

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
		lcStore:      lcStore,
		subHandler:   newSubTopicHandler(),
		rateLimiter:  newRateLimiter(p1),
	}
	pcl := protocol.ID(p2p.RPCLightClientBootstrapTopicV1)
	topic := string(pcl)
	r.rateLimiter.limiterMap[topic] = leakybucket.NewCollector(10000, 10000, time.Second, false)

	altairDigest := params.ForkDigest(params.BeaconConfig().AltairForkEpoch)
	bellatrixDigest := params.ForkDigest(params.BeaconConfig().BellatrixForkEpoch)
	capellaDigest := params.ForkDigest(params.BeaconConfig().CapellaForkEpoch)
	denebDigest := params.ForkDigest(params.BeaconConfig().DenebForkEpoch)
	electraDigest := params.ForkDigest(params.BeaconConfig().ElectraForkEpoch)
	for i := 1; i <= 5; i++ {
		t.Run(version.String(i), func(t *testing.T) {
			l := util.NewTestLightClient(t, i)
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
				rpcCtx, err := readContextFromStream(stream)
				require.NoError(t, err)
				require.Equal(t, 4, len(rpcCtx))

				var resSSZ []byte

				switch i {
				case version.Altair:
					require.DeepSSZEqual(t, altairDigest[:], rpcCtx)
					var res pb.LightClientBootstrapAltair
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				case version.Bellatrix:
					require.DeepSSZEqual(t, bellatrixDigest[:], rpcCtx)
					var res pb.LightClientBootstrapAltair
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				case version.Capella:
					require.DeepSSZEqual(t, capellaDigest[:], rpcCtx)
					var res pb.LightClientBootstrapCapella
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				case version.Deneb:
					require.DeepSSZEqual(t, denebDigest[:], rpcCtx)
					var res pb.LightClientBootstrapDeneb
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				case version.Electra:
					require.DeepSSZEqual(t, electraDigest[:], rpcCtx)
					var res pb.LightClientBootstrapElectra
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				default:
					t.Fatalf("unsupported version %d", i)
				}

				bootstrapSSZ, err := bootstrap.MarshalSSZ()
				require.NoError(t, err)
				require.DeepSSZEqual(t, resSSZ, bootstrapSSZ)
			})

			stream1, err := p1.BHost.NewStream(t.Context(), p2.BHost.ID(), pcl)
			require.NoError(t, err)
			err = r.lightClientBootstrapRPCHandler(ctx, &blockRoot, stream1)
			require.NoError(t, err)

			if util.WaitTimeout(&wg, 1*time.Second) {
				t.Fatal("Did not receive stream within 1 sec")
			}
		})
	}

}

func TestRPC_LightClientOptimisticUpdate(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	ctx := t.Context()
	p2pService := p2ptest.NewTestP2P(t)
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	require.Equal(t, 1, len(p1.BHost.Network().Peers()), "Expected peers to be connected")

	chainService := &mockChain.ChainService{
		ValidatorsRoot: [32]byte{'A'},
		Genesis:        time.Unix(time.Now().Unix(), 0),
	}
	d := db.SetupDB(t)
	lcStore := lightClient.NewLightClientStore(&p2ptest.FakeP2P{}, new(event.Feed), d)

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
		lcStore:      lcStore,
		subHandler:   newSubTopicHandler(),
		rateLimiter:  newRateLimiter(p1),
	}
	pcl := protocol.ID(p2p.RPCLightClientOptimisticUpdateTopicV1)
	topic := string(pcl)
	r.rateLimiter.limiterMap[topic] = leakybucket.NewCollector(10000, 10000, time.Second, false)

	altairDigest := params.ForkDigest(params.BeaconConfig().AltairForkEpoch)
	bellatrixDigest := params.ForkDigest(params.BeaconConfig().BellatrixForkEpoch)
	capellaDigest := params.ForkDigest(params.BeaconConfig().CapellaForkEpoch)
	denebDigest := params.ForkDigest(params.BeaconConfig().DenebForkEpoch)
	electraDigest := params.ForkDigest(params.BeaconConfig().ElectraForkEpoch)

	for i := 1; i <= 5; i++ {
		t.Run(version.String(i), func(t *testing.T) {
			l := util.NewTestLightClient(t, i)

			update, err := lightClient.NewLightClientOptimisticUpdateFromBeaconState(ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock)
			require.NoError(t, err)

			r.lcStore.SetLastOptimisticUpdate(update, false)

			var wg sync.WaitGroup
			wg.Add(1)
			p2.BHost.SetStreamHandler(pcl, func(stream network.Stream) {
				defer wg.Done()
				expectSuccess(t, stream)
				var resSSZ []byte

				rpcCtx, err := readContextFromStream(stream)
				require.NoError(t, err)
				require.Equal(t, 4, len(rpcCtx))

				switch i {
				case version.Altair:
					require.DeepSSZEqual(t, altairDigest[:], rpcCtx)
					var res pb.LightClientOptimisticUpdateAltair
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				case version.Bellatrix:
					require.DeepSSZEqual(t, bellatrixDigest[:], rpcCtx)
					var res pb.LightClientOptimisticUpdateAltair
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				case version.Capella:
					require.DeepSSZEqual(t, capellaDigest[:], rpcCtx)
					var res pb.LightClientOptimisticUpdateCapella
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				case version.Deneb:
					require.DeepSSZEqual(t, denebDigest[:], rpcCtx)
					var res pb.LightClientOptimisticUpdateDeneb
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				case version.Electra:
					require.DeepSSZEqual(t, electraDigest[:], rpcCtx)
					var res pb.LightClientOptimisticUpdateDeneb
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				default:
					t.Fatalf("unsupported version %d", i)
				}

				updateSSZ, err := update.MarshalSSZ()
				require.NoError(t, err)
				require.DeepSSZEqual(t, resSSZ, updateSSZ)
			})

			stream1, err := p1.BHost.NewStream(t.Context(), p2.BHost.ID(), pcl)
			require.NoError(t, err)
			err = r.lightClientOptimisticUpdateRPCHandler(ctx, nil, stream1)
			require.NoError(t, err)

			if util.WaitTimeout(&wg, 1*time.Second) {
				t.Fatal("Did not receive stream within 1 sec")
			}
		})
	}
}

func TestRPC_LightClientFinalityUpdate(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	ctx := t.Context()
	p2pService := p2ptest.NewTestP2P(t)
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	require.Equal(t, 1, len(p1.BHost.Network().Peers()), "Expected peers to be connected")

	chainService := &mockChain.ChainService{
		ValidatorsRoot: [32]byte{'A'},
		Genesis:        time.Unix(time.Now().Unix(), 0),
	}
	d := db.SetupDB(t)
	lcStore := lightClient.NewLightClientStore(&p2ptest.FakeP2P{}, new(event.Feed), d)

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
		lcStore:      lcStore,
		subHandler:   newSubTopicHandler(),
		rateLimiter:  newRateLimiter(p1),
	}
	pcl := protocol.ID(p2p.RPCLightClientFinalityUpdateTopicV1)
	topic := string(pcl)
	r.rateLimiter.limiterMap[topic] = leakybucket.NewCollector(10000, 10000, time.Second, false)

	altairDigest := params.ForkDigest(params.BeaconConfig().AltairForkEpoch)
	bellatrixDigest := params.ForkDigest(params.BeaconConfig().BellatrixForkEpoch)
	capellaDigest := params.ForkDigest(params.BeaconConfig().CapellaForkEpoch)
	denebDigest := params.ForkDigest(params.BeaconConfig().DenebForkEpoch)
	electraDigest := params.ForkDigest(params.BeaconConfig().ElectraForkEpoch)

	for i := 1; i <= 5; i++ {
		t.Run(version.String(i), func(t *testing.T) {
			l := util.NewTestLightClient(t, i)

			update, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
			require.NoError(t, err)

			r.lcStore.SetLastFinalityUpdate(update, false)

			var wg sync.WaitGroup
			wg.Add(1)
			p2.BHost.SetStreamHandler(pcl, func(stream network.Stream) {
				defer wg.Done()
				expectSuccess(t, stream)
				var resSSZ []byte

				rpcCtx, err := readContextFromStream(stream)
				require.NoError(t, err)
				require.Equal(t, 4, len(rpcCtx))

				switch i {
				case version.Altair:
					require.DeepSSZEqual(t, altairDigest[:], rpcCtx)
					var res pb.LightClientFinalityUpdateAltair
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				case version.Bellatrix:
					require.DeepSSZEqual(t, bellatrixDigest[:], rpcCtx)
					var res pb.LightClientFinalityUpdateAltair
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				case version.Capella:
					require.DeepSSZEqual(t, capellaDigest[:], rpcCtx)
					var res pb.LightClientFinalityUpdateCapella
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				case version.Deneb:
					require.DeepSSZEqual(t, denebDigest[:], rpcCtx)
					var res pb.LightClientFinalityUpdateDeneb
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				case version.Electra:
					require.DeepSSZEqual(t, electraDigest[:], rpcCtx)
					var res pb.LightClientFinalityUpdateElectra
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				default:
					t.Fatalf("unsupported version %d", i)
				}

				updateSSZ, err := update.MarshalSSZ()
				require.NoError(t, err)
				require.DeepSSZEqual(t, resSSZ, updateSSZ)
			})

			stream1, err := p1.BHost.NewStream(t.Context(), p2.BHost.ID(), pcl)
			require.NoError(t, err)
			err = r.lightClientFinalityUpdateRPCHandler(ctx, nil, stream1)
			require.NoError(t, err)

			if util.WaitTimeout(&wg, 1*time.Second) {
				t.Fatal("Did not receive stream within 1 sec")
			}
		})
	}
}

func TestRPC_LightClientUpdatesByRange(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	ctx := t.Context()
	p2pService := p2ptest.NewTestP2P(t)
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	require.Equal(t, 1, len(p1.BHost.Network().Peers()), "Expected peers to be connected")

	blk := util.NewBeaconBlock()
	signedBlk, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	chainService := &mockChain.ChainService{
		ValidatorsRoot: [32]byte{'A'},
		Genesis:        time.Unix(time.Now().Unix(), 0),
		Block:          signedBlk,
	}
	d := db.SetupDB(t)
	lcStore := lightClient.NewLightClientStore(&p2ptest.FakeP2P{}, new(event.Feed), d)
	require.NoError(t, err)

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
		lcStore:      lcStore,
		subHandler:   newSubTopicHandler(),
		rateLimiter:  newRateLimiter(p1),
	}
	pcl := protocol.ID(p2p.RPCLightClientUpdatesByRangeTopicV1)
	topic := string(pcl)
	r.rateLimiter.limiterMap[topic] = leakybucket.NewCollector(10000, 10000, time.Second, false)

	altairDigest := params.ForkDigest(params.BeaconConfig().AltairForkEpoch)
	bellatrixDigest := params.ForkDigest(params.BeaconConfig().BellatrixForkEpoch)
	capellaDigest := params.ForkDigest(params.BeaconConfig().CapellaForkEpoch)
	denebDigest := params.ForkDigest(params.BeaconConfig().DenebForkEpoch)
	electraDigest := params.ForkDigest(params.BeaconConfig().ElectraForkEpoch)

	for i := 1; i <= 5; i++ {
		t.Run(version.String(i), func(t *testing.T) {
			for j := 0; j < 5; j++ {
				l := util.NewTestLightClient(t, i, util.WithIncreasedAttestedSlot(uint64(j)))
				update, err := lightClient.NewLightClientUpdateFromBeaconState(ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
				require.NoError(t, err)
				require.NoError(t, r.cfg.beaconDB.SaveLightClientUpdate(ctx, uint64(j), update))
			}

			var wg sync.WaitGroup
			wg.Add(1)

			responseCounter := 0

			p2.BHost.SetStreamHandler(pcl, func(stream network.Stream) {
				defer wg.Done()
				expectSuccess(t, stream)
				rpcCtx, err := readContextFromStream(stream)
				require.NoError(t, err)
				require.Equal(t, 4, len(rpcCtx))

				var resSSZ []byte

				switch i {
				case version.Altair:
					require.DeepSSZEqual(t, altairDigest[:], rpcCtx)
					var res pb.LightClientUpdateAltair
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				case version.Bellatrix:
					require.DeepSSZEqual(t, bellatrixDigest[:], rpcCtx)
					var res pb.LightClientUpdateAltair
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				case version.Capella:
					require.DeepSSZEqual(t, capellaDigest[:], rpcCtx)
					var res pb.LightClientUpdateCapella
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				case version.Deneb:
					require.DeepSSZEqual(t, denebDigest[:], rpcCtx)
					var res pb.LightClientUpdateDeneb
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				case version.Electra:
					require.DeepSSZEqual(t, electraDigest[:], rpcCtx)
					var res pb.LightClientUpdateElectra
					require.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, &res))
					resSSZ, err = res.MarshalSSZ()
					require.NoError(t, err)
				default:
					t.Fatalf("unsupported version %d", i)
				}

				updates, err := r.lcStore.LightClientUpdates(ctx, 0, 4, signedBlk)
				require.NoError(t, err)
				updateSSZ, err := updates[uint64(responseCounter)].MarshalSSZ()
				require.NoError(t, err)
				require.DeepSSZEqual(t, resSSZ, updateSSZ)
				responseCounter++
			})

			stream1, err := p1.BHost.NewStream(t.Context(), p2.BHost.ID(), pcl)
			require.NoError(t, err)

			msg := pb.LightClientUpdatesByRangeRequest{
				StartPeriod: 0,
				Count:       5,
			}
			err = r.lightClientUpdatesByRangeRPCHandler(ctx, &msg, stream1)
			require.NoError(t, err)

			if util.WaitTimeout(&wg, 1*time.Second) {
				t.Fatal("Did not receive stream within 1 sec")
			}
		})
	}

}
