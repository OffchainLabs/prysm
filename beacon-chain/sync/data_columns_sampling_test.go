package sync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	GoKZG "github.com/crate-crypto/go-kzg-4844"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	kzg "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/peers"
	p2ptest "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
	p2pTypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/sirupsen/logrus"
)

func TestRandomizeColumns(t *testing.T) {
	const count uint64 = 128

	// Generate columns.
	columns := make(map[uint64]bool, count)
	for i := uint64(0); i < count; i++ {
		columns[i] = true
	}

	// Randomize columns.
	randomizedColumns := randomizeColumns(columns)

	// Convert back to a map.
	randomizedColumnsMap := make(map[uint64]bool, count)
	for _, column := range randomizedColumns {
		randomizedColumnsMap[column] = true
	}

	// Check duplicates and missing columns.
	require.Equal(t, len(columns), len(randomizedColumnsMap))

	// Check the values.
	for column := range randomizedColumnsMap {
		require.Equal(t, true, column < count)
	}
}

// createAndConnectPeer creates a peer with a private key `offset` fixed.
// The peer is added and connected to `p2pService`.
// If a `RPCDataColumnSidecarsByRootTopicV1` request is made with column index `i`,
// then the peer will respond with the `dataColumnSidecars[i]` if it is not in `columnsNotToRespond`.
// (If `len(dataColumnSidecars) < i`, then this function will panic.)
func createAndConnectPeer(
	t *testing.T,
	p2pService *p2ptest.TestP2P,
	chainService *mock.ChainService,
	dataColumnSidecars []*ethpb.DataColumnSidecar,
	custodySubnetCount uint64,
	columnsNotToRespond map[uint64]bool,
	offset int,
) *p2ptest.TestP2P {
	// Create the private key, depending on the offset.
	privateKeyBytes := make([]byte, 32)
	for i := 0; i < 32; i++ {
		privateKeyBytes[i] = byte(offset + i)
	}

	privateKey, err := crypto.UnmarshalSecp256k1PrivateKey(privateKeyBytes)
	require.NoError(t, err)

	// Create the peer.
	peer := p2ptest.NewTestP2P(t, libp2p.Identity(privateKey))

	peer.SetStreamHandler(p2p.RPCDataColumnSidecarsByRootTopicV1+"/ssz_snappy", func(stream network.Stream) {
		// Decode the request.
		req := new(p2pTypes.DataColumnSidecarsByRootReq)
		err := peer.Encoding().DecodeWithMaxLength(stream, req)
		require.NoError(t, err)

		for _, identifier := range *req {
			// Filter out the columns not to respond.
			if columnsNotToRespond[identifier.ColumnIndex] {
				continue
			}

			// Create the response.
			resp := dataColumnSidecars[identifier.ColumnIndex]

			// Send the response.
			err := WriteDataColumnSidecarChunk(stream, chainService, p2pService.Encoding(), resp)
			require.NoError(t, err)
		}

		// Close the stream.
		closeStream(stream, log)
	})

	// Create the record and set the custody count.
	enr := &enr.Record{}
	enr.Set(peerdas.Csc(custodySubnetCount))

	// Add the peer and connect it.
	p2pService.Peers().Add(enr, peer.PeerID(), nil, network.DirOutbound)
	p2pService.Peers().SetConnectionState(peer.PeerID(), peers.PeerConnected)
	p2pService.Connect(peer)

	return peer
}

type dataSamplerTest struct {
	ctx                context.Context
	p2pSvc             *p2ptest.TestP2P
	peers              []*p2ptest.TestP2P
	ctxMap             map[[4]byte]int
	chainSvc           *mock.ChainService
	blockRoot          [32]byte
	blobs              []kzg.Blob
	kzgCommitments     [][]byte
	kzgProofs          [][]byte
	dataColumnSidecars []*ethpb.DataColumnSidecar
}

