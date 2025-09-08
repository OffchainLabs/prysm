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

func TestDataColumnSubnets(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) *Service
		test  func(t *testing.T, svc *Service)
	}{
		{
			name: "allDataColumnSubnets returns nil when no validators tracked",
			setup: func(t *testing.T) *Service {
				// Service with no tracked validators
				return &Service{
					ctx:                    t.Context(),
					trackedValidatorsCache: cache.NewTrackedValidatorsCache(),
				}
			},
			test: func(t *testing.T, svc *Service) {
				result := svc.allDataColumnSubnets(primitives.Slot(0))
				assert.Equal(t, true, len(result) == 0, "Expected nil or empty map when no validators are tracked")
			},
		},
		{
			name: "allDataColumnSubnets returns all subnets logic test",
			setup: func(t *testing.T) *Service {
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

				return &Service{
					ctx:                    ctx,
					trackedValidatorsCache: tvc,
					cfg: &config{
						stateGen: stateGen,
						beaconDB: db,
					},
				}
			},
			test: func(t *testing.T, svc *Service) {
				dataColumnSidecarSubnetCount := params.BeaconConfig().DataColumnSidecarSubnetCount
				result := svc.allDataColumnSubnets(0)
				assert.Equal(t, dataColumnSidecarSubnetCount, uint64(len(result)))

				for i := range dataColumnSidecarSubnetCount {
					assert.Equal(t, true, result[i])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := tt.setup(t)
			tt.test(t, svc)
		})
	}
}
