package p2p

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers/scorers"
	testp2p "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/consensus-types/wrapper"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/metadata"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/network"
)

func TestEarliestAvailableSlot(t *testing.T) {
	const expected primitives.Slot = 100

	service := &Service{
		custodyInfoSet: make(chan struct{}),
		custodyInfo: &custodyInfo{
			earliestAvailableSlot: expected,
		},
	}

	close(service.custodyInfoSet)
	slot, err := service.EarliestAvailableSlot(t.Context())

	require.NoError(t, err)
	require.Equal(t, expected, slot)
}

func TestCustodyGroupCount(t *testing.T) {
	const expected uint64 = 5

	service := &Service{
		custodyInfoSet: make(chan struct{}),
		custodyInfo: &custodyInfo{
			groupCount: expected,
		},
	}

	close(service.custodyInfoSet)
	count, err := service.CustodyGroupCount(t.Context())

	require.NoError(t, err)
	require.Equal(t, expected, count)
}

func TestUpdateCustodyInfo(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.SamplesPerSlot = 8
	config.FuluForkEpoch = 10
	params.OverrideBeaconConfig(config)

	testCases := []struct {
		name               string
		initialCustodyInfo *custodyInfo
		inputSlot          primitives.Slot
		inputGroupCount    uint64
		expectedUpdated    bool
		expectedSlot       primitives.Slot
		expectedGroupCount uint64
		expectedErr        string
	}{
		{
			name:               "First time setting custody info",
			initialCustodyInfo: nil,
			inputSlot:          100,
			inputGroupCount:    5,
			expectedUpdated:    true,
			expectedSlot:       100,
			expectedGroupCount: 5,
		},
		{
			name: "Group count decrease - no update",
			initialCustodyInfo: &custodyInfo{
				earliestAvailableSlot: 50,
				groupCount:            10,
			},
			inputSlot:          60,
			inputGroupCount:    8,
			expectedUpdated:    false,
			expectedSlot:       50,
			expectedGroupCount: 10,
		},
		{
			name: "Earliest slot decrease - error",
			initialCustodyInfo: &custodyInfo{
				earliestAvailableSlot: 100,
				groupCount:            5,
			},
			inputSlot:       50,
			inputGroupCount: 10,
			expectedErr:     "earliest available slot 50 is less than the current one 100",
		},
		{
			name: "Group count increase but <= samples per slot",
			initialCustodyInfo: &custodyInfo{
				earliestAvailableSlot: 50,
				groupCount:            5,
			},
			inputSlot:          60,
			inputGroupCount:    8,
			expectedUpdated:    true,
			expectedSlot:       50,
			expectedGroupCount: 8,
		},
		{
			name: "Group count increase > samples per slot, before Fulu fork",
			initialCustodyInfo: &custodyInfo{
				earliestAvailableSlot: 50,
				groupCount:            5,
			},
			inputSlot:          60,
			inputGroupCount:    15,
			expectedUpdated:    true,
			expectedSlot:       50,
			expectedGroupCount: 15,
		},
		{
			name: "Group count increase > samples per slot, after Fulu fork",
			initialCustodyInfo: &custodyInfo{
				earliestAvailableSlot: 50,
				groupCount:            5,
			},
			inputSlot:          500,
			inputGroupCount:    15,
			expectedUpdated:    true,
			expectedSlot:       500,
			expectedGroupCount: 15,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service := &Service{
				custodyInfoSet: make(chan struct{}),
				custodyInfo:    tc.initialCustodyInfo,
			}

			slot, groupCount, err := service.UpdateCustodyInfo(tc.inputSlot, tc.inputGroupCount)

			if tc.expectedErr != "" {
				require.NotNil(t, err)
				require.Equal(t, true, strings.Contains(err.Error(), tc.expectedErr))
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectedSlot, slot)
			require.Equal(t, tc.expectedGroupCount, groupCount)

			if tc.expectedUpdated {
				require.NotNil(t, service.custodyInfo)
				require.Equal(t, tc.expectedSlot, service.custodyInfo.earliestAvailableSlot)
				require.Equal(t, tc.expectedGroupCount, service.custodyInfo.groupCount)
			}
		})
	}
}

