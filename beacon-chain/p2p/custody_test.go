package p2p

import (
	"context"
	"crypto/ecdsa"
	"net"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/peers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/peers/scorers"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/wrapper"
	ecdsaprysm "github.com/prysmaticlabs/prysm/v5/crypto/ecdsa"
	prysmNetwork "github.com/prysmaticlabs/prysm/v5/network"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/metadata"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func createPeer(t *testing.T, privateKeyOffset int, custodyCount uint64) (*enr.Record, peer.ID, *ecdsa.PrivateKey) {
	privateKeyBytes := make([]byte, 32)
	for i := 0; i < 32; i++ {
		privateKeyBytes[i] = byte(privateKeyOffset + i)
	}

	unmarshalledPrivateKey, err := crypto.UnmarshalSecp256k1PrivateKey(privateKeyBytes)
	require.NoError(t, err)

	privateKey, err := ecdsaprysm.ConvertFromInterfacePrivKey(unmarshalledPrivateKey)
	require.NoError(t, err)

	peerID, err := peer.IDFromPrivateKey(unmarshalledPrivateKey)
	require.NoError(t, err)

	record := &enr.Record{}
	record.Set(peerdas.Cgc(custodyCount))
	record.Set(enode.Secp256k1(privateKey.PublicKey))

	return record, peerID, privateKey
}

