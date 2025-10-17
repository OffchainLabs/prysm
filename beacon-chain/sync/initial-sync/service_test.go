package initialsync

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/abool"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	mock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/kv"
	dbtest "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	testp2p "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	prysmSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/paulbellamy/ratecounter"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestService_Constants(t *testing.T) {
	if params.BeaconConfig().MaxPeersToSync*flags.Get().BlockBatchLimit > 1000 {
		t.Fatal("rpc rejects requests over 1000 range slots")
	}
}

func TestService_InitStartStop(t *testing.T) {
	hook := logTest.NewGlobal()
	resetFlags := flags.Get()
	flags.Init(&flags.GlobalFlags{
		MinimumSyncPeers: 1,
	})
	defer func() {
		flags.Init(resetFlags)
	}()

	tests := []struct {
		name         string
		assert       func()
		setGenesis   func() *startup.Clock
		chainService func() *mock.ChainService
	}{
		{
			name: "head is not ready",
			assert: func() {
				assert.LogsContain(t, hook, "Waiting for state to be initialized")
			},
		},
		{
			name: "future genesis",
			chainService: func() *mock.ChainService {
				// Set to future time (genesis time hasn't arrived yet).
				st, err := util.NewBeaconState()
				require.NoError(t, err)

				return &mock.ChainService{
					State: st,
					FinalizedCheckPoint: &eth.Checkpoint{
						Epoch: 0,
					},
					Genesis:        time.Unix(4113849600, 0),
					ValidatorsRoot: [32]byte{},
				}
			},
			setGenesis: func() *startup.Clock {
				var vr [32]byte
				return startup.NewClock(time.Unix(4113849600, 0), vr)
			},
			assert: func() {
				assert.LogsContain(t, hook, "Genesis time has not arrived - not syncing")
				assert.LogsContain(t, hook, "Waiting for state to be initialized")
			},
		},
		{
			name: "zeroth epoch",
			chainService: func() *mock.ChainService {
				// Set to nearby slot.
				st, err := util.NewBeaconState()
				require.NoError(t, err)
				return &mock.ChainService{
					State: st,
					FinalizedCheckPoint: &eth.Checkpoint{
						Epoch: 0,
					},
					Genesis:        time.Now().Add(-5 * time.Minute),
					ValidatorsRoot: [32]byte{},
				}
			},
			setGenesis: func() *startup.Clock {
				var vr [32]byte
				return startup.NewClock(time.Now().Add(-5*time.Minute), vr)
			},
			assert: func() {
				assert.LogsContain(t, hook, "Chain started within the last epoch - not syncing")
				assert.LogsDoNotContain(t, hook, "Genesis time has not arrived - not syncing")
				assert.LogsContain(t, hook, "Waiting for state to be initialized")
			},
		},
		{
			name: "already synced",
			chainService: func() *mock.ChainService {
				// Set to some future slot, and then make sure that current head matches it.
				st, err := util.NewBeaconState()
				require.NoError(t, err)
				futureSlot := primitives.Slot(27354)
				require.NoError(t, st.SetSlot(futureSlot))
				return &mock.ChainService{
					State: st,
					FinalizedCheckPoint: &eth.Checkpoint{
						Epoch: slots.ToEpoch(futureSlot),
					},
					Genesis:        makeGenesisTime(futureSlot),
					ValidatorsRoot: [32]byte{},
				}
			},
			setGenesis: func() *startup.Clock {
				futureSlot := primitives.Slot(27354)
				var vr [32]byte
				return startup.NewClock(makeGenesisTime(futureSlot), vr)
			},
			assert: func() {
				assert.LogsContain(t, hook, "Starting initial chain sync...")
				assert.LogsContain(t, hook, "Already synced to the current chain head")
				assert.LogsDoNotContain(t, hook, "Chain started within the last epoch - not syncing")
				assert.LogsDoNotContain(t, hook, "Genesis time has not arrived - not syncing")
				assert.LogsContain(t, hook, "Waiting for state to be initialized")
			},
		},
	}

	p := p2ptest.NewTestP2P(t)
	connectPeers(t, p, []*peerData{}, p.Peers())
	for i, tt := range tests {
		if i == 0 {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			defer hook.Reset()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			mc := &mock.ChainService{Genesis: time.Now(), ValidatorsRoot: [32]byte{}}
			// Allow overriding with customized chain service.
			if tt.chainService != nil {
				mc = tt.chainService()
			}
			// Initialize feed
			gs := startup.NewClockSynchronizer()
			s := NewService(ctx, &Config{
				P2P:                 p,
				Chain:               mc,
				ClockWaiter:         gs,
				StateNotifier:       &mock.MockStateNotifier{},
				InitialSyncComplete: make(chan struct{}),
			})
			s.verifierWaiter = verification.NewInitializerWaiter(gs, nil, nil)
			time.Sleep(500 * time.Millisecond)
			assert.NotNil(t, s)
			if tt.setGenesis != nil {
				require.NoError(t, gs.SetClock(tt.setGenesis()))
			}

			wg := &sync.WaitGroup{}
			wg.Add(1)
			go func() {
				s.Start()
				wg.Done()
			}()

			go func() {
				// Allow to exit from test (on no head loop waiting for head is started).
				// In most tests, this is redundant, as Start() already exited.
				time.AfterFunc(3*time.Second, func() {
					cancel()
				})
			}()
			if util.WaitTimeout(wg, time.Second*4) {
				t.Fatalf("Test should have exited by now, timed out")
			}
			tt.assert()
		})
	}
}