func TestUpdateEarliestAvailableSlot(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.FuluForkEpoch = 0 // Enable Fulu from epoch 0
	params.OverrideBeaconConfig(config)

	t.Run("Valid update", func(t *testing.T) {
		const (
			initialSlot primitives.Slot = 50
			newSlot     primitives.Slot = 100
			groupCount  uint64          = 5
		)

		// Set up a scenario where we're far enough in the chain that increasing to newSlot is valid
		minEpochsForBlocks := primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests)
		currentEpoch := minEpochsForBlocks + 100 // Well beyond MIN_EPOCHS_FOR_BLOCK_REQUESTS
		currentSlot := primitives.Slot(currentEpoch) * primitives.Slot(params.BeaconConfig().SlotsPerEpoch)

		service := &Service{
			// Set genesis time in the past so currentSlot is the "current" slot
			genesisTime: time.Now().Add(-time.Duration(currentSlot) * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second),
			custodyInfo: &custodyInfo{
				earliestAvailableSlot: initialSlot,
				groupCount:            groupCount,
			},
		}

		err := service.UpdateEarliestAvailableSlot(newSlot)

		require.NoError(t, err)
		require.Equal(t, newSlot, service.custodyInfo.earliestAvailableSlot)
		require.Equal(t, groupCount, service.custodyInfo.groupCount) // Should preserve group count
	})

	t.Run("Earlier slot - allowed for backfill", func(t *testing.T) {
		const initialSlot primitives.Slot = 100
		const earlierSlot primitives.Slot = 50

		service := &Service{
			genesisTime: time.Now(),
			custodyInfo: &custodyInfo{
				earliestAvailableSlot: initialSlot,
				groupCount:            5,
			},
		}

		err := service.UpdateEarliestAvailableSlot(earlierSlot)

		require.NoError(t, err)
		require.Equal(t, earlierSlot, service.custodyInfo.earliestAvailableSlot) // Should decrease for backfill
	})

	t.Run("Prevent increase within MIN_EPOCHS_FOR_BLOCK_REQUESTS - late in chain", func(t *testing.T) {
		// Set current time far enough in the future to have a meaningful MIN_EPOCHS_FOR_BLOCK_REQUESTS period
		minEpochsForBlocks := primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests)
		currentEpoch := minEpochsForBlocks + 100 // Well beyond the minimum
		currentSlot := primitives.Slot(currentEpoch) * primitives.Slot(params.BeaconConfig().SlotsPerEpoch)

		// Calculate the minimum allowed epoch
		minRequiredEpoch := currentEpoch - minEpochsForBlocks
		minRequiredSlot := primitives.Slot(minRequiredEpoch) * primitives.Slot(params.BeaconConfig().SlotsPerEpoch)

		// Try to set earliest slot to a value within the MIN_EPOCHS_FOR_BLOCK_REQUESTS period (should fail)
		attemptedSlot := minRequiredSlot + 1000 // Within the mandatory retention period

		service := &Service{
			genesisTime: time.Now().Add(-time.Duration(currentSlot) * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second),
			custodyInfo: &custodyInfo{
				earliestAvailableSlot: minRequiredSlot - 100, // Current value is before the min required
				groupCount:            5,
			},
		}

		err := service.UpdateEarliestAvailableSlot(attemptedSlot)

		require.NotNil(t, err)
		require.Equal(t, true, strings.Contains(err.Error(), "cannot increase earliest available slot"))
	})

	t.Run("Prevent increase at epoch boundary - slot precision matters", func(t *testing.T) {
		minEpochsForBlocks := primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests)
		currentEpoch := minEpochsForBlocks + 976 // Current epoch
		currentSlot := primitives.Slot(currentEpoch) * primitives.Slot(params.BeaconConfig().SlotsPerEpoch)

		minRequiredEpoch := currentEpoch - minEpochsForBlocks                                                              // = 976
		storedEarliestSlot := primitives.Slot(minRequiredEpoch)*primitives.Slot(params.BeaconConfig().SlotsPerEpoch) - 232 // Before minRequired

		// Try to set earliest to slot 8 of the minRequiredEpoch (should fail with slot comparison)
		attemptedSlot := primitives.Slot(minRequiredEpoch)*primitives.Slot(params.BeaconConfig().SlotsPerEpoch) + 8

		service := &Service{
			genesisTime: time.Now().Add(-time.Duration(currentSlot) * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second),
			custodyInfo: &custodyInfo{
				earliestAvailableSlot: storedEarliestSlot,
				groupCount:            5,
			},
		}

		err := service.UpdateEarliestAvailableSlot(attemptedSlot)

		require.NotNil(t, err, "Should prevent increasing earliest slot beyond the minimum required SLOT (not just epoch)")
		require.Equal(t, true, strings.Contains(err.Error(), "cannot increase earliest available slot"))
	})

	t.Run("Prevent increase within MIN_EPOCHS_FOR_BLOCK_REQUESTS - early in chain", func(t *testing.T) {
		minEpochsForBlocks := primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests)
		currentEpoch := minEpochsForBlocks - 10 // Early in chain, BEFORE we have MIN_EPOCHS_FOR_BLOCK_REQUESTS of history
		currentSlot := primitives.Slot(currentEpoch) * primitives.Slot(params.BeaconConfig().SlotsPerEpoch)

		// Current earliest slot is at slot 100
		currentEarliestSlot := primitives.Slot(100)

		// Try to increase earliest slot to slot 1000 (which would be within the mandatory window from currentSlot)
		attemptedSlot := primitives.Slot(1000)

		service := &Service{
			genesisTime: time.Now().Add(-time.Duration(currentSlot) * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second),
			custodyInfo: &custodyInfo{
				earliestAvailableSlot: currentEarliestSlot,
				groupCount:            5,
			},
		}

		err := service.UpdateEarliestAvailableSlot(attemptedSlot)

		require.NotNil(t, err, "Should prevent increasing earliest slot within the mandatory retention window, even early in chain")
		require.Equal(t, true, strings.Contains(err.Error(), "cannot increase earliest available slot"))
	})

	t.Run("Nil custody info - should return error", func(t *testing.T) {
		service := &Service{
			genesisTime: time.Now(),
			custodyInfo: nil, // No custody info set
		}

		err := service.UpdateEarliestAvailableSlot(100)

		require.NotNil(t, err)
		require.Equal(t, true, strings.Contains(err.Error(), "no custody info available"))
	})
}