func setupDefaultDataColumnSamplerTest(t *testing.T) (*dataSamplerTest, *dataColumnSampler1D) {
	const (
		blobCount          uint64 = 3
		custodyRequirement uint64 = 1
	)

	test, sampler := setupDataColumnSamplerTest(t, blobCount)
	// Custody columns: [6, 38, 70, 102]
	p1 := createAndConnectPeer(t, test.p2pSvc, test.chainSvc, test.dataColumnSidecars, custodyRequirement, map[uint64]bool{}, 1)
	// Custody columns: [3, 35, 67, 99]
	p2 := createAndConnectPeer(t, test.p2pSvc, test.chainSvc, test.dataColumnSidecars, custodyRequirement, map[uint64]bool{}, 2)
	// Custody columns: [12, 44, 76, 108]
	p3 := createAndConnectPeer(t, test.p2pSvc, test.chainSvc, test.dataColumnSidecars, custodyRequirement, map[uint64]bool{}, 3)
	test.peers = []*p2ptest.TestP2P{p1, p2, p3}

	return test, sampler
}

func setupDataColumnSamplerTest(t *testing.T, blobCount uint64) (*dataSamplerTest, *dataColumnSampler1D) {
	require.NoError(t, kzg.Start())

	// Generate random blobs, commitments and inclusion proofs.
	blobs := make([]kzg.Blob, blobCount)
	kzgCommitments := make([][]byte, blobCount)
	kzgProofs := make([][]byte, blobCount)

	for i := uint64(0); i < blobCount; i++ {
		blob := getRandBlob(int64(i))

		kzgCommitment, kzgProof, err := generateCommitmentAndProof(&blob)
		require.NoError(t, err)

		blobs[i] = blob
		kzgCommitments[i] = kzgCommitment[:]
		kzgProofs[i] = kzgProof[:]
	}

	dbBlock := util.NewBeaconBlockDeneb()
	dbBlock.Block.Body.BlobKzgCommitments = kzgCommitments
	sBlock, err := blocks.NewSignedBeaconBlock(dbBlock)
	require.NoError(t, err)

	dataColumnSidecars, err := peerdas.DataColumnSidecars(sBlock, blobs)
	require.NoError(t, err)

	blockRoot, err := dataColumnSidecars[0].GetSignedBlockHeader().Header.HashTreeRoot()
	require.NoError(t, err)

	p2pSvc := p2ptest.NewTestP2P(t)
	chainSvc, clock := defaultMockChain(t)

	test := &dataSamplerTest{
		ctx:                context.Background(),
		p2pSvc:             p2pSvc,
		peers:              []*p2ptest.TestP2P{},
		ctxMap:             map[[4]byte]int{{245, 165, 253, 66}: version.Deneb},
		chainSvc:           chainSvc,
		blockRoot:          blockRoot,
		blobs:              blobs,
		kzgCommitments:     kzgCommitments,
		kzgProofs:          kzgProofs,
		dataColumnSidecars: dataColumnSidecars,
	}
	clockSync := startup.NewClockSynchronizer()
	require.NoError(t, clockSync.SetClock(clock))
	iniWaiter := verification.NewInitializerWaiter(clockSync, nil, nil)
	ini, err := iniWaiter.WaitForInitializer(context.Background())
	require.NoError(t, err)
	sampler := newDataColumnSampler1D(p2pSvc, clock, test.ctxMap, nil, newColumnVerifierFromInitializer(ini))

	return test, sampler
}

