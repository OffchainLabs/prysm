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
	record.Set(peerdas.Csc(custodyCount))
	record.Set(enode.Secp256k1(privateKey.PublicKey))

	return record, peerID, privateKey
}

func TestDataColumnsAdmissibleCustodyPeers(t *testing.T) {
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

	// Peer 1 custodies exactly the same columns than us.
	// (We use the same keys pair than ours for simplicity)
	peer1Record, peer1ID, localPrivateKey := createPeer(t, 1, custodyRequirement)

	// Peer 2 custodies all the columns.
	peer2Record, peer2ID, _ := createPeer(t, 2, dataColumnSidecarSubnetCount)

	// Peer 3 custodies different columns than us (but the same count).
	// (We use the same public key than peer 2 for simplicity)
	peer3Record, peer3ID, _ := createPeer(t, 3, custodyRequirement)

	// Peer 4 custodies less columns than us.
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

	actual, err := service.DataColumnsAdmissibleCustodyPeers([]peer.ID{peer1ID, peer2ID, peer3ID, peer4ID})
	require.NoError(t, err)

	expected := []peer.ID{peer1ID, peer2ID}
	require.DeepSSZEqual(t, expected, actual)
}

func TestDataColumnsCustodyCountFromRemotePeer(t *testing.T) {
	const (
		expectedENR      uint64 = 7
		expectedMetadata uint64 = 8
		pid                     = "test-id"
	)

	csc := peerdas.Csc(expectedENR)

	// Define a nil record
	var nilRecord *enr.Record = nil

	// Define an empty record (record with non `csc` entry)
	emptyRecord := &enr.Record{}

	// Define a nominal record
	nominalRecord := &enr.Record{}
	nominalRecord.Set(csc)

	// Define a metadata with zero custody.
	zeroMetadata := wrapper.WrappedMetadataV2(&pb.MetaDataV2{
		CustodySubnetCount: 0,
	})

	// Define a nominal metadata.
	nominalMetadata := wrapper.WrappedMetadataV2(&pb.MetaDataV2{
		CustodySubnetCount: expectedMetadata,
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
			actual := service.DataColumnsCustodyCountFromRemotePeer(pid)

			// Verify the result.
			require.Equal(t, tc.expected, actual)
		})
	}

}
