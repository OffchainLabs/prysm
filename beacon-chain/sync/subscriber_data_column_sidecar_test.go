package sync

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestAllDataColumnSubnets(t *testing.T) {
	t.Run("returns nil when no validators tracked", func(t *testing.T) {
		// Service with no tracked validators
		svc := &Service{
			ctx:                       t.Context(),
			subscribedValidatorsCache: cache.NewSubscribedValidatorsCache(time.Hour, 15*time.Minute),
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

		// At least one attached validator.
		svc := cache.NewSubscribedValidatorsCache(time.Hour, 15*time.Minute)
		svc.Add(1)

		s := &Service{
			ctx:                       ctx,
			subscribedValidatorsCache: svc,
			cfg: &config{
				stateGen: stateGen,
				beaconDB: db,
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