func TestDataColumnSampler1D_PeerManagement(t *testing.T) {
	testCases := []struct {
		numPeers           int
		custodyRequirement uint64
		subnetCount        uint64
		expectedColumns    [][]uint64
		prunePeers         map[int]bool // Peers to prune.
	}{
		{
			numPeers:           3,
			custodyRequirement: 1,
			subnetCount:        32,
			expectedColumns: [][]uint64{
				{6, 38, 70, 102},
				{3, 35, 67, 99},
				{12, 44, 76, 108},
			},
			prunePeers: map[int]bool{
				0: true,
			},
		},
		{
			numPeers:           3,
			custodyRequirement: 2,
			subnetCount:        32,
			expectedColumns: [][]uint64{
				{6, 16, 38, 48, 70, 80, 102, 112},
				{3, 13, 35, 45, 67, 77, 99, 109},
				{12, 31, 44, 63, 76, 95, 108, 127},
			},
			prunePeers: map[int]bool{
				0: true,
			},
		},
	}

	params.SetupTestConfigCleanup(t)
	for _, tc := range testCases {
		cfg := params.BeaconConfig()
		cfg.CustodyRequirement = tc.custodyRequirement
		cfg.DataColumnSidecarSubnetCount = tc.subnetCount
		params.OverrideBeaconConfig(cfg)
		test, sampler := setupDataColumnSamplerTest(t, uint64(tc.numPeers))
		for i := 0; i < tc.numPeers; i++ {
			p := createAndConnectPeer(t, test.p2pSvc, test.chainSvc, test.dataColumnSidecars, tc.custodyRequirement, nil, i+1)
			test.peers = append(test.peers, p)
		}

		// confirm everything works
		sampler.refreshPeerInfo()
		require.Equal(t, params.BeaconConfig().NumberOfColumns, uint64(len(sampler.peerFromColumn)))

		require.Equal(t, tc.numPeers, len(sampler.columnFromPeer))
		for i, peer := range test.peers {
			// confirm peer has the expected columns
			require.Equal(t, len(tc.expectedColumns[i]), len(sampler.columnFromPeer[peer.PeerID()]))
			for _, column := range tc.expectedColumns[i] {
				require.Equal(t, true, sampler.columnFromPeer[peer.PeerID()][column])
			}

			// confirm column to peer mapping are correct
			for _, column := range tc.expectedColumns[i] {
				require.Equal(t, true, sampler.peerFromColumn[column][peer.PeerID()])
			}
		}

		// prune peers
		for peer := range tc.prunePeers {
			err := test.p2pSvc.Disconnect(test.peers[peer].PeerID())
			test.p2pSvc.Peers().SetConnectionState(test.peers[peer].PeerID(), peers.PeerDisconnected)
			require.NoError(t, err)
		}
		sampler.refreshPeerInfo()

		require.Equal(t, tc.numPeers-len(tc.prunePeers), len(sampler.columnFromPeer))
		for i, peer := range test.peers {
			for _, column := range tc.expectedColumns[i] {
				expected := true
				if tc.prunePeers[i] {
					expected = false
				}
				require.Equal(t, expected, sampler.peerFromColumn[column][peer.PeerID()])
			}
		}
	}
}

