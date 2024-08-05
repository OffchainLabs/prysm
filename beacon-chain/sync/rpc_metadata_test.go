package sync

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/prysmaticlabs/go-bitfield"
	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	db "github.com/prysmaticlabs/prysm/v5/beacon-chain/db/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	p2ptest "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/wrapper"
	leakybucket "github.com/prysmaticlabs/prysm/v5/container/leaky-bucket"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/metadata"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestMetaDataRPCHandler_ReceivesMetadata(t *testing.T) {
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	assert.Equal(t, 1, len(p1.BHost.Network().Peers()), "Expected peers to be connected")
	bitfield := [8]byte{'A', 'B'}
	p1.LocalMetadata = wrapper.WrappedMetadataV0(&pb.MetaDataV0{
		SeqNumber: 2,
		Attnets:   bitfield[:],
	})

	// Set up a head state in the database with data we expect.
	d := db.SetupDB(t)
	r := &Service{
		cfg: &config{
			beaconDB: d,
			p2p:      p1,
			chain: &mock.ChainService{
				ValidatorsRoot: [32]byte{},
			},
		},
		rateLimiter: newRateLimiter(p1),
	}

	// Setup streams
	pcl := protocol.ID(p2p.RPCMetaDataTopicV1)
	topic := string(pcl)
	r.rateLimiter.limiterMap[topic] = leakybucket.NewCollector(1, 1, time.Second, false)
	var wg sync.WaitGroup
	wg.Add(1)
	p2.BHost.SetStreamHandler(pcl, func(stream network.Stream) {
		defer wg.Done()
		expectSuccess(t, stream)
		out := new(pb.MetaDataV0)
		assert.NoError(t, r.cfg.p2p.Encoding().DecodeWithMaxLength(stream, out))
		assert.DeepEqual(t, p1.LocalMetadata.InnerObject(), out, "MetadataV0 unequal")
	})
	stream1, err := p1.BHost.NewStream(context.Background(), p2.BHost.ID(), pcl)
	require.NoError(t, err)

	assert.NoError(t, r.metaDataHandler(context.Background(), new(interface{}), stream1))

	if util.WaitTimeout(&wg, 1*time.Second) {
		t.Fatal("Did not receive stream within 1 sec")
	}

	conns := p1.BHost.Network().ConnsToPeer(p2.BHost.ID())
	if len(conns) == 0 {
		t.Error("Peer is disconnected despite receiving a valid ping")
	}
}

func createService(peer p2p.P2P, chain *mock.ChainService) *Service {
	return &Service{
		cfg: &config{
			p2p:   peer,
			chain: chain,
			clock: startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
		},
		rateLimiter: newRateLimiter(peer),
	}
}

