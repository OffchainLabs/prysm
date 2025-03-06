package sync

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	kzg "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/peers"
	p2ptest "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
	p2pTypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/wrapper"
	ecdsaprysm "github.com/prysmaticlabs/prysm/v5/crypto/ecdsa"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/sirupsen/logrus"
)

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

	service, err := p2p.NewService(context.Background(), &p2p.Config{})
	require.NoError(t, err)

	custodyRequirement := params.BeaconConfig().CustodyRequirement
	// Helper function to create a peer with metadata and add it to the service
	createCustodyPeerWithMetadata := func(t *testing.T, id int, custodyCount uint64) (peer.ID, *enr.Record) {
		peerRecord, peerID, _ := createCustodyPeer(t, id, custodyCount)
		peerMetadata := wrapper.WrappedMetadataV2(&pb.MetaDataV2{
			CustodyGroupCount: custodyCount,
		})
		service.Peers().Add(peerRecord, peerID, nil, network.DirOutbound)
		service.Peers().SetMetadata(peerID, peerMetadata)
		return peerID, peerRecord
	}

	// Create 7 peers with metadata
	peer1ID, _ := createCustodyPeerWithMetadata(t, 1, custodyRequirement)
	peer2ID, _ := createCustodyPeerWithMetadata(t, 2, custodyRequirement)
	peer3ID, _ := createCustodyPeerWithMetadata(t, 3, custodyRequirement)
	peer4ID, _ := createCustodyPeerWithMetadata(t, 4, custodyRequirement)
	peer5ID, _ := createCustodyPeerWithMetadata(t, 5, custodyRequirement)

	// Create peers with overlapping columns. Peers 6 and 7 happen to have an
	// overlapping column.
	peer6ID, _ := createCustodyPeerWithMetadata(t, 6, custodyRequirement)
	peer7ID, _ := createCustodyPeerWithMetadata(t, 7, custodyRequirement)

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
			admissiblePeersByDataColumn, dataColumnsByAdmissiblePeer, _, err := AdmissiblePeersForDataColumns(
				peerList,
				tc.neededDataColumns,
				service,
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

func TestSelectPeersToFetchDataColumnsFrom(t *testing.T) {
	testCases := []struct {
		name string

		// Inputs
		neededDataColumns map[uint64]bool
		dataColumnsByPeer map[peer.ID]map[uint64]bool

		// Expected outputs
		dataColumnsToFetchByPeer map[peer.ID][]uint64
		err                      error
	}{
		{
			name:              "no data columns needed",
			neededDataColumns: map[uint64]bool{},
			dataColumnsByPeer: map[peer.ID]map[uint64]bool{
				peer.ID("peer1"): {1: true, 2: true},
				peer.ID("peer2"): {3: true, 4: true},
			},
			dataColumnsToFetchByPeer: map[peer.ID][]uint64{},
			err:                      nil,
		},
		{
			name:              "one peer has all data columns needed",
			neededDataColumns: map[uint64]bool{1: true, 3: true, 5: true},
			dataColumnsByPeer: map[peer.ID]map[uint64]bool{
				peer.ID("peer1"): {2: true, 4: true},
				peer.ID("peer2"): {1: true, 3: true, 5: true, 7: true, 9: true},
				peer.ID("peer3"): {6: true},
			},
			dataColumnsToFetchByPeer: map[peer.ID][]uint64{
				peer.ID("peer2"): {1, 3, 5},
			},
			err: nil,
		},
		{
			name:              "multiple peers are needed - 1",
			neededDataColumns: map[uint64]bool{1: true, 3: true, 5: true, 7: true, 9: true},
			dataColumnsByPeer: map[peer.ID]map[uint64]bool{
				peer.ID("peer1"): {3: true, 7: true},
				peer.ID("peer2"): {1: true, 3: true, 5: true, 9: true, 10: true},
				peer.ID("peer3"): {6: true, 10: true, 12: true, 14: true, 16: true, 18: true, 20: true},
				peer.ID("peer4"): {9: true},
			},
			dataColumnsToFetchByPeer: map[peer.ID][]uint64{
				peer.ID("peer2"): {1, 3, 5, 9},
				peer.ID("peer1"): {7},
			},
			err: nil,
		},
		{
			name:              "multiple peers are needed - 2",
			neededDataColumns: map[uint64]bool{1: true, 3: true, 5: true, 7: true, 9: true},
			dataColumnsByPeer: map[peer.ID]map[uint64]bool{
				peer.ID("peer1"): {9: true, 10: true},
				peer.ID("peer2"): {3: true, 7: true},
				peer.ID("peer3"): {1: true, 5: true},
			},
			dataColumnsToFetchByPeer: map[peer.ID][]uint64{
				peer.ID("peer1"): {9},
				peer.ID("peer2"): {3, 7},
				peer.ID("peer3"): {1, 5},
			},
			err: nil,
		},
		{
			name:              "some columns are not owned by any peer",
			neededDataColumns: map[uint64]bool{1: true, 3: true, 5: true, 7: true, 9: true},
			dataColumnsByPeer: map[peer.ID]map[uint64]bool{
				peer.ID("peer1"): {9: true, 10: true},
				peer.ID("peer2"): {2: true, 6: true},
				peer.ID("peer3"): {1: true, 5: true},
			},
			dataColumnsToFetchByPeer: map[peer.ID][]uint64{
				peer.ID("peer1"): {9},
				peer.ID("peer3"): {1, 5},
			},
			err: errors.New("no peer to fetch the following data columns: [3 7]"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := SelectPeersToFetchDataColumnsFrom(tc.neededDataColumns, tc.dataColumnsByPeer)

			if tc.err != nil {
				require.Equal(t, tc.err.Error(), err.Error())
			} else {
				require.NoError(t, err)
			}

			expected := tc.dataColumnsToFetchByPeer
			require.Equal(t, len(expected), len(actual))

			for peerID, expectedDataColumns := range expected {
				actualDataColumns, ok := actual[peerID]
				require.Equal(t, true, ok)
				require.DeepSSZEqual(t, expectedDataColumns, actualDataColumns)
			}
		})
	}
}

type peerSetup struct {
	offset            int
	custodyGroupCount uint64
}

func TestRequestDataColumnSidecars(t *testing.T) {
	const (
		blobsCount = 6
	)

	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	// Set up test environment
	log.Logger.SetOutput(os.Stdout)
	log.Logger.SetLevel(logrus.DebugLevel)
	params.BeaconConfig().FuluForkEpoch = 1
	chainService, clock := defaultMockChain(t)
	p2pService := p2ptest.NewTestP2P(t)

	// Create test block with blobs
	pbSignedBeaconBlock := util.NewBeaconBlockDeneb()
	blockSlot := primitives.Slot(100)
	pbSignedBeaconBlock.Block.Slot = blockSlot

	blobs := make([]kzg.Blob, blobsCount)
	blobKzgCommitments := make([][]byte, blobsCount)

	for j := range blobs {
		blob := getRandBlob(int64(j))
		blobs[j] = blob

		blobKzgCommitment, err := kzg.BlobToKZGCommitment(&blob)
		require.NoError(t, err)

		blobKzgCommitments[j] = blobKzgCommitment[:]
	}

	pbSignedBeaconBlock.Block.Body.BlobKzgCommitments = blobKzgCommitments

	signedBlock, err := blocks.NewSignedBeaconBlock(pbSignedBeaconBlock)
	require.NoError(t, err)

	dataColumnSidecars, err := peerdas.DataColumnSidecars(signedBlock, blobs)
	require.NoError(t, err)

	// Calculate block root
	blockRoot, err := signedBlock.Block().HashTreeRoot()
	require.NoError(t, err)

	testCases := []struct {
		name string
		// Test inputs
		dataColumns map[uint64]bool
		peerSetup   []peerSetup
		expectError bool
	}{
		{
			name:        "No data columns requested",
			dataColumns: map[uint64]bool{},
			peerSetup: []peerSetup{
				{offset: 1, custodyGroupCount: 4},
			},
			expectError: false,
		},
		{
			name:        "Single data column successful request",
			dataColumns: map[uint64]bool{37: true},
			peerSetup: []peerSetup{
				{offset: 1, custodyGroupCount: 4}, // This peer will custody columns [6, 37, 48, 113]
			},
			expectError: false,
		},
		{
			name:        "Multiple data columns successful request",
			dataColumns: map[uint64]bool{37: true, 28: true},
			peerSetup: []peerSetup{
				{offset: 1, custodyGroupCount: 4},  // This peer will custody columns [6, 37, 48, 113]
				{offset: 10, custodyGroupCount: 4}, // This peer will custody columns [6, 28, 53, 71]
			},
			expectError: false,
		},
		{
			name:        "No peers respond",
			dataColumns: map[uint64]bool{37: true},
			peerSetup:   []peerSetup{}, // No peers
			expectError: true,
		},
		{
			name:        "No peer has the requested column",
			dataColumns: map[uint64]bool{1000: true}, // Column that no peer will have
			peerSetup:   []peerSetup{},               // No peers
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create responding peers with deterministic peer IDs
			peerIDs := make([]core.PeerID, 0, len(tc.peerSetup))
			for _, setup := range tc.peerSetup {
				// Create the private key, depending on the offset
				peer := createAndConnectCustodyPeer(t, setup, dataColumnSidecars, chainService, p2pService)

				peerIDs = append(peerIDs, peer.PeerID())
			}

			ctxMap := map[[4]byte]int{{245, 165, 253, 66}: version.Fulu}
			verifier := func(cols []blocks.RODataColumn, reqs []verification.Requirement) verification.DataColumnsVerifier {
				initializer := &verification.Initializer{}
				return initializer.NewDataColumnsVerifier(cols, reqs)
			}

			// Call the function under test
			responseCols, err := RequestDataColumnSidecars(
				context.Background(),
				tc.dataColumns,
				signedBlock,
				blockRoot,
				peerIDs,
				clock,
				p2pService,
				ctxMap,
				verifier,
			)

			// Verify results
			if tc.expectError {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)

			require.Equal(t, len(tc.dataColumns), len(responseCols))
			expectedColumns := make([]uint64, 0, len(tc.dataColumns))
			for col := range tc.dataColumns {
				expectedColumns = append(expectedColumns, col)
			}

			fmt.Println("tc.dataColumns", tc.dataColumns)
			fmt.Println("expectedColumns", expectedColumns)

			for i := range responseCols {
				require.Equal(t, expectedColumns[i], responseCols[i].DataColumnSidecar.ColumnIndex)
			}
		})
	}
}