func TestService_waitForStateInitialization(t *testing.T) {
	hook := logTest.NewGlobal()
	newService := func(ctx context.Context, mc *mock.ChainService) (*Service, *startup.ClockSynchronizer) {
		cs := startup.NewClockSynchronizer()
		ctx, cancel := context.WithCancel(ctx)
		s := &Service{
			cfg:          &Config{Chain: mc, StateNotifier: mc.StateNotifier(), ClockWaiter: cs, InitialSyncComplete: make(chan struct{})},
			ctx:          ctx,
			cancel:       cancel,
			synced:       abool.New(),
			chainStarted: abool.New(),
			counter:      ratecounter.NewRateCounter(counterSeconds * time.Second),
			genesisChan:  make(chan time.Time),
		}
		s.verifierWaiter = verification.NewInitializerWaiter(cs, nil, nil)
		return s, cs
	}

	t.Run("no state and context close", func(t *testing.T) {
		defer hook.Reset()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		s, _ := newService(ctx, &mock.ChainService{Genesis: time.Now(), ValidatorsRoot: [32]byte{}})
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			s.Start()
			wg.Done()
		}()
		go func() {
			time.AfterFunc(500*time.Millisecond, func() {
				cancel()
			})
		}()

		if util.WaitTimeout(wg, time.Second*2) {
			t.Fatalf("Test should have exited by now, timed out")
		}
		assert.LogsContain(t, hook, "Waiting for state to be initialized")
		assert.LogsContain(t, hook, "Initial-sync failed to receive startup event")
		assert.LogsDoNotContain(t, hook, "Subscription to state notifier failed")
	})

	t.Run("no state and state init event received", func(t *testing.T) {
		defer hook.Reset()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		st, err := util.NewBeaconState()
		require.NoError(t, err)
		gt := st.GenesisTime()
		s, gs := newService(ctx, &mock.ChainService{State: st, Genesis: gt, ValidatorsRoot: [32]byte{}})

		expectedGenesisTime := gt
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			s.Start()
			wg.Done()
		}()
		rg := func() time.Time { return gt.Add(time.Second * 12) }
		go func() {
			time.AfterFunc(200*time.Millisecond, func() {
				var vr [32]byte
				require.NoError(t, gs.SetClock(startup.NewClock(expectedGenesisTime, vr, startup.WithNower(rg))))
			})
		}()

		if util.WaitTimeout(wg, time.Second*2) {
			t.Fatalf("Test should have exited by now, timed out")
		}
		assert.LogsContain(t, hook, "Waiting for state to be initialized")
		assert.LogsContain(t, hook, "Received state initialized event")
		assert.LogsDoNotContain(t, hook, "Context closed, exiting goroutine")
	})

	t.Run("no state and state init event received and service start", func(t *testing.T) {
		defer hook.Reset()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		s, gs := newService(ctx, &mock.ChainService{Genesis: time.Now(), ValidatorsRoot: [32]byte{}})
		// Initialize mock feed
		_ = s.cfg.StateNotifier.StateFeed()

		expectedGenesisTime := time.Now().Add(60 * time.Second)
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			time.AfterFunc(500*time.Millisecond, func() {
				var vr [32]byte
				require.NoError(t, gs.SetClock(startup.NewClock(expectedGenesisTime, vr)))
			})
			s.Start()
			wg.Done()
		}()

		if util.WaitTimeout(wg, time.Second*5) {
			t.Fatalf("Test should have exited by now, timed out")
		}
		assert.LogsContain(t, hook, "Waiting for state to be initialized")
		assert.LogsContain(t, hook, "Received state initialized event")
		assert.LogsDoNotContain(t, hook, "Context closed, exiting goroutine")
	})
}