func TestAdmissibleCustodyGroupsPeers(t *testing.T) {
	genesisValidatorRoot := make([]byte, 32)

	for i := 0; i < 32; i++ {
		genesisValidatorRoot[i] = byte(i)
	}

	service := &Service{
		cfg:                   &Config{},
		genesisTime:           time.Now(),
		genesisValidatorsRoot: genesisValidatorRoot,
		peers: peers.NewStatus(context.Background(), &peers.StatusConfig{
			ScorerParams: &scorers.Config{},
		}),
	}

	ipAddrString, err := prysmNetwork.ExternalIPv4()
	require.NoError(t, err)
	ipAddr := net.ParseIP(ipAddrString)

	custodyRequirement := params.BeaconConfig().CustodyRequirement
	dataColumnSidecarSubnetCount := params.BeaconConfig().DataColumnSidecarSubnetCount

	// Peer 1 custodies exactly the same groups than us.
	// (We use the same keys pair than ours for simplicity)
	peer1Record, peer1ID, localPrivateKey := createPeer(t, 1, custodyRequirement)

	// Peer 2 custodies all the groups.
	peer2Record, peer2ID, _ := createPeer(t, 2, dataColumnSidecarSubnetCount)

	// Peer 3 custodies different groups than us (but the same count).
	// (We use the same public key than peer 2 for simplicity)
	peer3Record, peer3ID, _ := createPeer(t, 3, custodyRequirement)

	// Peer 4 custodies less groups than us.
	peer4Record, peer4ID, _ := createPeer(t, 4, custodyRequirement-1)

	createListener := func() (*discover.UDPv5, error) {
		return service.createListener(ipAddr, localPrivateKey)
	}

	listener, err := newListener(createListener)
	require.NoError(t, err)

	service.dv5Listener = listener

	service.peers.Add(peer1Record, peer1ID, nil, network.DirOutbound)
	service.peers.Add(peer2Record, peer2ID, nil, network.DirOutbound)
	service.peers.Add(peer3Record, peer3ID, nil, network.DirOutbound)
	service.peers.Add(peer4Record, peer4ID, nil, network.DirOutbound)

	actual, err := service.AdmissibleCustodyGroupsPeers([]peer.ID{peer1ID, peer2ID, peer3ID, peer4ID})
	require.NoError(t, err)

	expected := []peer.ID{peer1ID, peer2ID}
	require.DeepSSZEqual(t, expected, actual)
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

func TestAdmissiblePeersForDataColumns(t *testing.T) {
	type testCase struct {
		name              string
		neededDataColumns map[uint64]bool
		expectedPeerMap   map[peer.ID]map[uint64]bool
		expectedColumnMap map[uint64][]peer.ID
	}

	genesisValidatorRoot := make([]byte, 32)
	for i := 0; i < 32; i++ {
		genesisValidatorRoot[i] = byte(i)
	}

	service := &Service{
		cfg:                   &Config{},
		genesisTime:           time.Now(),
		genesisValidatorsRoot: genesisValidatorRoot,
		peers: peers.NewStatus(context.Background(), &peers.StatusConfig{
			ScorerParams: &scorers.Config{},
		}),
	}

	custodyRequirement := params.BeaconConfig().CustodyRequirement
	// Helper function to create a peer with metadata and add it to the service
	createPeerWithMetadata := func(t *testing.T, id int, custodyCount uint64) (peer.ID, *enr.Record) {
		peerRecord, peerID, _ := createPeer(t, id, custodyCount)
		peerMetadata := wrapper.WrappedMetadataV2(&pb.MetaDataV2{
			CustodyGroupCount: custodyCount,
		})
		service.peers.Add(peerRecord, peerID, nil, network.DirOutbound)
		service.peers.SetMetadata(peerID, peerMetadata)
		return peerID, peerRecord
	}

	// Create 7 peers with metadata
	peer1ID, _ := createPeerWithMetadata(t, 1, custodyRequirement)
	peer2ID, _ := createPeerWithMetadata(t, 2, custodyRequirement)
	peer3ID, _ := createPeerWithMetadata(t, 3, custodyRequirement)
	peer4ID, _ := createPeerWithMetadata(t, 4, custodyRequirement)
	peer5ID, _ := createPeerWithMetadata(t, 5, custodyRequirement)

	// Create peers with overlapping columns. Peers 6 and 7 happen to have an
	// overlapping column.
	peer6ID, _ := createPeerWithMetadata(t, 6, custodyRequirement)
	peer7ID, _ := createPeerWithMetadata(t, 7, custodyRequirement)

	// List of peers to check
	peerList := []peer.ID{peer1ID, peer2ID, peer3ID, peer4ID, peer5ID, peer6ID, peer7ID}

	// Hardcoded overlapping column - from diagnostic output column 109 is custodied by two peers
	overlappingColumn := uint64(109)

	// Define test cases with hardcoded expected values based on diagnostic output
	tests := []testCase{
		{
			name: "Request columns 0-9",
			neededDataColumns: func() map[uint64]bool {
				columns := make(map[uint64]bool)
				for i := uint64(0); i < 10; i++ {
					columns[i] = true
				}
				return columns
			}(),
			// Hardcoded values from diagnostic output - only peer1 custodies column 6 in range 0-9
			expectedPeerMap: map[peer.ID]map[uint64]bool{
				peer1ID: {6: true, 37: true, 48: true, 113: true},
			},
			expectedColumnMap: map[uint64][]peer.ID{
				6: {peer1ID},
			},
		},
		{
			name: "Request specific columns",
			neededDataColumns: map[uint64]bool{
				6:   true, // custodied by peer1
				35:  true, // custodied by peer2
				48:  true, // custodied by peer1
				113: true, // custodied by peer1
			},
			// Values from diagnostic output
			expectedPeerMap: map[peer.ID]map[uint64]bool{
				peer1ID: {6: true, 37: true, 48: true, 113: true},
				peer2ID: {35: true, 79: true, 92: true, 109: true},
			},
			expectedColumnMap: map[uint64][]peer.ID{
				6:   {peer1ID},
				35:  {peer2ID},
				48:  {peer1ID},
				113: {peer1ID},
			},
		},
		{
			name: "Request columns no peer custodies",
			neededDataColumns: map[uint64]bool{
				1000: true, // Use a column number that's guaranteed to be out of range
				1001: true,
				1002: true,
				1003: true,
			},
			// When no peer custodies the requested columns, empty maps are returned
			expectedPeerMap:   map[peer.ID]map[uint64]bool{},
			expectedColumnMap: map[uint64][]peer.ID{},
		},
		{
			name: "Multiple peers custody same column",
			neededDataColumns: map[uint64]bool{
				overlappingColumn: true, // Column 109 is custodied by peer2 and peer7
			},
			// Values from diagnostic output
			expectedPeerMap: map[peer.ID]map[uint64]bool{
				peer2ID: {35: true, 79: true, 92: true, 109: true},
				peer7ID: {40: true, 59: true, 94: true, 109: true},
			},
			expectedColumnMap: map[uint64][]peer.ID{
				overlappingColumn: {peer2ID, peer7ID},
			},
		},
		{
			name: "Mix of covered and uncovered columns",
			neededDataColumns: map[uint64]bool{
				6:    true, // covered by peer1
				35:   true, // covered by peer2
				1000: true, // not covered by any peer (out of range)
				113:  true, // covered by peer1
			},
			// Values from diagnostic output
			expectedPeerMap: map[peer.ID]map[uint64]bool{
				peer1ID: {6: true, 37: true, 48: true, 113: true},
				peer2ID: {35: true, 79: true, 92: true, 109: true},
			},
			expectedColumnMap: map[uint64][]peer.ID{
				6:   {peer1ID},
				35:  {peer2ID},
				113: {peer1ID},
			},
		},
		{
			name:              "Empty request",
			neededDataColumns: map[uint64]bool{},
			expectedPeerMap:   map[peer.ID]map[uint64]bool{},
			expectedColumnMap: map[uint64][]peer.ID{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function we want to test
			admissiblePeersByDataColumn, dataColumnsByAdmissiblePeer, _, err := service.AdmissiblePeersForDataColumns(
				peerList,
				tc.neededDataColumns,
			)
			require.NoError(t, err)

			for peerID, expectedColumns := range tc.expectedPeerMap {
				actualColumns, exists := admissiblePeersByDataColumn[peerID]
				require.Equal(t, true, exists, "Expected peer %s to be in admissiblePeersByDataColumn", peerID)
				require.Equal(t, len(expectedColumns), len(actualColumns), "Column map size mismatch for peer %s", peerID)

				for colID := range expectedColumns {
					_, exists := actualColumns[colID]
					require.Equal(t, expectedColumns[colID], exists,
						"Column %d presence mismatch for peer %s", colID, peerID)
				}
			}

			for colID, expectedPeers := range tc.expectedColumnMap {
				actualPeers, exists := dataColumnsByAdmissiblePeer[colID]
				if len(expectedPeers) == 0 {
					require.Equal(t, false, exists, "Column %d shouldn't be in dataColumnsByAdmissiblePeer", colID)
					continue
				}

				require.Equal(t, true, exists, "Column %d should be in dataColumnsByAdmissiblePeer", colID)
				require.Equal(t, len(expectedPeers), len(actualPeers),
					"Peer list size mismatch for column %d", colID)

				for _, expectedPeer := range expectedPeers {
					found := false
					for _, actualPeer := range actualPeers {
						if expectedPeer == actualPeer {
							found = true
							break
						}
					}
					require.Equal(t, true, found, "Expected peer %s to be in peers list for column %d", expectedPeer, colID)
				}
			}

			// Ensure only needed columns are returned in dataColumnsByAdmissiblePeer
			for colID := range dataColumnsByAdmissiblePeer {
				_, needed := tc.neededDataColumns[colID]
				require.Equal(t, true, needed,
					"Column %d in dataColumnsByAdmissiblePeer was not in neededDataColumns", colID)
			}
		})
	}
}
