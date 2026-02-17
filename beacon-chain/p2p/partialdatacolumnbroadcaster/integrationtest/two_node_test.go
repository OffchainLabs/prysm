package integrationtest

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/partialdatacolumnbroadcaster"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	simlibp2p "github.com/libp2p/go-libp2p/x/simlibp2p"
	"github.com/marcopolo/simnet"
	"github.com/sirupsen/logrus"
)

// TestTwoNodePartialColumnExchange tests that two nodes can exchange partial columns
// and reconstruct the complete column. Node 1 has cells 0-2, Node 2 has cells 3-5.
// After exchange, both should have all cells.
func TestTwoNodePartialColumnExchange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Create a simulated libp2p network
		latency := time.Millisecond * 10
		network, meta, err := simlibp2p.SimpleLibp2pNetwork([]simlibp2p.NodeLinkSettingsAndCount{
			{LinkSettings: simnet.NodeBiDiLinkSettings{
				Downlink: simnet.LinkSettings{BitsPerSecond: 20 * simlibp2p.OneMbps, Latency: latency / 2},
				Uplink:   simnet.LinkSettings{BitsPerSecond: 20 * simlibp2p.OneMbps, Latency: latency / 2},
			}, Count: 2},
		}, simlibp2p.NetworkSettings{UseBlankHost: true})
		require.NoError(t, err)
		require.NoError(t, network.Start())
		defer func() {
			require.NoError(t, network.Close())
		}()
		defer func() {
			for _, node := range meta.Nodes {
				err := node.Close()
				if err != nil {
					panic(err)
				}
			}
		}()

		h1 := meta.Nodes[0]
		h2 := meta.Nodes[1]

		logger := logrus.New()
		logger.SetLevel(logrus.DebugLevel)
		broadcaster1 := partialdatacolumnbroadcaster.NewBroadcaster(logger)
		broadcaster2 := partialdatacolumnbroadcaster.NewBroadcaster(logger)

		opts1 := broadcaster1.AppendPubSubOpts([]pubsub.Option{
			pubsub.WithMessageSigning(false),
			pubsub.WithStrictSignatureVerification(false),
		})
		opts2 := broadcaster2.AppendPubSubOpts([]pubsub.Option{
			pubsub.WithMessageSigning(false),
			pubsub.WithStrictSignatureVerification(false),
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ps1, err := pubsub.NewGossipSub(ctx, h1, opts1...)
		require.NoError(t, err)
		ps2, err := pubsub.NewGossipSub(ctx, h2, opts2...)
		require.NoError(t, err)

		defer func() {
			broadcaster1.Stop()
			broadcaster2.Stop()
		}()

		// Generate Test Data
		var blockRoot [fieldparams.RootLength]byte
		copy(blockRoot[:], []byte("test-block-root"))

		numCells := 6
		commitments := make([][]byte, numCells)
		cells := make([][]byte, numCells)
		proofs := make([][]byte, numCells)

		for i := range numCells {
			commitments[i] = make([]byte, 48)

			cells[i] = make([]byte, 2048)
			_, err := rand.Read(cells[i])
			require.NoError(t, err)
			proofs[i] = make([]byte, 48)
			_ = fmt.Appendf(proofs[i][:0], "proof %d", i)
		}

		roDC, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{
			{
				BodyRoot:       blockRoot[:],
				KzgCommitments: commitments,
				Column:         cells,
				KzgProofs:      proofs,
			},
		})

		pc1, err := blocks.NewPartialDataColumn(roDC[0].DataColumnSidecar.SignedBlockHeader, roDC[0].Index, roDC[0].KzgCommitments, roDC[0].KzgCommitmentsInclusionProof)
		require.NoError(t, err)
		pc2, err := blocks.NewPartialDataColumn(roDC[0].DataColumnSidecar.SignedBlockHeader, roDC[0].Index, roDC[0].KzgCommitments, roDC[0].KzgCommitmentsInclusionProof)
		require.NoError(t, err)

		// Split data
		for i := range numCells {
			if i%2 == 0 {
				pc1.ExtendFromVerifiedCell(uint64(i), roDC[0].Column[i], roDC[0].KzgProofs[i])
			} else {
				pc2.ExtendFromVerifiedCell(uint64(i), roDC[0].Column[i], roDC[0].KzgProofs[i])
			}
		}

		// Setup Topic and Subscriptions
		digest := params.ForkDigest(0)
		columnIndex := uint64(12)
		subnet := peerdas.ComputeSubnetForDataColumnSidecar(columnIndex)
		topicStr := fmt.Sprintf(p2p.DataColumnSubnetTopicFormat, digest, subnet) +
			encoder.SszNetworkEncoder{}.ProtocolSuffix()

		time.Sleep(100 * time.Millisecond)

		topic1, err := ps1.Join(topicStr, pubsub.RequestPartialMessages())
		require.NoError(t, err)
		topic2, err := ps2.Join(topicStr, pubsub.RequestPartialMessages())
		require.NoError(t, err)

		// Header validator
		headerValidator := func(header *ethpb.PartialDataColumnHeader) (reject bool, err error) {
			if header == nil {
				return false, fmt.Errorf("nil header")
			}
			if header.SignedBlockHeader == nil || header.SignedBlockHeader.Header == nil {
				return true, fmt.Errorf("nil signed block header")
			}
			if len(header.KzgCommitments) == 0 {
				return true, fmt.Errorf("empty kzg commitments")
			}

			t.Log("Header validation passed")
			return false, nil
		}

		cellValidator := func(_ []blocks.CellProofBundle) error {
			return nil
		}

		node1Complete := make(chan blocks.VerifiedRODataColumn, 1)
		node2Complete := make(chan blocks.VerifiedRODataColumn, 1)

		handler1 := func(topic string, col blocks.VerifiedRODataColumn) {
			t.Logf("Node 1: Completed! Column has %d cells", len(col.Column))
			node1Complete <- col
		}

		handler2 := func(topic string, col blocks.VerifiedRODataColumn) {
			t.Logf("Node 2: Completed! Column has %d cells", len(col.Column))
			node2Complete <- col
		}

		// Connect hosts
		err = h1.Connect(context.Background(), peer.AddrInfo{
			ID:    h2.ID(),
			Addrs: h2.Addrs(),
		})
		require.NoError(t, err)
		time.Sleep(300 * time.Millisecond)

		// Subscribe to regular GossipSub (critical for partial message RPC exchange!)
		sub1, err := topic1.Subscribe()
		require.NoError(t, err)
		defer sub1.Cancel()

		sub2, err := topic2.Subscribe()
		require.NoError(t, err)
		defer sub2.Cancel()

		noopHeaderHandler := func(header *ethpb.PartialDataColumnHeader, groupID string) {}

		err = broadcaster1.Start(headerValidator, cellValidator, handler1, noopHeaderHandler)
		require.NoError(t, err)

		err = broadcaster2.Start(headerValidator, cellValidator, handler2, noopHeaderHandler)
		require.NoError(t, err)

		err = broadcaster1.Subscribe(topic1)
		require.NoError(t, err)
		err = broadcaster2.Subscribe(topic2)
		require.NoError(t, err)

		// Wait for mesh to form
		time.Sleep(2 * time.Second)

		// Publish
		t.Log("Publishing from Node 1")
		err = broadcaster1.Publish(topicStr, pc1, true)
		require.NoError(t, err)

		time.Sleep(200 * time.Millisecond)

		t.Log("Publishing from Node 2")
		err = broadcaster2.Publish(topicStr, pc2, true)
		require.NoError(t, err)

		//  Wait for Completion
		timeout := time.After(10 * time.Second)
		var col1, col2 blocks.VerifiedRODataColumn
		receivedCount := 0

		for receivedCount < 2 {
			select {
			case col1 = <-node1Complete:
				t.Log("Node 1 completed reconstruction")
				receivedCount++
			case col2 = <-node2Complete:
				t.Log("Node 2 completed reconstruction")
				receivedCount++
			case <-timeout:
				t.Fatalf("Timeout: Only %d/2 nodes completed", receivedCount)
			}
		}

		// Verify both columns have all cells
		assert.Equal(t, numCells, len(col1.Column), "Node 1 should have all cells")
		assert.Equal(t, numCells, len(col2.Column), "Node 2 should have all cells")
		assert.DeepSSZEqual(t, cells, col1.Column, "Node 1 cell mismatch")
		assert.DeepSSZEqual(t, cells, col2.Column, "Node 2 cell mismatch")
	})
}
