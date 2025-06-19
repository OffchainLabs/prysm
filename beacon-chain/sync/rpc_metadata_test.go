package sync

import (
	"context"
	"sync"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	db "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/wrapper"
	leakybucket "github.com/OffchainLabs/prysm/v6/container/leaky-bucket"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/equality"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/metadata"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/prysmaticlabs/go-bitfield"
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
	stream1, err := p1.BHost.NewStream(t.Context(), p2.BHost.ID(), pcl)
	require.NoError(t, err)

	assert.NoError(t, r.metaDataHandler(t.Context(), new(interface{}), stream1))

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
		requestTimeout    = 1 * time.Second
		seqNumber         = 2
		custodyGroupCount = 4
	)

	attnets := []byte{'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H'}
	syncnets := []byte{0x4}

	// Configure the test beacon chain.
	params.SetupTestConfigCleanup(t)
	beaconChainConfig := params.BeaconConfig().Copy()
	beaconChainConfig.AltairForkEpoch = 5
	beaconChainConfig.FuluForkEpoch = 15
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
			name:                    "Phase0-Fulu",
			topic:                   p2p.RPCMetaDataTopicV1,
			epochsSinceGenesisPeer1: 0,
			epochsSinceGenesisPeer2: 15,
			metadataPeer2: wrapper.WrappedMetadataV2(&pb.MetaDataV2{
				SeqNumber:         seqNumber,
				Attnets:           attnets,
				Syncnets:          syncnets,
				CustodyGroupCount: custodyGroupCount,
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
			name:                    "Altair-Fulu",
			topic:                   p2p.RPCMetaDataTopicV2,
			epochsSinceGenesisPeer1: 5,
			epochsSinceGenesisPeer2: 15,
			metadataPeer2: wrapper.WrappedMetadataV2(&pb.MetaDataV2{
				SeqNumber:         seqNumber,
				Attnets:           attnets,
				Syncnets:          syncnets,
				CustodyGroupCount: custodyGroupCount,
			}),
			expected: wrapper.WrappedMetadataV1(&pb.MetaDataV1{
				SeqNumber: seqNumber,
				Attnets:   attnets,
				Syncnets:  syncnets,
			}),
		},
		{
			name:                    "Fulu-Phase0",
			topic:                   p2p.RPCMetaDataTopicV3,
			epochsSinceGenesisPeer1: 15,
			epochsSinceGenesisPeer2: 0,
			metadataPeer2: wrapper.WrappedMetadataV0(&pb.MetaDataV0{
				SeqNumber: seqNumber,
				Attnets:   attnets,
			}),
			expected: wrapper.WrappedMetadataV2(&pb.MetaDataV2{
				SeqNumber:         seqNumber,
				Attnets:           attnets,
				Syncnets:          bitfield.Bitvector4{byte(0x00)},
				CustodyGroupCount: 0,
			}),
		},
		{
			name:                    "Fulu-Altair",
			topic:                   p2p.RPCMetaDataTopicV3,
			epochsSinceGenesisPeer1: 15,
			epochsSinceGenesisPeer2: 5,
			metadataPeer2: wrapper.WrappedMetadataV1(&pb.MetaDataV1{
				SeqNumber: seqNumber,
				Attnets:   attnets,
				Syncnets:  syncnets,
			}),
			expected: wrapper.WrappedMetadataV2(&pb.MetaDataV2{
				SeqNumber:         seqNumber,
				Attnets:           attnets,
				Syncnets:          syncnets,
				CustodyGroupCount: 0,
			}),
		},
		{
			name:                    "Fulu-Fulu",
			topic:                   p2p.RPCMetaDataTopicV3,
			epochsSinceGenesisPeer1: 15,
			epochsSinceGenesisPeer2: 15,
			metadataPeer2: wrapper.WrappedMetadataV2(&pb.MetaDataV2{
				SeqNumber:         seqNumber,
				Attnets:           attnets,
				Syncnets:          syncnets,
				CustodyGroupCount: custodyGroupCount,
			}),
			expected: wrapper.WrappedMetadataV2(&pb.MetaDataV2{
				SeqNumber:         seqNumber,
				Attnets:           attnets,
				Syncnets:          syncnets,
				CustodyGroupCount: custodyGroupCount,
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var wg sync.WaitGroup

			ctx := context.Background()

			// Setup and connect peers.
			peer1, peer2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
			peer1.Connect(peer2)

			// Ensure the peers are connected.
			peersCount := len(peer1.BHost.Network().Peers())
			require.Equal(t, 1, peersCount, "Expected peers to be connected")

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
				require.NoError(t, err)
			})

			// Send a METADATA request from peer1 to peer2.
			actual, err := servicePeer1.sendMetaDataRequest(ctx, peer2.BHost.ID())
			require.NoError(t, err)

			// Wait until the METADATA request is received by peer2 or timeout.
			timeOutReached := util.WaitTimeout(&wg, requestTimeout)
			require.Equal(t, false, timeOutReached, "Did not receive METADATA request within timeout")

			// Compare the received METADATA object with the expected METADATA object.
			require.DeepSSZEqual(t, tc.expected.InnerObject(), actual.InnerObject(), "Metadata unequal")

			// Ensure the peers are still connected.
			peersCount = len(peer1.BHost.Network().Peers())
			assert.Equal(t, 1, peersCount, "Expected peers to be connected")
		})
	}
}

