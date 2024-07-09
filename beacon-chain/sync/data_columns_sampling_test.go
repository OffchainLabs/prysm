package sync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
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
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/peers"
	p2ptest "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
	p2pTypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
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
// The peer is added and connected to `p2pService`
func createAndConnectPeer(
	t *testing.T,
	p2pService *p2ptest.TestP2P,
	chainService *mock.ChainService,
	dataColumnSidecars []*ethpb.DataColumnSidecar,
	custodyCount uint64,
	columnsNotToRespond map[uint64]bool,
	offset int,
) {
	// Create the private key, depending on the offset.
	privateKeyBytes := make([]byte, 32)
	for i := 0; i < 32; i++ {
		privateKeyBytes[i] = byte(offset + i)
	}

	privateKey, err := crypto.UnmarshalSecp256k1PrivateKey(privateKeyBytes)
	require.NoError(t, err)

	// Create the peer.
	peer := p2ptest.NewTestP2P(t, libp2p.Identity(privateKey))

	// TODO: Do not hardcode the topic.
	peer.SetStreamHandler("/eth2/beacon_chain/req/data_column_sidecars_by_root/1/ssz_snappy", func(stream network.Stream) {
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
	enr.Set(peerdas.Csc(custodyCount))

	// Add the peer and connect it.
	p2pService.Peers().Add(enr, peer.PeerID(), nil, network.DirOutbound)
	p2pService.Peers().SetConnectionState(peer.PeerID(), peers.PeerConnected)
	p2pService.Connect(peer)
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

func TestIncrementalDAS(t *testing.T) {
	const (
		blobCount                 = 3
		custodyRequirement uint64 = 1
	)

	err := kzg.Start()
	require.NoError(t, err)

	// Generate random blobs, commitments and inclusion proofs.
	blobs := make([]kzg.Blob, blobCount)
	kzgCommitments := make([][]byte, blobCount)
	kzgProofs := make([][]byte, blobCount)

	for i := int64(0); i < blobCount; i++ {
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
		// Create a context.
		ctx := context.Background()

		// Create the p2p service.
		p2pService := p2ptest.NewTestP2P(t)

		// Create a peer custodying `custodyRequirement` subnets.
		chainService, clock := defaultMockChain(t)

		// Custody columns: [6, 38, 70, 102]
		createAndConnectPeer(t, p2pService, chainService, dataColumnSidecars, custodyRequirement, tc.columnsNotToRespond, 1)

		// Custody columns: [3, 35, 67, 99]
		createAndConnectPeer(t, p2pService, chainService, dataColumnSidecars, custodyRequirement, tc.columnsNotToRespond, 2)

		// Custody columns: [12, 44, 76, 108]
		createAndConnectPeer(t, p2pService, chainService, dataColumnSidecars, custodyRequirement, tc.columnsNotToRespond, 3)

		service := &Service{
			cfg: &config{
				p2p:   p2pService,
				clock: clock,
			},
			ctx:    ctx,
			ctxMap: map[[4]byte]int{{245, 165, 253, 66}: version.Deneb},
		}

		actualSuccess, actualRoundSummaries, err := service.incrementalDAS(blockRoot, tc.possibleColumnsToRequest, tc.samplesCount)

		require.NoError(t, err)
		require.Equal(t, tc.expectedSuccess, actualSuccess)
		require.DeepEqual(t, tc.expectedRoundSummaries, actualRoundSummaries)
	}
}