func TestDataColumnSampler1D_SampleDistribution(t *testing.T) {
	testCases := []struct {
		numPeers             int
		custodyRequirement   uint64
		subnetCount          uint64
		columnsToDistribute  [][]uint64
		expectedDistribution []map[int][]uint64
	}{
		{
			numPeers:           3,
			custodyRequirement: 1,
			subnetCount:        32,
			// peer custody maps
			// p0: {6, 38, 70, 102},
			// p1: {3, 35, 67, 99},
			// p2: {12, 44, 76, 108},
			columnsToDistribute: [][]uint64{
				{3, 6, 12},
				{6, 3, 12, 38, 35, 44},
				{6, 38, 70},
				{11},
			},
			expectedDistribution: []map[int][]uint64{
				{
					0: {6},  // p1
					1: {3},  // p2
					2: {12}, // p3
				},
				{
					0: {6, 38},  // p1
					1: {3, 35},  // p2
					2: {12, 44}, // p3
				},
				{
					0: {6, 38, 70}, // p1
				},
				{},
			},
		},
		{
			numPeers:           3,
			custodyRequirement: 2,
			subnetCount:        32,
			// peer custody maps
			// p0: {6, 16, 38, 48, 70, 80, 102, 112},
			// p1: {3, 13, 35, 45, 67, 77, 99, 109},
			// p2: {12, 31, 44, 63, 76, 95, 108, 127},
			columnsToDistribute: [][]uint64{
				{3, 6, 12, 109, 112, 127}, // all covered by peers
				{13, 16, 31, 32},          // 32 not in covered by peers
			},
			expectedDistribution: []map[int][]uint64{
				{
					0: {6, 112},  // p1
					1: {3, 109},  // p2
					2: {12, 127}, // p3
				},
				{
					0: {16}, // p1
					1: {13}, // p2
					2: {31}, // p3
				},
			},
		},
	}
	params.SetupTestConfigCleanup(t)
	for _, tc := range testCases {
		cfg := params.BeaconConfig()
		cfg.CustodyRequirement = tc.custodyRequirement
		cfg.DataColumnSidecarSubnetCount = tc.subnetCount
		params.OverrideBeaconConfig(cfg)
		test, sampler := setupDataColumnSamplerTest(t, uint64(tc.numPeers))
		for i := 0; i < tc.numPeers; i++ {
			p := createAndConnectPeer(t, test.p2pSvc, test.chainSvc, test.dataColumnSidecars, tc.custodyRequirement, nil, i+1)
			test.peers = append(test.peers, p)
		}
		sampler.refreshPeerInfo()

		for idx, columns := range tc.columnsToDistribute {
			result := sampler.distributeSamplesToPeer(columns)
			require.Equal(t, len(tc.expectedDistribution[idx]), len(result), fmt.Sprintf("%v - %v", tc.expectedDistribution[idx], result))

			for peerIdx, dist := range tc.expectedDistribution[idx] {
				for _, column := range dist {
					peerID := test.peers[peerIdx].PeerID()
					require.Equal(t, true, result[peerID][column])
				}
			}
		}
	}
}

func TestDataColumnSampler1D_SampleDataColumns(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.DataColumnSidecarSubnetCount = 32
	params.OverrideBeaconConfig(cfg)
	test, sampler := setupDefaultDataColumnSamplerTest(t)
	sampler.refreshPeerInfo()

	// Sample all columns.
	sampleColumns := []uint64{6, 3, 12, 38, 35, 44, 70, 67, 76, 102, 99, 108}
	retrieved := sampler.sampleDataColumns(test.ctx, test.blockRoot, sampleColumns)
	require.Equal(t, 12, len(retrieved))
	for _, column := range sampleColumns {
		require.Equal(t, true, retrieved[column])
	}

	// Sample a subset of columns.
	sampleColumns = []uint64{6, 3, 12, 38, 35, 44}
	retrieved = sampler.sampleDataColumns(test.ctx, test.blockRoot, sampleColumns)
	require.Equal(t, 6, len(retrieved))
	for _, column := range sampleColumns {
		require.Equal(t, true, retrieved[column])
	}

	// Sample a subset of columns with missing columns.
	sampleColumns = []uint64{6, 3, 12, 127}
	retrieved = sampler.sampleDataColumns(test.ctx, test.blockRoot, sampleColumns)
	require.Equal(t, 3, len(retrieved))
	require.DeepEqual(t, map[uint64]bool{6: true, 3: true, 12: true}, retrieved)
}