func TestMetadataRPCHandler_SendsMetadataQUIC(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	bCfg := params.BeaconConfig().Copy()
	bCfg.AltairForkEpoch = 5
	params.OverrideBeaconConfig(bCfg)
	params.BeaconConfig().InitializeForkSchedule()

	p1 := p2ptest.NewTestP2P(t, libp2p.Transport(libp2pquic.NewTransport))
	p2 := p2ptest.NewTestP2P(t, libp2p.Transport(libp2pquic.NewTransport))
	p1.Connect(p2)
	assert.Equal(t, 1, len(p1.BHost.Network().Peers()), "Expected peers to be connected")
	bitfield := [8]byte{'A', 'B'}
	p2.LocalMetadata = wrapper.WrappedMetadataV0(&pb.MetaDataV0{
		SeqNumber: 2,
		Attnets:   bitfield[:],
	})

	// Set up a head state in the database with data we expect.
	d := db.SetupDB(t)
	chain := &mock.ChainService{Genesis: time.Now().Add(-5 * oneEpoch()), ValidatorsRoot: [32]byte{}}
	r := &Service{
		cfg: &config{
			beaconDB: d,
			p2p:      p1,
			chain:    chain,
			clock:    startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
		},
		rateLimiter: newRateLimiter(p1),
	}

	chain2 := &mock.ChainService{Genesis: time.Now().Add(-5 * oneEpoch()), ValidatorsRoot: [32]byte{}}
	r2 := &Service{
		cfg: &config{
			beaconDB: d,
			p2p:      p2,
			chain:    chain2,
			clock:    startup.NewClock(chain2.Genesis, chain2.ValidatorsRoot),
		},
		rateLimiter: newRateLimiter(p2),
	}

	// Setup streams
	pcl := protocol.ID(p2p.RPCMetaDataTopicV2 + r.cfg.p2p.Encoding().ProtocolSuffix())
	topic := string(pcl)
	r.rateLimiter.limiterMap[topic] = leakybucket.NewCollector(2, 2, time.Second, false)
	r2.rateLimiter.limiterMap[topic] = leakybucket.NewCollector(2, 2, time.Second, false)

	var wg sync.WaitGroup
	wg.Add(1)
	p2.BHost.SetStreamHandler(pcl, func(stream network.Stream) {
		defer wg.Done()
		err := r2.metaDataHandler(t.Context(), new(interface{}), stream)
		assert.NoError(t, err)
	})

	_, err := r.sendMetaDataRequest(t.Context(), p2.BHost.ID())
	assert.NoError(t, err)

	if util.WaitTimeout(&wg, 1*time.Second) {
		t.Fatal("Did not receive stream within 1 sec")
	}

	// Fix up peer with the correct metadata.
	p2.LocalMetadata = wrapper.WrappedMetadataV1(&pb.MetaDataV1{
		SeqNumber: 2,
		Attnets:   bitfield[:],
		Syncnets:  []byte{0x0},
	})

	wg.Add(1)
	p2.BHost.SetStreamHandler(pcl, func(stream network.Stream) {
		defer wg.Done()
		assert.NoError(t, r2.metaDataHandler(t.Context(), new(interface{}), stream))
	})

	md, err := r.sendMetaDataRequest(t.Context(), p2.BHost.ID())
	assert.NoError(t, err)

	if !equality.DeepEqual(md.InnerObject(), p2.LocalMetadata.InnerObject()) {
		t.Fatalf("MetadataV1 unequal, received %v but wanted %v", md, p2.LocalMetadata)
	}

	if util.WaitTimeout(&wg, 1*time.Second) {
		t.Fatal("Did not receive stream within 1 sec")
	}

	conns := p1.BHost.Network().ConnsToPeer(p2.BHost.ID())
	if len(conns) == 0 {
		t.Error("Peer is disconnected despite receiving a valid ping")
	}
}
