package sync

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestUpdateToAdvertiseCustodyGroupCount(t *testing.T) {
	testCases := []struct {
		name                         string
		topics                       []string
		validatorsCustodyRequirement uint64
		expected                     uint64
	}{
		{
			name:                         "some missing registered topics",
			topics:                       []string{"/eth2/ae729ef4/data_column_sidecar_1"},
			validatorsCustodyRequirement: 9,
			expected:                     4,
		},
		{
			name: "all registered topics",
			topics: []string{
				"/eth2/ae729ef4/data_column_sidecar_1",
				"/eth2/ae729ef4/data_column_sidecar_6",
				"/eth2/ae729ef4/data_column_sidecar_17",
				"/eth2/ae729ef4/data_column_sidecar_19",
				"/eth2/ae729ef4/data_column_sidecar_42",
				"/eth2/ae729ef4/data_column_sidecar_75",
				"/eth2/ae729ef4/data_column_sidecar_87",
				"/eth2/ae729ef4/data_column_sidecar_102",
				"/eth2/ae729ef4/data_column_sidecar_117",
			},
			validatorsCustodyRequirement: 9,
			expected:                     9,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a new subTopicHandler and add the topics.
			subHandler := newSubTopicHandler()
			for _, topic := range tc.topics {
				subHandler.addTopic(topic, nil)
			}

			// Create a new service.
			service := &Service{
				cfg: &config{
					p2p:         &p2p.Service{},
					custodyInfo: &peerdas.CustodyInfo{},
				},
				subHandler: subHandler,
			}

			// Set the target custody group count.
			service.cfg.custodyInfo.TargetGroupCount.SetValidatorsCustodyRequirement(tc.validatorsCustodyRequirement)

			// Update the custody group count to advertise.
			service.updateToAdvertiseCustodyGroupCount()

			// Get the custody group count to advertise.
			actual := service.cfg.custodyInfo.ToAdvertiseGroupCount.Get()

			// Check if the custody group count to advertise is as expected.
			require.Equal(t, tc.expected, actual)
		})
	}

}

func TestSetTargetValidatorsCustodyRequirement(t *testing.T) {
	testCases := []struct {
		name                            string
		latestProcessedEpoch            primitives.Epoch
		validatorsBalance               []uint64
		expectedTargetCustodyGroupCount uint64
	}{
		{
			name:                            "no tracked validators",
			latestProcessedEpoch:            0,
			expectedTargetCustodyGroupCount: 4,
		},
		{
			name:                            "some tracked validators",
			latestProcessedEpoch:            0,
			validatorsBalance:               []uint64{64_000_000_000, 64_000_000_000, 64_000_000_000, 64_000_000_000, 33_000_000_000},
			expectedTargetCustodyGroupCount: 9,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			beaconDB := testDB.SetupDB(t)
			stateGen := stategen.New(beaconDB, doublylinkedtree.New())
			state, _ := util.DeterministicGenesisState(t, 32)
			err := state.SetBalances(tc.validatorsBalance)
			require.NoError(t, err)

			stateGen.SaveFinalizedState(0, [32]byte{}, state)

			service := &Service{
				trackedValidatorsCache: cache.NewTrackedValidatorsCache(),
				cfg: &config{
					stateGen:    stateGen,
					custodyInfo: &peerdas.CustodyInfo{},
				},
			}

			for index := range tc.validatorsBalance {
				validator := cache.TrackedValidator{
					Active: true,
					Index:  primitives.ValidatorIndex(index),
				}

				service.trackedValidatorsCache.Set(validator)
			}

			service.setTargetValidatorsCustodyRequirement()

			actualTargetCustodyGroup := service.cfg.custodyInfo.TargetGroupCount.Get()
			require.Equal(t, tc.expectedTargetCustodyGroupCount, actualTargetCustodyGroup)
		})
	}
}

func TestExtractGossipMessage(t *testing.T) {
	testCases := []struct {
		name     string
		expected string
	}{
		{
			name:     "/eth2/ae729ef4/beacon_attestation_28/ssz_snappy",
			expected: "beacon_attestation_28",
		},
		{
			name:     "/eth2/ae729ef4/beacon_attestation_28",
			expected: "beacon_attestation_28",
		},
		{
			name:     "/eth2/ae729ef4",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := extractGossipMessage(tc.name)
			require.Equal(t, tc.expected, actual)
		})
	}
}