// createAndConnectCustodyPeer creates a new peer with a deterministic private key and connects it to the p2p service.
// It then sets up the peer to respond with data columns it custodies.
func createAndConnectCustodyPeer(t *testing.T, setup peerSetup, dataColumnSidecars []*eth.DataColumnSidecar, chainService *mock.ChainService, p2pService *p2ptest.TestP2P) *p2ptest.TestP2P {
	privateKeyBytes := make([]byte, 32)
	for i := 0; i < 32; i++ {
		privateKeyBytes[i] = byte(setup.offset + i)
	}

	privateKey, err := crypto.UnmarshalSecp256k1PrivateKey(privateKeyBytes)
	require.NoError(t, err)

	// Create the peer
	peer := p2ptest.NewTestP2P(t, libp2p.Identity(privateKey))

	// Set up the peer to respond with data columns it custodies
	peer.SetStreamHandler(p2p.RPCDataColumnSidecarsByRootTopicV1+"/ssz_snappy", func(stream network.Stream) {
		// Decode the request
		req := new(p2pTypes.DataColumnSidecarsByRootReq)
		if err := peer.Encoding().DecodeWithMaxLength(stream, req); err != nil {
			log.WithError(err).Error("Failed to decode request")
			closeStream(stream, log)
			return
		}

		// The test peers have peer.EnodeID set to zero. Derive the enode ID from the peer ID instead.
		enodeID, err := p2p.ConvertPeerIDToNodeID(peer.PeerID())
		if err != nil {
			log.WithError(err).Error("Failed to convert peer ID to enode ID")
			closeStream(stream, log)
			return
		}

		peerInfo, _, err := peerdas.Info(enodeID, setup.custodyGroupCount)
		if err != nil {
			log.WithError(err).Error("Failed to get peer info")
			closeStream(stream, log)
			return
		}

		// For each requested column, check if we custody it and respond if we do
		for _, identifier := range *req {
			// Skip columns that we don't custody
			if !peerInfo.CustodyGroups[identifier.ColumnIndex] {
				log.Debugf("Peer %s does not custody column %d", peer.PeerID(), identifier.ColumnIndex)
				log.Debugf("Peer custody columns: %+v", peerInfo.CustodyColumns)
				log.Debugf("Peer enode id: %s", enodeID)
				continue
			}
			// Send the response
			col := dataColumnSidecars[identifier.ColumnIndex]
			if err := WriteDataColumnSidecarChunk(stream, chainService, p2pService.Encoding(), col); err != nil {
				log.WithError(err).Error("Failed to write data column sidecar chunk")
				closeStream(stream, log)
				return
			}
		}

		// Close the stream
		closeStream(stream, log)
	})

	// Create the record and set the custody count
	enr := &enr.Record{}
	enr.Set(peerdas.Cgc(setup.custodyGroupCount))

	// Add the peer and connect it
	p2pService.Peers().Add(enr, peer.PeerID(), nil, network.DirOutbound)
	p2pService.Peers().SetConnectionState(peer.PeerID(), peers.Connected)
	p2pService.Connect(peer)
	return peer
}

func createCustodyPeer(t *testing.T, privateKeyOffset int, custodyCount uint64) (*enr.Record, peer.ID, *ecdsa.PrivateKey) {
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