func TestDataColumnSampler1D_IncrementalDAS(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.DataColumnSidecarSubnetCount = 32
	params.OverrideBeaconConfig(cfg)

	testCases := []struct {
		name                     string
		samplesCount             uint64
		possibleColumnsToRequest []uint64
		columnsNotToRespond      map[uint64]bool
		expectedSuccess          bool
		expectedRoundSummaries   []roundSummary
	}{
		{
			name:                     "All columns are correctly sampled in a single round",
			samplesCount:             5,
			possibleColumnsToRequest: []uint64{70, 35, 99, 6, 38, 3, 67, 102, 12, 44, 76, 108},
			columnsNotToRespond:      map[uint64]bool{},
			expectedSuccess:          true,
			expectedRoundSummaries: []roundSummary{
				{
					RequestedColumns: []uint64{70, 35, 99, 6, 38},
					MissingColumns:   map[uint64]bool{},
				},
			},
		},
		{
			name:                     "Two missing columns in the first round, ok in the second round",
			samplesCount:             5,
			possibleColumnsToRequest: []uint64{70, 35, 99, 6, 38, 3, 67, 102, 12, 44, 76, 108},
			columnsNotToRespond:      map[uint64]bool{6: true, 70: true},
			expectedSuccess:          true,
			expectedRoundSummaries: []roundSummary{
				{
					RequestedColumns: []uint64{70, 35, 99, 6, 38},
					MissingColumns:   map[uint64]bool{70: true, 6: true},
				},
				{
					RequestedColumns: []uint64{3, 67, 102, 12, 44, 76},
					MissingColumns:   map[uint64]bool{},
				},
			},
		},
		{
			name:                     "Two missing columns in the first round, one missing in the second round. Fail to sample.",
			samplesCount:             5,
			possibleColumnsToRequest: []uint64{70, 35, 99, 6, 38, 3, 67, 102, 12, 44, 76, 108},
			columnsNotToRespond:      map[uint64]bool{6: true, 70: true, 3: true},
			expectedSuccess:          false,
			expectedRoundSummaries: []roundSummary{
				{
					RequestedColumns: []uint64{70, 35, 99, 6, 38},
					MissingColumns:   map[uint64]bool{70: true, 6: true},
				},
				{
					RequestedColumns: []uint64{3, 67, 102, 12, 44, 76},
					MissingColumns:   map[uint64]bool{3: true},
				},
			},
		},
	}

	for _, tc := range testCases {
		test, sampler := setupDataColumnSamplerTest(t, 3)
		p1 := createAndConnectPeer(t, test.p2pSvc, test.chainSvc, test.dataColumnSidecars, params.BeaconConfig().CustodyRequirement, tc.columnsNotToRespond, 1)
		p2 := createAndConnectPeer(t, test.p2pSvc, test.chainSvc, test.dataColumnSidecars, params.BeaconConfig().CustodyRequirement, tc.columnsNotToRespond, 2)
		p3 := createAndConnectPeer(t, test.p2pSvc, test.chainSvc, test.dataColumnSidecars, params.BeaconConfig().CustodyRequirement, tc.columnsNotToRespond, 3)
		test.peers = []*p2ptest.TestP2P{p1, p2, p3}

		sampler.refreshPeerInfo()

		success, summaries, err := sampler.incrementalDAS(test.ctx, test.blockRoot, tc.possibleColumnsToRequest, tc.samplesCount)
		require.NoError(t, err)
		require.Equal(t, tc.expectedSuccess, success)
		require.DeepEqual(t, tc.expectedRoundSummaries, summaries)
	}
}

func deterministicRandomness(seed int64) [32]byte {
	// Converts an int64 to a byte slice
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, seed)
	if err != nil {
		logrus.WithError(err).Error("Failed to write int64 to bytes buffer")
		return [32]byte{}
	}
	bytes := buf.Bytes()

	return sha256.Sum256(bytes)
}

// Returns a serialized random field element in big-endian
func getRandFieldElement(seed int64) [32]byte {
	bytes := deterministicRandomness(seed)
	var r fr.Element
	r.SetBytes(bytes[:])

	return GoKZG.SerializeScalar(r)
}

// Returns a random blob using the passed seed as entropy
func getRandBlob(seed int64) kzg.Blob {
	var blob kzg.Blob
	for i := 0; i < len(blob); i += 32 {
		fieldElementBytes := getRandFieldElement(seed + int64(i))
		copy(blob[i:i+32], fieldElementBytes[:])
	}
	return blob
}

func generateCommitmentAndProof(blob *kzg.Blob) (*kzg.Commitment, *kzg.Proof, error) {
	commitment, err := kzg.BlobToKZGCommitment(blob)
	if err != nil {
		return nil, nil, err
	}
	proof, err := kzg.ComputeBlobKZGProof(blob, commitment)
	if err != nil {
		return nil, nil, err
	}
	return &commitment, &proof, err
}
