package p2p

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers/scorers"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/wrapper"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/metadata"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/network"
)

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
			peers := peers.NewStatus(context.Background(), &peers.StatusConfig{
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
			}

			// Retrieve the custody count from the remote peer.
			actual := service.CustodyGroupCountFromPeer(pid)

			// Verify the result.
			require.Equal(t, tc.expected, actual)
		})
	}

}
