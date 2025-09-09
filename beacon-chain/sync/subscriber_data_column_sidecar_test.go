package sync

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	dbtest "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
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

		db := dbtest.SetupDB(t)

		// Create and save genesis state
		genesisState, _ := util.DeterministicGenesisState(t, 64)
		require.NoError(t, db.SaveGenesisData(ctx, genesisState))

		// Create stategen and initialize with genesis state
		stateGen := stategen.New(db, doublylinkedtree.New())
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
				beaconDB: db,
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