func TestService_markSynced(t *testing.T) {
	mc := &mock.ChainService{Genesis: time.Now(), ValidatorsRoot: [32]byte{}}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	s := NewService(ctx, &Config{
		Chain:               mc,
		StateNotifier:       mc.StateNotifier(),
		InitialSyncComplete: make(chan struct{}),
	})
	require.NotNil(t, s)
	assert.Equal(t, false, s.chainStarted.IsSet())
	assert.Equal(t, false, s.synced.IsSet())
	assert.Equal(t, true, s.Syncing())
	assert.NoError(t, s.Status())
	s.chainStarted.Set()
	assert.ErrorContains(t, "syncing", s.Status())

	go func() {
		s.markSynced()
	}()

	select {
	case <-s.cfg.InitialSyncComplete:
	case <-ctx.Done():
		require.NoError(t, ctx.Err()) // this is an error because it means initial sync complete failed to close
	}

	assert.Equal(t, false, s.Syncing())
}

func TestService_Resync(t *testing.T) {
	p := p2ptest.NewTestP2P(t)
	connectPeers(t, p, []*peerData{
		{blocks: makeSequence(1, 160), finalizedEpoch: 5, headSlot: 160},
	}, p.Peers())
	cache.initializeRootCache(makeSequence(1, 160), t)
	beaconDB := dbtest.SetupDB(t)
	util.SaveBlock(t, t.Context(), beaconDB, util.NewBeaconBlock())
	cache.RLock()
	genesisRoot := cache.rootCache[0]
	cache.RUnlock()

	hook := logTest.NewGlobal()
	tests := []struct {
		name         string
		assert       func(s *Service)
		chainService func() *mock.ChainService
		wantedErr    string
	}{
		{
			name:      "no head state",
			wantedErr: "could not retrieve head state",
		},
		{
			name: "resync ok",
			chainService: func() *mock.ChainService {
				st, err := util.NewBeaconState()
				require.NoError(t, err)
				futureSlot := primitives.Slot(160)
				genesis := makeGenesisTime(futureSlot)
				require.NoError(t, st.SetGenesisTime(genesis))
				return &mock.ChainService{
					State: st,
					Root:  genesisRoot[:],
					DB:    beaconDB,
					FinalizedCheckPoint: &eth.Checkpoint{
						Epoch: slots.ToEpoch(futureSlot),
					},
					Genesis:        genesis,
					ValidatorsRoot: [32]byte{},
				}
			},
			assert: func(s *Service) {
				assert.LogsContain(t, hook, "Resync attempt complete")
				assert.Equal(t, primitives.Slot(160), s.cfg.Chain.HeadSlot())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer hook.Reset()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			mc := &mock.ChainService{}
			// Allow overriding with customized chain service.
			if tt.chainService != nil {
				mc = tt.chainService()
			}
			s := NewService(ctx, &Config{
				DB:            beaconDB,
				P2P:           p,
				Chain:         mc,
				StateNotifier: mc.StateNotifier(),
				BlobStorage:   filesystem.NewEphemeralBlobStorage(t),
			})
			assert.NotNil(t, s)
			s.genesisTime = mc.Genesis
			assert.Equal(t, primitives.Slot(0), s.cfg.Chain.HeadSlot())
			err := s.Resync()
			if tt.wantedErr != "" {
				assert.ErrorContains(t, tt.wantedErr, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.assert != nil {
				tt.assert(s)
			}
		})
	}
}

func TestService_Initialized(t *testing.T) {
	s := NewService(t.Context(), &Config{
		StateNotifier: &mock.MockStateNotifier{},
	})
	s.chainStarted.Set()
	assert.Equal(t, true, s.Initialized())
	s.chainStarted.UnSet()
	assert.Equal(t, false, s.Initialized())
}

func TestService_Synced(t *testing.T) {
	s := NewService(t.Context(), &Config{})
	s.synced.UnSet()
	assert.Equal(t, false, s.Synced())
	s.synced.Set()
	assert.Equal(t, true, s.Synced())
}

func TestMissingBlobRequest(t *testing.T) {
	ds := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)
	cases := []struct {
		name  string
		setup func(t *testing.T) (blocks.ROBlock, *filesystem.BlobStorage)
		nReq  int
		err   error
	}{
		{
			name: "pre-deneb",
			setup: func(t *testing.T) (blocks.ROBlock, *filesystem.BlobStorage) {
				cb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
				require.NoError(t, err)
				rob, err := blocks.NewROBlockWithRoot(cb, [32]byte{})
				require.NoError(t, err)
				return rob, nil
			},
			nReq: 0,
		},
		{
			name: "deneb zero commitments",
			setup: func(t *testing.T) (blocks.ROBlock, *filesystem.BlobStorage) {
				bk, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 0, 0)
				return bk, nil
			},
			nReq: 0,
		},
		{
			name: "2 commitments, all missing",
			setup: func(t *testing.T) (blocks.ROBlock, *filesystem.BlobStorage) {
				bk, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 0, 2)
				fs := filesystem.NewEphemeralBlobStorage(t)
				return bk, fs
			},
			nReq: 2,
		},
		{
			name: "2 commitments, 1 missing",
			setup: func(t *testing.T) (blocks.ROBlock, *filesystem.BlobStorage) {
				bk, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, ds, 2)
				bm, fs := filesystem.NewEphemeralBlobStorageWithMocker(t)
				require.NoError(t, bm.CreateFakeIndices(bk.Root(), bk.Block().Slot(), 1))
				return bk, fs
			},
			nReq: 1,
		},
		{
			name: "2 commitments, 0 missing",
			setup: func(t *testing.T) (blocks.ROBlock, *filesystem.BlobStorage) {
				bk, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, ds, 2)
				bm, fs := filesystem.NewEphemeralBlobStorageWithMocker(t)
				require.NoError(t, bm.CreateFakeIndices(bk.Root(), bk.Block().Slot(), 0, 1))
				return bk, fs
			},
			nReq: 0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			blk, store := c.setup(t)
			req, err := missingBlobRequest(blk, store)
			require.NoError(t, err)
			require.Equal(t, c.nReq, len(req))
		})
	}
}