func TestMetadataRPCHandler_SendMetadataRequest(t *testing.T) {
	const (
		requestTimeout     = 1 * time.Second
		seqNumber          = 2
		custodySubnetCount = 4
	)

	attnets := []byte{'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H'}
	syncnets := []byte{0x4}

	// Configure the test beacon chain.
	params.SetupTestConfigCleanup(t)
	beaconChainConfig := params.BeaconConfig().Copy()
	beaconChainConfig.AltairForkEpoch = 5
	beaconChainConfig.DenebForkEpoch = 10
	beaconChainConfig.Eip7594ForkEpoch = 10
	params.OverrideBeaconConfig(beaconChainConfig)
	params.BeaconConfig().InitializeForkSchedule()

	// Compute the number of seconds in an epoch.
	secondsPerEpoch := oneEpoch()

	testCases := []struct {
		name                                             string
		topic                                            string
		epochsSinceGenesisPeer1, epochsSinceGenesisPeer2 int
		metadataPeer2, expected                          metadata.Metadata
	}{
		{
			name:                    "Phase0-Phase0",
			topic:                   p2p.RPCMetaDataTopicV1,
			epochsSinceGenesisPeer1: 0,
			epochsSinceGenesisPeer2: 0,
			metadataPeer2: wrapper.WrappedMetadataV0(&pb.MetaDataV0{
				SeqNumber: seqNumber,
				Attnets:   attnets,
			}),
			expected: wrapper.WrappedMetadataV0(&pb.MetaDataV0{
				SeqNumber: seqNumber,
				Attnets:   attnets,
			}),
		},
		{
			name:                    "Phase0-Altair",
			topic:                   p2p.RPCMetaDataTopicV1,
			epochsSinceGenesisPeer1: 0,
			epochsSinceGenesisPeer2: 5,
			metadataPeer2: wrapper.WrappedMetadataV1(&pb.MetaDataV1{
				SeqNumber: seqNumber,
				Attnets:   attnets,
				Syncnets:  syncnets,
			}),
			expected: wrapper.WrappedMetadataV0(&pb.MetaDataV0{
				SeqNumber: seqNumber,
				Attnets:   attnets,
			}),
		},
		{
			name:                    "Phase0-PeerDAS",
			topic:                   p2p.RPCMetaDataTopicV1,
			epochsSinceGenesisPeer1: 0,
			epochsSinceGenesisPeer2: 10,
			metadataPeer2: wrapper.WrappedMetadataV2(&pb.MetaDataV2{
				SeqNumber:          seqNumber,
				Attnets:            attnets,
				Syncnets:           syncnets,
				CustodySubnetCount: custodySubnetCount,
			}),
			expected: wrapper.WrappedMetadataV0(&pb.MetaDataV0{
				SeqNumber: seqNumber,
				Attnets:   attnets,
			}),
		},
		{
			name:                    "Altair-Phase0",
			topic:                   p2p.RPCMetaDataTopicV2,
			epochsSinceGenesisPeer1: 5,
			epochsSinceGenesisPeer2: 0,
			metadataPeer2: wrapper.WrappedMetadataV0(&pb.MetaDataV0{
				SeqNumber: seqNumber,
				Attnets:   attnets,
			}),
			expected: wrapper.WrappedMetadataV1(&pb.MetaDataV1{
				SeqNumber: seqNumber,
				Attnets:   attnets,
				Syncnets:  bitfield.Bitvector4{byte(0x00)},
			}),
		},
		{
			name:                    "Altair-Altair",
			topic:                   p2p.RPCMetaDataTopicV2,
			epochsSinceGenesisPeer1: 5,
			epochsSinceGenesisPeer2: 5,
			metadataPeer2: wrapper.WrappedMetadataV1(&pb.MetaDataV1{
				SeqNumber: seqNumber,
				Attnets:   attnets,
				Syncnets:  syncnets,
			}),
			expected: wrapper.WrappedMetadataV1(&pb.MetaDataV1{
				SeqNumber: seqNumber,
				Attnets:   attnets,
				Syncnets:  syncnets,
			}),
		},
		{
			name:                    "Altair-PeerDAS",
			topic:                   p2p.RPCMetaDataTopicV2,
			epochsSinceGenesisPeer1: 5,
			epochsSinceGenesisPeer2: 10,
			metadataPeer2: wrapper.WrappedMetadataV2(&pb.MetaDataV2{
				SeqNumber:          seqNumber,
				Attnets:            attnets,
				Syncnets:           syncnets,
				CustodySubnetCount: custodySubnetCount,
			}),
			expected: wrapper.WrappedMetadataV1(&pb.MetaDataV1{
				SeqNumber: seqNumber,
				Attnets:   attnets,
				Syncnets:  syncnets,
			}),
		},
		{
			name:                    "PeerDAS-Phase0",
			topic:                   p2p.RPCMetaDataTopicV3,
			epochsSinceGenesisPeer1: 10,
			epochsSinceGenesisPeer2: 0,
			metadataPeer2: wrapper.WrappedMetadataV0(&pb.MetaDataV0{
				SeqNumber: seqNumber,
				Attnets:   attnets,
			}),
			expected: wrapper.WrappedMetadataV2(&pb.MetaDataV2{
				SeqNumber:          seqNumber,
				Attnets:            attnets,
				Syncnets:           bitfield.Bitvector4{byte(0x00)},
				CustodySubnetCount: 0,
			}),
		},
		{
			name:                    "PeerDAS-Altail",
			topic:                   p2p.RPCMetaDataTopicV3,
			epochsSinceGenesisPeer1: 10,
			epochsSinceGenesisPeer2: 5,
			metadataPeer2: wrapper.WrappedMetadataV1(&pb.MetaDataV1{
				SeqNumber: seqNumber,
				Attnets:   attnets,
				Syncnets:  syncnets,
			}),
			expected: wrapper.WrappedMetadataV2(&pb.MetaDataV2{
				SeqNumber:          seqNumber,
				Attnets:            attnets,
				Syncnets:           syncnets,
				CustodySubnetCount: 0,
			}),
		},
		{
			name:                    "PeerDAS-PeerDAS",
			topic:                   p2p.RPCMetaDataTopicV3,
			epochsSinceGenesisPeer1: 10,
			epochsSinceGenesisPeer2: 10,
			metadataPeer2: wrapper.WrappedMetadataV2(&pb.MetaDataV2{
				SeqNumber:          seqNumber,
				Attnets:            attnets,
				Syncnets:           syncnets,
				CustodySubnetCount: custodySubnetCount,
			}),
			expected: wrapper.WrappedMetadataV2(&pb.MetaDataV2{
				SeqNumber:          seqNumber,
				Attnets:            attnets,
				Syncnets:           syncnets,
				CustodySubnetCount: custodySubnetCount,
			}),
		},
	}

	for _, tc := range testCases {
		var wg sync.WaitGroup

		ctx := context.Background()

		// Setup and connect peers.
		peer1, peer2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
		peer1.Connect(peer2)

		// Ensure the peers are connected.
		peersCount := len(peer1.BHost.Network().Peers())
		assert.Equal(t, 1, peersCount, "Expected peers to be connected")

		// Setup sync services.
		genesisPeer1 := time.Now().Add(-time.Duration(tc.epochsSinceGenesisPeer1) * secondsPerEpoch)
		genesisPeer2 := time.Now().Add(-time.Duration(tc.epochsSinceGenesisPeer2) * secondsPerEpoch)

		chainPeer1 := &mock.ChainService{Genesis: genesisPeer1, ValidatorsRoot: [32]byte{}}
		chainPeer2 := &mock.ChainService{Genesis: genesisPeer2, ValidatorsRoot: [32]byte{}}

		servicePeer1 := createService(peer1, chainPeer1)
		servicePeer2 := createService(peer2, chainPeer2)

		// Define the behavior of peer2 when receiving a METADATA request.
		protocolSuffix := servicePeer2.cfg.p2p.Encoding().ProtocolSuffix()
		protocolID := protocol.ID(tc.topic + protocolSuffix)
		peer2.LocalMetadata = tc.metadataPeer2

		wg.Add(1)
		peer2.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer wg.Done()
			err := servicePeer2.metaDataHandler(ctx, new(interface{}), stream)
			assert.NoError(t, err)
		})

		// Send a METADATA request from peer1 to peer2.
		actual, err := servicePeer1.sendMetaDataRequest(ctx, peer2.BHost.ID())
		assert.NoError(t, err)

		// Wait until the METADATA request is received by peer2 or timeout.
		timeOutReached := util.WaitTimeout(&wg, requestTimeout)
		require.Equal(t, false, timeOutReached, "Did not receive METADATA request within timeout")

		// Compare the received METADATA object with the expected METADATA object.
		require.DeepSSZEqual(t, tc.expected.InnerObject(), actual.InnerObject(), "Metadata unequal")

		// Ensure the peers are still connected.
		peersCount = len(peer1.BHost.Network().Peers())
		assert.Equal(t, 1, peersCount, "Expected peers to be connected")
	}
}