func TestCustodyGroupCountFromPeer(t *testing.T) {
	const (
		expectedENR      uint64 = 7
		expectedMetadata uint64 = 8
		pid                     = "test-id"
	)

	cgc := peerdas.Cgc(expectedENR)

	// Define a nil record
	var nilRecord *enr.Record = nil

	// Define an empty record (record with non `cgc` entry)
	emptyRecord := &enr.Record{}

	// Define a nominal record
	nominalRecord := &enr.Record{}
	nominalRecord.Set(cgc)

	// Define a metadata with zero custody.
	zeroMetadata := wrapper.WrappedMetadataV2(&pb.MetaDataV2{
		CustodyGroupCount: 0,
	})

	// Define a nominal metadata.
	nominalMetadata := wrapper.WrappedMetadataV2(&pb.MetaDataV2{
		CustodyGroupCount: expectedMetadata,
	})

	testCases := []struct {
		name     string
		record   *enr.Record
		metadata metadata.Metadata
		expected uint64
	}{
		{
			name:     "No metadata - No ENR",
			record:   nilRecord,
			expected: params.BeaconConfig().CustodyRequirement,
		},
		{
			name:     "No metadata - Empty ENR",
			record:   emptyRecord,
			expected: params.BeaconConfig().CustodyRequirement,
		},
		{
			name:     "No Metadata - ENR",
			record:   nominalRecord,
			expected: expectedENR,
		},
		{
			name:     "Metadata with 0 value - ENR",
			record:   nominalRecord,
			metadata: zeroMetadata,
			expected: expectedENR,
		},
		{
			name:     "Metadata - ENR",
			record:   nominalRecord,
			metadata: nominalMetadata,
			expected: expectedMetadata,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create peers status.
			peers := peers.NewStatus(t.Context(), &peers.StatusConfig{
				ScorerParams: &scorers.Config{},
			})

			// Set the metadata.
			if tc.metadata != nil {
				peers.SetMetadata(pid, tc.metadata)
			}

			// Add a new peer with the record.
			peers.Add(tc.record, pid, nil, network.DirOutbound)

			// Create a new service.
			service := &Service{
				peers:    peers,
				metaData: tc.metadata,
				host:     testp2p.NewTestP2P(t).Host(),
			}

			// Retrieve the custody count from the remote peer.
			actual := service.CustodyGroupCountFromPeer(pid)

			// Verify the result.
			require.Equal(t, tc.expected, actual)
		})
	}

}

func TestCustodyGroupCountFromPeerENR(t *testing.T) {
	const (
		expectedENR uint64 = 7
		pid                = "test-id"
	)

	cgc := peerdas.Cgc(expectedENR)
	custodyRequirement := params.BeaconConfig().CustodyRequirement

	testCases := []struct {
		name     string
		record   *enr.Record
		expected uint64
		wantErr  bool
	}{
		{
			name:     "No ENR record",
			record:   nil,
			expected: custodyRequirement,
		},
		{
			name:     "Empty ENR record",
			record:   &enr.Record{},
			expected: custodyRequirement,
		},
		{
			name: "Valid ENR with custody group count",
			record: func() *enr.Record {
				record := &enr.Record{}
				record.Set(cgc)
				return record
			}(),
			expected: expectedENR,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			peers := peers.NewStatus(context.Background(), &peers.StatusConfig{
				ScorerParams: &scorers.Config{},
			})

			if tc.record != nil {
				peers.Add(tc.record, pid, nil, network.DirOutbound)
			}

			service := &Service{
				peers: peers,
				host:  testp2p.NewTestP2P(t).Host(),
			}

			actual := service.custodyGroupCountFromPeerENR(pid)
			require.Equal(t, tc.expected, actual)
		})
	}
}