func TestOriginOutsideRetention(t *testing.T) {
	ctx := t.Context()
	bdb := dbtest.SetupDB(t)
	genesis := time.Unix(0, 0)
	secsPerEpoch := params.BeaconConfig().SecondsPerSlot * uint64(params.BeaconConfig().SlotsPerEpoch)
	retentionPeriod := time.Second * time.Duration(uint64(params.BeaconConfig().MinEpochsForBlobsSidecarsRequest+1)*secsPerEpoch)
	outsideRetention := genesis.Add(retentionPeriod)
	now := func() time.Time {
		return outsideRetention
	}
	clock := startup.NewClock(genesis, [32]byte{}, startup.WithNower(now))
	s := &Service{ctx: ctx, cfg: &Config{DB: bdb}, clock: clock}
	blk, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 0, 1)
	require.NoError(t, bdb.SaveBlock(ctx, blk))
	concreteDB, ok := bdb.(*kv.Store)
	require.Equal(t, true, ok)
	require.NoError(t, concreteDB.SaveOriginCheckpointBlockRoot(ctx, blk.Root()))
	// This would break due to missing service dependencies, but will return nil fast due to being outside retention.
	require.Equal(t, false, params.WithinDAPeriod(slots.ToEpoch(blk.Block().Slot()), slots.ToEpoch(clock.CurrentSlot())))
	require.NoError(t, s.fetchOriginSidecars([]peer.ID{}))
}

