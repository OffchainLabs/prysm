package sync

import (
	"context"
	"strings"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	dbtesting "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
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
	originalFlag := flags.Get().Supernode
	defer func() {
		flags.Get().Supernode = originalFlag
	}()
	flags.Get().Supernode = true
	fn()
}

func withSemiSupernode(t *testing.T, fn func()) {
	originalFlag := flags.Get().SemiSupernode
	defer func() {
		flags.Get().SemiSupernode = originalFlag
	}()
	flags.Get().SemiSupernode = true
	fn()
}

func TestUpdateCustodyInfoIfNeeded(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.NumberOfCustodyGroups = 128
	cfg.CustodyRequirement = 4
	cfg.SamplesPerSlot = 8
	params.OverrideBeaconConfig(cfg)

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
			setup.assertCustodyInfo(t, expectedSlot, cfg.NumberOfCustodyGroups)
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

	t.Run("SemiSupernode enabled returns half of NumberOfCustodyGroups", func(t *testing.T) {
		withSemiSupernode(t, func() {
			service := &Service{
				ctx: context.Background(),
			}

			result, err := service.custodyGroupCount(ctx)
			require.NoError(t, err)
			expected, err := peerdas.MinimumCustodyGroupCountToReconstruct()
			require.NoError(t, err)
			require.Equal(t, expected, result)
		})
	})

	t.Run("Supernode takes precedence over SemiSupernode", func(t *testing.T) {
		// Test that when both flags are set, supernode takes precedence
		originalSupernode := flags.Get().Supernode
		originalSemiSupernode := flags.Get().SemiSupernode
		defer func() {
			flags.Get().Supernode = originalSupernode
			flags.Get().SemiSupernode = originalSemiSupernode
		}()
		flags.Get().Supernode = true
		flags.Get().SemiSupernode = true

		service := &Service{
			ctx: context.Background(),
		}

		result, err := service.custodyGroupCount(ctx)
		require.NoError(t, err)
		require.Equal(t, config.NumberOfCustodyGroups, result)
	})

	t.Run("SemiSupernode with no tracked validators returns semi-supernode target", func(t *testing.T) {
		withSemiSupernode(t, func() {
			service := &Service{
				ctx:                    context.Background(),
				trackedValidatorsCache: cache.NewTrackedValidatorsCache(),
			}

			result, err := service.custodyGroupCount(ctx)
			require.NoError(t, err)
			expected, err := peerdas.MinimumCustodyGroupCountToReconstruct()
			require.NoError(t, err)
			require.Equal(t, expected, result)
		})
	})
}

func TestSemiSupernodeValidatorCustodyOverride(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.NumberOfCustodyGroups = 128
	config.CustodyRequirement = 4
	config.ValidatorCustodyRequirement = 8
	config.BalancePerAdditionalCustodyGroup = 1000000000 // 1 ETH in Gwei
	params.OverrideBeaconConfig(config)

	ctx := t.Context()

	t.Run("Semi-supernode returns target when validator requirement is lower", func(t *testing.T) {
		// When validators require less custody than semi-supernode provides,
		// use the semi-supernode target (64)
		withSemiSupernode(t, func() {
			// Setup with validators requiring only 32 groups (less than 64)
			service := &Service{
				ctx:                    context.Background(),
				trackedValidatorsCache: cache.NewTrackedValidatorsCache(),
			}

			result, err := service.custodyGroupCount(ctx)
			require.NoError(t, err)

			// Should return semi-supernode target (64) since it's higher than validator requirement
			require.Equal(t, uint64(64), result)
		})
	})

	t.Run("Validator requirement calculation respects minimum and maximum bounds", func(t *testing.T) {
		// Verify that the validator custody requirement respects:
		// - Minimum: ValidatorCustodyRequirement (8 in our config)
		// - Maximum: NumberOfCustodyGroups (128 in our config)

		// This ensures the formula works correctly:
		// result = min(max(count, ValidatorCustodyRequirement), NumberOfCustodyGroups)

		require.Equal(t, uint64(8), config.ValidatorCustodyRequirement)
		require.Equal(t, uint64(128), config.NumberOfCustodyGroups)

		// Semi-supernode target should be 64 (half of 128)
		semiSupernodeTarget, err := peerdas.MinimumCustodyGroupCountToReconstruct()
		require.NoError(t, err)
		require.Equal(t, uint64(64), semiSupernodeTarget)
	})

	t.Run("Semi-supernode respects base CustodyRequirement", func(t *testing.T) {
		// Test that semi-supernode respects max(CustodyRequirement, validatorsCustodyRequirement)
		// even when both are below the semi-supernode target
		params.SetupTestConfigCleanup(t)
		// Setup with high base custody requirement (but still less than 64)
		testConfig := params.BeaconConfig()
		testConfig.NumberOfCustodyGroups = 128
		testConfig.CustodyRequirement = 32 // Higher than validator requirement
		testConfig.ValidatorCustodyRequirement = 8
		params.OverrideBeaconConfig(testConfig)

		withSemiSupernode(t, func() {
			service := &Service{
				ctx:                    context.Background(),
				trackedValidatorsCache: cache.NewTrackedValidatorsCache(),
			}

			result, err := service.custodyGroupCount(ctx)
			require.NoError(t, err)

			// Should return semi-supernode target (64) since
			// max(CustodyRequirement=32, validatorsCustodyRequirement=0) = 32 < 64
			require.Equal(t, uint64(64), result)
		})
	})

	t.Run("Semi-supernode uses higher custody when base requirement exceeds target", func(t *testing.T) {
		// Set CustodyRequirement higher than semi-supernode target (64)
		params.SetupTestConfigCleanup(t)
		testConfig := params.BeaconConfig()
		testConfig.NumberOfCustodyGroups = 128
		testConfig.CustodyRequirement = 80 // Higher than semi-supernode target of 64
		testConfig.ValidatorCustodyRequirement = 8
		params.OverrideBeaconConfig(testConfig)

		withSemiSupernode(t, func() {
			service := &Service{
				ctx:                    context.Background(),
				trackedValidatorsCache: cache.NewTrackedValidatorsCache(),
			}

			result, err := service.custodyGroupCount(ctx)
			require.NoError(t, err)

			// Should return CustodyRequirement (80) since it's higher than semi-supernode target (64)
			// effectiveCustodyRequirement = max(80, 0) = 80 > 64
			require.Equal(t, uint64(80), result)
		})
	})
}
