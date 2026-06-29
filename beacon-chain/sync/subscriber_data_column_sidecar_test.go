package sync

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
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
			ctx:                    t.Context(),
			trackedValidatorsCache: cache.NewTrackedValidatorsCache(),
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

		// At least one tracked validator.
		tvc := cache.NewTrackedValidatorsCache()
		tvc.Set(cache.TrackedValidator{Active: true, Index: 1})

		svc := &Service{
			ctx:                    ctx,
			trackedValidatorsCache: tvc,
			cfg: &config{
				stateGen: stateGen,
				beaconDB: beaconDB,
			},
		}

		dataColumnSidecarSubnetCount := params.BeaconConfig().DataColumnSidecarSubnetCount
		result := svc.allDataColumnSubnets(0)
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