func TestFetchOriginSidecars(t *testing.T) {
	ctx := t.Context()

	beaconConfig := params.BeaconConfig()
	genesisTime := time.Date(2025, time.August, 10, 0, 0, 0, 0, time.UTC)
	secondsPerSlot := beaconConfig.SecondsPerSlot
	slotsPerEpoch := beaconConfig.SlotsPerEpoch
	secondsPerEpoch := uint64(slotsPerEpoch.Mul(secondsPerSlot))
	retentionEpochs := beaconConfig.MinEpochsForDataColumnSidecarsRequest

	genesisValidatorRoot := [fieldparams.RootLength]byte{}

	t.Run("out of retention period", func(t *testing.T) {
		// Create an origin block.
		block := util.NewBeaconBlockFulu()
		signedBlock, err := blocks.NewSignedBeaconBlock(block)
		require.NoError(t, err)
		roBlock, err := blocks.NewROBlock(signedBlock)
		require.NoError(t, err)

		// Save the block.
		db := dbtest.SetupDB(t)
		err = db.SaveOriginCheckpointBlockRoot(ctx, roBlock.Root())
		require.NoError(t, err)
		err = db.SaveBlock(ctx, roBlock)
		require.NoError(t, err)

		// Define "now" to be one epoch after genesis time + retention period.
		nowWrtGenesisSecs := retentionEpochs.Add(1).Mul(secondsPerEpoch)
		now := genesisTime.Add(time.Duration(nowWrtGenesisSecs) * time.Second)
		nower := func() time.Time { return now }
		clock := startup.NewClock(genesisTime, genesisValidatorRoot, startup.WithNower(nower))

		service := &Service{
			cfg: &Config{
				DB: db,
			},
			clock: clock,
		}

		err = service.fetchOriginSidecars(nil)
		require.NoError(t, err)
	})

	t.Run("no commitments", func(t *testing.T) {
		// Create an origin block.
		block := util.NewBeaconBlockFulu()
		signedBlock, err := blocks.NewSignedBeaconBlock(block)
		require.NoError(t, err)
		roBlock, err := blocks.NewROBlock(signedBlock)
		require.NoError(t, err)

		// Save the block.
		db := dbtest.SetupDB(t)
		err = db.SaveOriginCheckpointBlockRoot(ctx, roBlock.Root())
		require.NoError(t, err)
		err = db.SaveBlock(ctx, roBlock)
		require.NoError(t, err)

		// Define "now" to be after genesis time + retention period.
		nowWrtGenesisSecs := retentionEpochs.Mul(secondsPerEpoch)
		now := genesisTime.Add(time.Duration(nowWrtGenesisSecs) * time.Second)
		nower := func() time.Time { return now }
		clock := startup.NewClock(genesisTime, genesisValidatorRoot, startup.WithNower(nower))

		service := &Service{
			cfg: &Config{
				DB:  db,
				P2P: p2ptest.NewTestP2P(t),
			},
			clock: clock,
		}

		err = service.fetchOriginSidecars(nil)
		require.NoError(t, err)
	})

	t.Run("nominal", func(t *testing.T) {
		samplesPerSlot := params.BeaconConfig().SamplesPerSlot

		// Start the trusted setup.
		err := kzg.Start()
		require.NoError(t, err)

		// Create block and sidecars.
		const blobCount = 1
		roBlock, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, blobCount)

		// Save the block.
		db := dbtest.SetupDB(t)
		err = db.SaveOriginCheckpointBlockRoot(ctx, roBlock.Root())
		require.NoError(t, err)

		err = db.SaveBlock(ctx, roBlock)
		require.NoError(t, err)

		// Create a data columns storage.
		dir := t.TempDir()
		dataColumnStorage, err := filesystem.NewDataColumnStorage(ctx, filesystem.WithDataColumnBasePath(dir))
		require.NoError(t, err)

		// Compute the columns to request.
		p2p := p2ptest.NewTestP2P(t)
		custodyGroupCount, err := p2p.CustodyGroupCount(t.Context())
		require.NoError(t, err)

		samplingSize := max(custodyGroupCount, samplesPerSlot)
		info, _, err := peerdas.Info(p2p.NodeID(), samplingSize)
		require.NoError(t, err)

		// Save all sidecars except what we need.
		toSave := make([]blocks.VerifiedRODataColumn, 0, uint64(len(verifiedRoSidecars))-samplingSize)
		for _, sidecar := range verifiedRoSidecars {
			if !info.CustodyColumns[sidecar.Index] {
				toSave = append(toSave, sidecar)
			}
		}

		err = dataColumnStorage.Save(toSave)
		require.NoError(t, err)

		// Define "now" to be after genesis time + retention period.
		nowWrtGenesisSecs := retentionEpochs.Mul(secondsPerEpoch)
		now := genesisTime.Add(time.Duration(nowWrtGenesisSecs) * time.Second)
		nower := func() time.Time { return now }
		clock := startup.NewClock(genesisTime, genesisValidatorRoot, startup.WithNower(nower))

		service := &Service{
			cfg: &Config{
				DB:                db,
				P2P:               p2p,
				DataColumnStorage: dataColumnStorage,
			},
			clock: clock,
		}

		err = service.fetchOriginSidecars(nil)
		require.NoError(t, err)

		// Check that needed sidecars are saved.
		summary := dataColumnStorage.Summary(roBlock.Root())
		for index := range info.CustodyColumns {
			require.Equal(t, true, summary.HasIndex(index))
		}
	})
}

