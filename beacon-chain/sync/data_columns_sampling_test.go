package sync

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/peers"
	p2ptest "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
	p2pTypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
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
	header *ethpb.BeaconBlockHeader,
	custodyCount uint64,
	columnsNotToRespond map[uint64]bool,
	offset int,
) {
	emptyRoot := [fieldparams.RootLength]byte{}
	emptySignature := [fieldparams.BLSSignatureLength]byte{}
	emptyKzgCommitmentInclusionProof := [4][]byte{
		emptyRoot[:], emptyRoot[:], emptyRoot[:], emptyRoot[:],
	}

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
			resp := ethpb.DataColumnSidecar{
				ColumnIndex: identifier.ColumnIndex,
				SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
					Header:    header,
					Signature: emptySignature[:],
				},
				KzgCommitmentsInclusionProof: emptyKzgCommitmentInclusionProof[:],
			}

			// Send the response.
			err := WriteDataColumnSidecarChunk(stream, chainService, p2pService.Encoding(), &resp)
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

func TestIncrementalDAS(t *testing.T) {
	const custodyRequirement uint64 = 1

	emptyRoot := [fieldparams.RootLength]byte{}
	emptyHeader := &ethpb.BeaconBlockHeader{
		ParentRoot: emptyRoot[:],
		StateRoot:  emptyRoot[:],
		BodyRoot:   emptyRoot[:],
	}

	emptyHeaderRoot, err := emptyHeader.HashTreeRoot()
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
		createAndConnectPeer(t, p2pService, chainService, emptyHeader, custodyRequirement, tc.columnsNotToRespond, 1)

		// Custody columns: [3, 35, 67, 99]
		createAndConnectPeer(t, p2pService, chainService, emptyHeader, custodyRequirement, tc.columnsNotToRespond, 2)

		// Custody columns: [12, 44, 76, 108]
		createAndConnectPeer(t, p2pService, chainService, emptyHeader, custodyRequirement, tc.columnsNotToRespond, 3)

		service := &Service{
			cfg: &config{
				p2p:   p2pService,
				clock: clock,
			},
			ctx:    ctx,
			ctxMap: map[[4]byte]int{{245, 165, 253, 66}: version.Deneb},
		}

		actualSuccess, actualRoundSummaries, err := service.incrementalDAS(emptyHeaderRoot, tc.possibleColumnsToRequest, tc.samplesCount)

		require.NoError(t, err)
		require.Equal(t, tc.expectedSuccess, actualSuccess)
		require.DeepEqual(t, tc.expectedRoundSummaries, actualRoundSummaries)
	}
}
