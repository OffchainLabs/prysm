package sync

import (
	"context"
	"fmt"
	"os"
	"testing"

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
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/sirupsen/logrus"
)

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

func createTestDataColumn(columnIndex uint64, header *eth.SignedBeaconBlockHeader) (blocks.RODataColumn, error) {
	return blocks.NewRODataColumn(&eth.DataColumnSidecar{
		ColumnIndex:       columnIndex,
		SignedBlockHeader: header,
		KzgCommitmentsInclusionProof: [][]byte{
			make([]byte, 32),
			make([]byte, 32),
			make([]byte, 32),
			make([]byte, 32),
		},
	})
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
