package sync

import (
	"context"
	"strings"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	dbtesting "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

type testSetup struct {
	service      *Service
	p2pService   *p2ptest.TestP2P
	beaconDB     db.Database
	ctx          context.Context
	initialSlot  primitives.Slot
	initialCount uint64
}

func setupCustodyTest(t *testing.T, withChain bool) *testSetup {
	ctx := t.Context()
	p2pService := p2ptest.NewTestP2P(t)
	beaconDB := dbtesting.SetupDB(t)

	const (
		initialEarliestSlot = primitives.Slot(50)
		initialCustodyCount = uint64(5)
	)

	_, _, err := p2pService.UpdateCustodyInfo(initialEarliestSlot, initialCustodyCount)
	require.NoError(t, err)

	dbEarliestAvailableSlot, dbCustodyCount, err := beaconDB.UpdateCustodyInfo(ctx, initialEarliestSlot, initialCustodyCount)
	require.NoError(t, err)
	require.Equal(t, initialEarliestSlot, dbEarliestAvailableSlot)
	require.Equal(t, initialCustodyCount, dbCustodyCount)

	cfg := &config{
		p2p:      p2pService,
		beaconDB: beaconDB,
	}

	if withChain {
		const headSlot = primitives.Slot(100)
		block, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlock{
			Block: &ethpb.BeaconBlock{
				Body: &ethpb.BeaconBlockBody{},
				Slot: headSlot,
			},
		})
		require.NoError(t, err)

		cfg.chain = &mock.ChainService{
			Genesis:          time.Now(),
			ValidAttestation: true,
			FinalizedCheckPoint: &ethpb.Checkpoint{
				Epoch: 0,
			},
			Block: block,
		}
	}

	service := &Service{
		ctx:                    ctx,
		cfg:                    cfg,
		trackedValidatorsCache: cache.NewTrackedValidatorsCache(),
	}

	return &testSetup{
		service:      service,
		p2pService:   p2pService,
		beaconDB:     beaconDB,
		ctx:          ctx,
		initialSlot:  initialEarliestSlot,
		initialCount: initialCustodyCount,
	}
}

func (ts *testSetup) assertCustodyInfo(t *testing.T, expectedSlot primitives.Slot, expectedCount uint64) {
	ctx := t.Context()

	p2pEarliestSlot, err := ts.p2pService.EarliestAvailableSlot(ctx)
	require.NoError(t, err)
	require.Equal(t, expectedSlot, p2pEarliestSlot)

	p2pCustodyCount, err := ts.p2pService.CustodyGroupCount(ctx)
	require.NoError(t, err)
	require.Equal(t, expectedCount, p2pCustodyCount)

	dbEarliestSlot, dbCustodyCount, err := ts.beaconDB.UpdateCustodyInfo(ts.ctx, 0, 0)
	require.NoError(t, err)
	require.Equal(t, expectedSlot, dbEarliestSlot)
	require.Equal(t, expectedCount, dbCustodyCount)
}

func withSubscribeAllDataSubnets(t *testing.T, fn func()) {
	originalFlag := flags.Get().SubscribeAllDataSubnets
	defer func() {
		flags.Get().SubscribeAllDataSubnets = originalFlag
	}()
	flags.Get().SubscribeAllDataSubnets = true
	fn()
}

func TestUpdateCustodyInfoIfNeeded(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	beaconConfig := params.BeaconConfig()
	beaconConfig.NumberOfCustodyGroups = 128
	beaconConfig.CustodyRequirement = 4
	beaconConfig.SamplesPerSlot = 8
	params.OverrideBeaconConfig(beaconConfig)

	t.Run("Skip update when actual custody count >= target", func(t *testing.T) {
		setup := setupCustodyTest(t, false)

		err := setup.service.updateCustodyInfoIfNeeded()
		require.NoError(t, err)

		setup.assertCustodyInfo(t, setup.initialSlot, setup.initialCount)
	})

	t.Run("not enough peers in some subnets", func(t *testing.T) {
		const randomTopic = "aTotalRandomTopicName"
		require.Equal(t, false, strings.Contains(randomTopic, p2p.GossipDataColumnSidecarMessage))

		withSubscribeAllDataSubnets(t, func() {
			setup := setupCustodyTest(t, false)

			_, err := setup.service.cfg.p2p.SubscribeToTopic(p2p.GossipDataColumnSidecarMessage)
			require.NoError(t, err)

			_, err = setup.service.cfg.p2p.SubscribeToTopic(randomTopic)
			require.NoError(t, err)

			err = setup.service.updateCustodyInfoIfNeeded()
			require.NoError(t, err)

			setup.assertCustodyInfo(t, setup.initialSlot, setup.initialCount)
		})
	})

	t.Run("should update", func(t *testing.T) {
		withSubscribeAllDataSubnets(t, func() {
			setup := setupCustodyTest(t, true)

			err := setup.service.updateCustodyInfoIfNeeded()
			require.NoError(t, err)

			const expectedSlot = primitives.Slot(100)
			setup.assertCustodyInfo(t, expectedSlot, beaconConfig.NumberOfCustodyGroups)
		})
	})
}

func TestCustodyGroupCount(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.NumberOfCustodyGroups = 10
	config.CustodyRequirement = 3
	params.OverrideBeaconConfig(config)

	ctx := t.Context()

	t.Run("SubscribeAllDataSubnets enabled returns NumberOfCustodyGroups", func(t *testing.T) {
		withSubscribeAllDataSubnets(t, func() {
			service := &Service{
				ctx: context.Background(),
			}

			result, err := service.custodyGroupCount(ctx)
			require.NoError(t, err)
			require.Equal(t, config.NumberOfCustodyGroups, result)
		})
	})

	t.Run("No tracked validators returns CustodyRequirement", func(t *testing.T) {
		service := &Service{
			ctx:                    context.Background(),
			trackedValidatorsCache: cache.NewTrackedValidatorsCache(),
		}

		result, err := service.custodyGroupCount(ctx)
		require.NoError(t, err)
		require.Equal(t, config.CustodyRequirement, result)
	})
}
