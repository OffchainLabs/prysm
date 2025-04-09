package sync

import (
	"context"
	"crypto/ecdsa"
	"sort"
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
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
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
	params.SetupTestConfigCleanup(t)
	originalConfig := params.BeaconConfig().Copy()

	testCases := []struct {
		name string

		// Inputs
		neededDataColumns map[uint64]bool
		dataColumnsByPeer map[peer.ID]map[uint64]bool

		// Expected outputs
		dataColumnsToFetchByPeer map[peer.ID][]uint64
		err                      error

		// Optional test configuration
		maxRequestDataColumnSidecars uint64
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
		{
			name:              "respects MaxRequestDataColumnSidecars limit",
			neededDataColumns: map[uint64]bool{1: true, 2: true, 3: true, 4: true},
			dataColumnsByPeer: map[peer.ID]map[uint64]bool{
				peer.ID("peer1"): {1: true, 2: true, 3: true, 4: true},
				peer.ID("peer2"): {3: true, 4: true}, // Duplicate peer for remaining columns
			},
			dataColumnsToFetchByPeer: map[peer.ID][]uint64{
				peer.ID("peer1"): {1, 2}, // First request limited to 2 columns
				peer.ID("peer2"): {3, 4}, // Second request with remaining columns from different peer
			},
			err:                          nil,
			maxRequestDataColumnSidecars: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Start each test case with a fresh copy of the original config
			cfg := originalConfig.Copy()
			if tc.maxRequestDataColumnSidecars > 0 {
				cfg.MaxRequestDataColumnSidecars = tc.maxRequestDataColumnSidecars
			}
			params.OverrideBeaconConfig(cfg)

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

// requestTracker is used to track requests made to peers during tests
type requestTracker struct {
	requests map[int][]uint64 // maps peer offset to requested columns
}

func newRequestTracker() *requestTracker {
	return &requestTracker{
		requests: make(map[int][]uint64),
	}
}

func (rt *requestTracker) trackRequest(offset int, columns []uint64) {
	rt.requests[offset] = columns
}

func TestRequestDataColumnSidecarsByRoot(t *testing.T) {
	const blobsCount = 6

	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	// Configure forks for testing with proper cleanup
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	chainService, clock := defaultMockChain(t)

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

	roBlock, err := blocks.NewROBlock(signedBlock)
	require.NoError(t, err)

	cellsAndProofs := util.GenerateCellsAndProofs(t, blobs)
	dataColumnSidecars, err := peerdas.DataColumnSidecars(signedBlock, cellsAndProofs)
	require.NoError(t, err)

	testCases := []struct {
		name                 string
		dataColumns          map[uint64]bool
		peerSetup            []peerSetup
		expectError          bool
		expectedPeerRequests map[int][]uint64
		skipColumns          map[int]map[uint64]bool // Maps peer offset -> column index -> should skip
	}{
		{
			name:        "No data columns requested",
			dataColumns: map[uint64]bool{},
			peerSetup: []peerSetup{
				{offset: 1, custodyGroupCount: 4},
			},
			expectError:          false,
			expectedPeerRequests: map[int][]uint64{},
		},
		{
			name:        "Single data column successful request",
			dataColumns: map[uint64]bool{37: true},
			peerSetup: []peerSetup{
				{offset: 1, custodyGroupCount: 4}, // This peer will custody columns [6, 37, 48, 113]
			},
			expectError: false,
			expectedPeerRequests: map[int][]uint64{
				1: {37},
			},
		},
		{
			name:        "Multiple data columns successful request",
			dataColumns: map[uint64]bool{37: true, 28: true},
			peerSetup: []peerSetup{
				{offset: 1, custodyGroupCount: 4},  // This peer will custody columns [6, 37, 48, 113]
				{offset: 10, custodyGroupCount: 4}, // This peer will custody columns [6, 28, 53, 71]
			},
			expectError: false,
			expectedPeerRequests: map[int][]uint64{
				1:  {37},
				10: {28},
			},
		},
		{
			name:                 "No peers respond",
			dataColumns:          map[uint64]bool{37: true},
			peerSetup:            []peerSetup{}, // No peers
			expectError:          true,
			expectedPeerRequests: map[int][]uint64{},
		},
		{
			name:                 "No peer has the requested column",
			dataColumns:          map[uint64]bool{1000: true}, // Column that no peer will have
			peerSetup:            []peerSetup{},               // No peers
			expectError:          true,
			expectedPeerRequests: map[int][]uint64{},
		},
		{
			name: "Multiple peers with overlapping custody",
			dataColumns: map[uint64]bool{
				6:   true, // Column custodied by both peers
				37:  true, // Column custodied by peer 1 only
				113: true, // Column custodied by peer 1 only
				28:  true, // Column custodied by peer 2 only
			},
			peerSetup: []peerSetup{
				{offset: 1, custodyGroupCount: 4},  // Peer 1 custodies [6, 37, 48, 113]
				{offset: 10, custodyGroupCount: 4}, // Peer 2 custodies [6, 28, 53, 71]
			},
			expectError: false,
			expectedPeerRequests: map[int][]uint64{
				1:  {6, 37, 113}, // Peer 1 should handle column 6 since it's already getting other columns
				10: {28},         // Peer 2 should only handle column 28
			},
		},
		{
			name: "Mixed valid and invalid columns",
			dataColumns: map[uint64]bool{
				37:   true, // Valid column
				1000: true, // Invalid column
				6:    true, // Valid column
			},
			peerSetup: []peerSetup{
				{offset: 1, custodyGroupCount: 4}, // This peer will custody columns [6, 37, 48, 113]
			},
			expectError:          true,
			expectedPeerRequests: map[int][]uint64{},
		},
		{
			name: "Peer doesn't respond with all claimed columns",
			dataColumns: map[uint64]bool{
				6:  true, // Column that peer claims but won't respond with
				37: true, // Column that peer will respond with
				48: true, // Column that peer claims but won't respond with
			},
			peerSetup: []peerSetup{
				{
					offset:            1,
					custodyGroupCount: 4, // This peer claims custody of [6, 37, 48, 113] but only responds with [37]
				},
			},
			expectError: true, // Should error since not all requested columns were received
			expectedPeerRequests: map[int][]uint64{
				1: {6, 37, 48}, // Peer should be asked for all columns it claims to have
			},
			skipColumns: map[int]map[uint64]bool{
				1: {
					6:  true, // Skip column 6
					48: true, // Skip column 48
				},
			},
		},
		{
			name: "Fallback to other peers when primary peer skips columns",
			dataColumns: map[uint64]bool{
				37: true, // Column that both peers custody (peer with offset 1 will skip)
				48: true, // Column that only peer with offset 1 custodies
			},
			peerSetup: []peerSetup{
				{
					offset:            1,
					custodyGroupCount: 4, // Peer custodies [6, 37, 48, 113]
				},
				{
					offset:            12,
					custodyGroupCount: 4, // Peer custodies [2, 37, 120, 121]
				},
			},
			expectError: false, // Should succeed since peer with offset 12 can provide column 37
			expectedPeerRequests: map[int][]uint64{
				1:  {37, 48}, // First peer is asked for both columns initially
				12: {37},     // Second peer is asked for column 37 as fallback
			},
			skipColumns: map[int]map[uint64]bool{
				1: {
					37: true, // First peer skips column 37, which second peer provides
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hostP2P := p2ptest.NewTestP2P(t)
			tracker := newRequestTracker()

			// Create responding peers with deterministic peer IDs
			peerIDs := make([]core.PeerID, 0, len(tc.peerSetup))
			for _, setup := range tc.peerSetup {
				skipColumnsMap := make(map[uint64]bool)
				if cols, exists := tc.skipColumns[setup.offset]; exists {
					skipColumnsMap = cols
				}
				peerP2P := createAndConnectCustodyPeer(t, setup, dataColumnSidecars, chainService, hostP2P, tracker, skipColumnsMap)
				peerIDs = append(peerIDs, peerP2P.PeerID())
			}

			ctxMap := map[[4]byte]int{{245, 165, 253, 66}: version.Fulu}
			verifier := func(cols []blocks.RODataColumn, reqs []verification.Requirement) verification.DataColumnsVerifier {
				initializer := &verification.Initializer{}
				return initializer.NewDataColumnsVerifier(cols, reqs)
			}

			// Call the function under test
			responseCols, err := RequestDataColumnSidecarsByRoot(
				context.Background(),
				tc.dataColumns,
				roBlock,
				peerIDs,
				clock,
				hostP2P,
				ctxMap,
				verifier,
			)

			// Verify results
			if tc.expectError {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)

			// Verify response columns
			require.Equal(t, len(tc.dataColumns), len(responseCols))
			expectedColumns := make([]uint64, 0, len(tc.dataColumns))
			for col := range tc.dataColumns {
				expectedColumns = append(expectedColumns, col)
			}

			sort.Slice(expectedColumns, func(i, j int) bool {
				return expectedColumns[i] < expectedColumns[j]
			})
			sort.Slice(responseCols, func(i, j int) bool {
				return responseCols[i].DataColumnSidecar.Index < responseCols[j].DataColumnSidecar.Index
			})

			for i := range responseCols {
				require.Equal(t, expectedColumns[i], responseCols[i].DataColumnSidecar.Index)
			}

			// Verify peer request optimization
			require.Equal(t, len(tc.expectedPeerRequests), len(tracker.requests),
				"Number of peers requested from doesn't match expected")

			for offset, expectedCols := range tc.expectedPeerRequests {
				actualCols, exists := tracker.requests[offset]
				require.Equal(t, true, exists, "Expected requests from peer with offset %d", offset)
				require.Equal(t, len(expectedCols), len(actualCols),
					"Number of columns requested from peer with offset %d doesn't match expected", offset)

				// Sort both slices for comparison
				sort.Slice(expectedCols, func(i, j int) bool { return expectedCols[i] < expectedCols[j] })
				sort.Slice(actualCols, func(i, j int) bool { return actualCols[i] < actualCols[j] })
				for i := range expectedCols {
					require.Equal(t, expectedCols[i], actualCols[i],
						"Columns requested from peer with offset %d don't match expected", offset)
				}
			}
		})
	}
}

// createAndConnectCustodyPeer creates a new peer with a deterministic private key and connects it to the p2p service.
// It then sets up the peer to respond with data columns it custodies.
func createAndConnectCustodyPeer(t *testing.T, setup peerSetup, dataColumnSidecars []*pb.DataColumnSidecar, chainService *mock.ChainService, hostP2P *p2ptest.TestP2P, tracker *requestTracker, skipColumns map[uint64]bool) *p2ptest.TestP2P {
	privateKeyBytes := make([]byte, 32)
	for i := range 32 {
		privateKeyBytes[i] = byte(setup.offset + i)
	}

	privateKey, err := crypto.UnmarshalSecp256k1PrivateKey(privateKeyBytes)
	require.NoError(t, err)

	// Create the peerP2P
	peerP2P := p2ptest.NewTestP2P(t, libp2p.Identity(privateKey))

	// Set up the peer to respond with data columns it custodies
	peerP2P.SetStreamHandler(p2p.RPCDataColumnSidecarsByRootTopicV1+"/ssz_snappy", func(stream network.Stream) {
		// Decode the request
		req := new(p2pTypes.DataColumnSidecarsByRootReq)
		if err := peerP2P.Encoding().DecodeWithMaxLength(stream, req); err != nil {
			log.WithError(err).Error("Failed to decode request")
			closeStream(stream, log)
			return
		}

		// Track the request if we have a tracker
		if tracker != nil {
			requestedColumns := make([]uint64, 0, len(*req))
			for _, identifier := range *req {
				requestedColumns = append(requestedColumns, identifier.Index)
			}
			tracker.trackRequest(setup.offset, requestedColumns)
		}

		// Continue with normal response handling
		enodeID, err := p2p.ConvertPeerIDToNodeID(peerP2P.PeerID())
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

		for _, identifier := range *req {
			// Check if this column should be skipped using direct map lookup
			if skipColumns[identifier.Index] {
				continue
			}

			if !peerInfo.CustodyColumns[identifier.Index] {
				continue
			}
			col := dataColumnSidecars[identifier.Index]
			if err := WriteDataColumnSidecarChunk(stream, chainService, peerP2P.Encoding(), col); err != nil {
				log.WithError(err).Error("Failed to write data column sidecar chunk")
				closeStream(stream, log)
				return
			}
		}
		closeStream(stream, log)
	})

	// Create the record and set the custody count
	enr := &enr.Record{}
	enr.Set(peerdas.Cgc(setup.custodyGroupCount))

	// Add the peer and connect it
	hostP2P.Peers().Add(enr, peerP2P.PeerID(), nil, network.DirOutbound)
	hostP2P.Peers().SetConnectionState(peerP2P.PeerID(), peers.Connected)
	hostP2P.Connect(peerP2P)

	return peerP2P
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