func TestFetchOriginColumns(t *testing.T) {
	// Load the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	// Setup test environment
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	cfg.BlobSchedule = []params.BlobScheduleEntry{{Epoch: 0, MaxBlobsPerBlock: 10}}
	params.OverrideBeaconConfig(cfg)

	const (
		delay     = 0
		blobCount = 1
	)

	t.Run("block has no commitments", func(t *testing.T) {
		service := new(Service)

		// Create a block with no blob commitments
		block := util.NewBeaconBlockFulu()
		signedBlock, err := blocks.NewSignedBeaconBlock(block)
		require.NoError(t, err)
		roBlock, err := blocks.NewROBlock(signedBlock)
		require.NoError(t, err)

		err = service.fetchOriginDataColumnSidecars(roBlock, delay)
		require.NoError(t, err)
	})

	t.Run("FetchDataColumnSidecars succeeds immediately", func(t *testing.T) {
		storage := filesystem.NewEphemeralDataColumnStorage(t)
		p2p := p2ptest.NewTestP2P(t)

		service := &Service{
			cfg: &Config{
				P2P:               p2p,
				DataColumnStorage: storage,
			},
		}

		// Create a block with blob commitments and sidecars
		roBlock, _, verifiedSidecars := util.GenerateTestFuluBlockWithSidecars(t, blobCount)

		// Store all sidecars in advance so FetchDataColumnSidecars succeeds immediately
		err := storage.Save(verifiedSidecars)
		require.NoError(t, err)

		err = service.fetchOriginDataColumnSidecars(roBlock, delay)
		require.NoError(t, err)
	})

	t.Run("first attempt to FetchDataColumnSidecars fails but second attempt succeeds", func(t *testing.T) {
		numberOfCustodyGroups := params.BeaconConfig().NumberOfCustodyGroups
		storage := filesystem.NewEphemeralDataColumnStorage(t)

		// Custody columns with this private key and 4-cgc: 31, 81, 97, 105
		privateKeyBytes := [32]byte{1}
		privateKey, err := crypto.UnmarshalSecp256k1PrivateKey(privateKeyBytes[:])
		require.NoError(t, err)

		protocol := fmt.Sprintf("%s/ssz_snappy", p2p.RPCDataColumnSidecarsByRangeTopicV1)

		p2p, other := testp2p.NewTestP2P(t), testp2p.NewTestP2P(t, libp2p.Identity(privateKey))
		p2p.Peers().SetConnectionState(other.PeerID(), peers.Connected)
		p2p.Connect(other)

		p2p.Peers().SetChainState(other.PeerID(), &ethpb.StatusV2{
			HeadSlot: 5,
		})

		other.ENR().Set(peerdas.Cgc(numberOfCustodyGroups))
		p2p.Peers().UpdateENR(other.ENR(), other.PeerID())

		allBut42 := make([]uint64, 0, numberOfCustodyGroups-1)
		for i := range numberOfCustodyGroups {
			if i != 42 {
				allBut42 = append(allBut42, i)
			}
		}

		expectedRequests := []*ethpb.DataColumnSidecarsByRangeRequest{
			{
				StartSlot: 0,
				Count:     1,
				Columns:   []uint64{1, 17, 19, 42, 75, 87, 102, 117},
			},
			{
				StartSlot: 0,
				Count:     1,
				Columns:   allBut42,
			},
			{
				StartSlot: 0,
				Count:     1,
				Columns:   []uint64{1, 17, 19, 75, 87, 102, 117},
			},
		}

		toRespondByAttempt := [][]uint64{
			{42},
			{},
			{1, 17, 19, 75, 87, 102, 117},
		}

		clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})

		gs := startup.NewClockSynchronizer()
		err = gs.SetClock(startup.NewClock(time.Unix(4113849600, 0), [fieldparams.RootLength]byte{}))
		require.NoError(t, err)

		waiter := verification.NewInitializerWaiter(gs, nil, nil)
		initializer, err := waiter.WaitForInitializer(t.Context())
		require.NoError(t, err)

		newDataColumnsVerifier := newDataColumnsVerifierFromInitializer(initializer)

		// Create a block with blob commitments and sidecars
		roBlock, _, verifiedRoSidecars := util.GenerateTestFuluBlockWithSidecars(t, blobCount)

		ctxMap, err := prysmSync.ContextByteVersionsForValRoot(params.BeaconConfig().GenesisValidatorsRoot)
		require.NoError(t, err)

		service := &Service{
			ctx:                    t.Context(),
			clock:                  clock,
			newDataColumnsVerifier: newDataColumnsVerifier,
			cfg: &Config{
				P2P:               p2p,
				DataColumnStorage: storage,
			},
			ctxMap: ctxMap,
		}

		// Do not respond any sidecar on the first attempt, and respond everything requested on the second one.
		attempt := 0
		other.SetStreamHandler(protocol, func(stream network.Stream) {
			actualRequest := new(ethpb.DataColumnSidecarsByRangeRequest)
			err := other.Encoding().DecodeWithMaxLength(stream, actualRequest)
			assert.NoError(t, err)
			assert.DeepEqual(t, expectedRequests[attempt], actualRequest)

			for _, column := range toRespondByAttempt[attempt] {
				err = prysmSync.WriteDataColumnSidecarChunk(stream, clock, other.Encoding(), verifiedRoSidecars[column].DataColumnSidecar)
				assert.NoError(t, err)
			}

			err = stream.CloseWrite()
			assert.NoError(t, err)

			attempt++
		})

		err = service.fetchOriginDataColumnSidecars(roBlock, delay)
		require.NoError(t, err)

		// Check all corresponding sidecars are saved in the store.
		summary := storage.Summary(roBlock.Root())
		for _, indices := range toRespondByAttempt {
			for _, index := range indices {
				require.Equal(t, true, summary.HasIndex(index))
			}
		}
	})
}
