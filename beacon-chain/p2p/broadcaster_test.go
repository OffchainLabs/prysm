package p2p

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers/scorers"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/wrapper"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	testpb "github.com/OffchainLabs/prysm/v7/proto/testing"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"google.golang.org/protobuf/proto"
)

func TestService_Broadcast(t *testing.T) {
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	if len(p1.BHost.Network().Peers()) == 0 {
		t.Fatal("No peers")
	}

	p := &Service{
		host:                  p1.BHost,
		pubsub:                p1.PubSub(),
		joinedTopics:          map[string]*pubsub.Topic{},
		cfg:                   &Config{},
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
	}

	msg := &ethpb.Fork{
		Epoch:           55,
		CurrentVersion:  []byte("fooo"),
		PreviousVersion: []byte("barr"),
	}

	topic := "/eth2/%x/testing"
	// Set a test gossip mapping for testpb.TestSimpleMessage.
	GossipTypeMapping[reflect.TypeFor[*ethpb.Fork]()] = topic
	digest, err := p.currentForkDigest()
	require.NoError(t, err)
	topic = fmt.Sprintf(topic, digest)

	// External peer subscribes to the topic.
	topic += p.Encoding().ProtocolSuffix()
	sub, err := p2.SubscribeToTopic(topic)
	require.NoError(t, err)

	// Wait for libp2p mesh to establish
	require.Eventually(t, func() bool {
		return len(p.pubsub.ListPeers(topic)) > 0
	}, 5*time.Second, 10*time.Millisecond, "libp2p mesh did not establish")

	// Async listen for the pubsub, must be before the broadcast.
	var wg sync.WaitGroup
	wg.Add(1)
	go func(tt *testing.T) {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(t.Context(), 1*time.Second)
		defer cancel()

		incomingMessage, err := sub.Next(ctx)
		require.NoError(t, err)

		result := &ethpb.Fork{}
		require.NoError(t, p.Encoding().DecodeGossip(incomingMessage.Data, result))
		if !proto.Equal(result, msg) {
			tt.Errorf("Did not receive expected message, got %+v, wanted %+v", result, msg)
		}
	}(t)

	// Broadcast to peers and wait.
	require.NoError(t, p.Broadcast(t.Context(), msg))
	if util.WaitTimeout(&wg, 1*time.Second) {
		t.Error("Failed to receive pubsub within 1s")
	}
}

func TestService_Broadcast_ReturnsErr_TopicNotMapped(t *testing.T) {
	p := Service{
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
	}
	assert.ErrorContains(t, ErrMessageNotMapped.Error(), p.Broadcast(t.Context(), &testpb.AddressBook{}))
}

func TestService_Attestation_Subnet(t *testing.T) {
	if gtm := GossipTypeMapping[reflect.TypeFor[*ethpb.Attestation]()]; gtm != AttestationSubnetTopicFormat {
		t.Errorf("Constant is out of date. Wanted %s, got %s", AttestationSubnetTopicFormat, gtm)
	}

	tests := []struct {
		att   *ethpb.Attestation
		topic string
	}{
		{
			att: &ethpb.Attestation{
				Data: &ethpb.AttestationData{
					CommitteeIndex: 0,
					Slot:           2,
				},
			},
			topic: "/eth2/00000000/beacon_attestation_2",
		},
		{
			att: &ethpb.Attestation{
				Data: &ethpb.AttestationData{
					CommitteeIndex: 11,
					Slot:           10,
				},
			},
			topic: "/eth2/00000000/beacon_attestation_21",
		},
		{
			att: &ethpb.Attestation{
				Data: &ethpb.AttestationData{
					CommitteeIndex: 55,
					Slot:           529,
				},
			},
			topic: "/eth2/00000000/beacon_attestation_8",
		},
	}
	for _, tt := range tests {
		subnet := helpers.ComputeSubnetFromCommitteeAndSlot(100, tt.att.Data.CommitteeIndex, tt.att.Data.Slot)
		assert.Equal(t, tt.topic, attestationToTopic(subnet, [4]byte{} /* fork digest */), "Wrong topic")
	}
}

func TestService_BroadcastAttestation(t *testing.T) {
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	if len(p1.BHost.Network().Peers()) == 0 {
		t.Fatal("No peers")
	}

	p := &Service{
		host:                  p1.BHost,
		pubsub:                p1.PubSub(),
		joinedTopics:          map[string]*pubsub.Topic{},
		cfg:                   &Config{},
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
		subnetsLock:           make(map[uint64]*sync.RWMutex),
		subnetsLockLock:       sync.Mutex{},
		peers: peers.NewStatus(t.Context(), &peers.StatusConfig{
			ScorerParams: &scorers.Config{},
		}),
	}

	msg := util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.NewBitlist(7)})
	subnet := uint64(5)

	topic := AttestationSubnetTopicFormat
	GossipTypeMapping[reflect.TypeFor[*ethpb.Attestation]()] = topic
	digest, err := p.currentForkDigest()
	require.NoError(t, err)
	topic = fmt.Sprintf(topic, digest, subnet)

	// External peer subscribes to the topic.
	topic += p.Encoding().ProtocolSuffix()
	sub, err := p2.SubscribeToTopic(topic)
	require.NoError(t, err)

	// Wait for libp2p mesh to establish
	require.Eventually(t, func() bool {
		return len(p.pubsub.ListPeers(topic)) > 0
	}, 5*time.Second, 10*time.Millisecond, "libp2p mesh did not establish")

	// Async listen for the pubsub, must be before the broadcast.
	var wg sync.WaitGroup
	wg.Add(1)
	go func(tt *testing.T) {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(t.Context(), 1*time.Second)
		defer cancel()

		incomingMessage, err := sub.Next(ctx)
		require.NoError(t, err)

		result := &ethpb.Attestation{}
		require.NoError(t, p.Encoding().DecodeGossip(incomingMessage.Data, result))
		if !proto.Equal(result, msg) {
			tt.Errorf("Did not receive expected message, got %+v, wanted %+v", result, msg)
		}
	}(t)

	// Attempt to broadcast nil object should fail.
	ctx := t.Context()
	require.ErrorContains(t, "attempted to broadcast nil", p.BroadcastAttestation(ctx, subnet, nil))

	// Broadcast to peers and wait.
	require.NoError(t, p.BroadcastAttestation(ctx, subnet, msg))
	if util.WaitTimeout(&wg, 1*time.Second) {
		t.Error("Failed to receive pubsub within 1s")
	}
}

func getEventHandler(t *testing.T, s *Service, topic string) *pubsub.TopicEventHandler {
	th, err := s.JoinTopic(topic)
	require.NoError(t, err)
	ev, err := th.EventHandler()
	require.NoError(t, err)
	return ev
}

func TestService_BroadcastAttestationWithDiscoveryAttempts(t *testing.T) {
	const port = uint(2000)

	// The DB has to be shared in all peers to avoid the
	// duplicate metrics collector registration attempted.
	// However, we don't care for this test.
	db := testDB.SetupDB(t)

	// Setup bootnode.
	cfg := &Config{PingInterval: testPingInterval, DB: db}
	cfg.UDPPort = uint(port)
	_, pkey := createAddrAndPrivKey(t)
	ipAddr := net.ParseIP("127.0.0.1")
	genesisTime := time.Now()
	genesisValidatorsRoot := make([]byte, 32)

	s := &Service{
		cfg:                   cfg,
		genesisTime:           genesisTime,
		genesisValidatorsRoot: genesisValidatorsRoot,
		custodyInfo:           &custodyInfo{},
		ctx:                   t.Context(),
		custodyInfoSet:        make(chan struct{}),
	}

	close(s.custodyInfoSet)

	bootListener, err := s.createListener(ipAddr, pkey)
	require.NoError(t, err)
	defer bootListener.Close()

	bootNode := bootListener.Self()
	subnet := uint64(5)

	var listeners []*listenerWrapper
	var hosts []host.Host
	// setup other nodes.
	cfg = &Config{
		Discv5BootStrapAddrs: []string{bootNode.String()},
		MaxPeers:             2,
		PingInterval:         testPingInterval,
		DB:                   db,
	}
	// Setup 2 different hosts
	for i := uint(1); i <= 2; i++ {
		h, pkey, ipAddr := createHost(t, port+i)
		cfg.UDPPort = uint(port + i)
		cfg.TCPPort = uint(port + i)
		if len(listeners) > 0 {
			cfg.Discv5BootStrapAddrs = append(cfg.Discv5BootStrapAddrs, listeners[len(listeners)-1].Self().String())
		}
		s := &Service{
			cfg:                   cfg,
			genesisTime:           genesisTime,
			genesisValidatorsRoot: genesisValidatorsRoot,
			custodyInfo:           &custodyInfo{},
			ctx:                   t.Context(),
			custodyInfoSet:        make(chan struct{}),
		}

		close(s.custodyInfoSet)

		listener, err := s.startDiscoveryV5(ipAddr, pkey)
		// Set for 2nd peer
		if i == 2 {
			s.dv5Listener = listener
			s.metaData = wrapper.WrappedMetadataV0(new(ethpb.MetaDataV0))
			bitV := bitfield.NewBitvector64()
			bitV.SetBitAt(subnet, true)
			err := s.updateSubnetRecordWithMetadata(bitV)
			require.NoError(t, err)
		}
		assert.NoError(t, err, "Could not start discovery for node")
		listeners = append(listeners, listener)
		hosts = append(hosts, h)
	}
	defer func() {
		// Close down all peers.
		for _, listener := range listeners {
			listener.Close()
		}
	}()

	// close peers upon exit of test
	defer func() {
		for _, h := range hosts {
			if err := h.Close(); err != nil {
				t.Log(err)
			}
		}
	}()

	ps1, err := pubsub.NewGossipSub(t.Context(), hosts[0],
		pubsub.WithMessageSigning(false),
		pubsub.WithStrictSignatureVerification(false),
	)
	require.NoError(t, err)

	ps2, err := pubsub.NewGossipSub(t.Context(), hosts[1],
		pubsub.WithMessageSigning(false),
		pubsub.WithStrictSignatureVerification(false),
	)
	require.NoError(t, err)
	p := &Service{
		host:                  hosts[0],
		ctx:                   t.Context(),
		pubsub:                ps1,
		dv5Listener:           listeners[0],
		joinedTopics:          map[string]*pubsub.Topic{},
		cfg:                   cfg,
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
		subnetsLock:           make(map[uint64]*sync.RWMutex),
		subnetsLockLock:       sync.Mutex{},
		peers: peers.NewStatus(t.Context(), &peers.StatusConfig{
			ScorerParams: &scorers.Config{},
		}),
	}

	p2 := &Service{
		host:                  hosts[1],
		ctx:                   t.Context(),
		pubsub:                ps2,
		dv5Listener:           listeners[1],
		joinedTopics:          map[string]*pubsub.Topic{},
		cfg:                   cfg,
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
		subnetsLock:           make(map[uint64]*sync.RWMutex),
		subnetsLockLock:       sync.Mutex{},
		peers: peers.NewStatus(t.Context(), &peers.StatusConfig{
			ScorerParams: &scorers.Config{},
		}),
	}
	go p.listenForNewNodes()
	go p2.listenForNewNodes()

	msg := util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.NewBitlist(7)})
	topic := AttestationSubnetTopicFormat
	GossipTypeMapping[reflect.TypeFor[*ethpb.Attestation]()] = topic
	digest, err := p.currentForkDigest()
	require.NoError(t, err)
	topic = fmt.Sprintf(topic, digest, subnet)

	// External peer subscribes to the topic.
	topic += p.Encoding().ProtocolSuffix()

	ev := getEventHandler(t, p, topic)

	tpHandle, err := p2.JoinTopic(topic)
	require.NoError(t, err)
	sub, err := tpHandle.Subscribe()
	require.NoError(t, err)

	pe, err := ev.NextPeerEvent(t.Context())
	require.NoError(t, err)
	require.Equal(t, pe.Peer, p2.PeerID())
	require.Equal(t, pe.Type, pubsub.PeerJoin)

	// Async listen for the pubsub, must be before the broadcast.
	var wg sync.WaitGroup
	wg.Add(1)
	go func(tt *testing.T) {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
		defer cancel()

		incomingMessage, err := sub.Next(ctx)
		require.NoError(t, err)

		result := &ethpb.Attestation{}
		require.NoError(t, p.Encoding().DecodeGossip(incomingMessage.Data, result))
		if !proto.Equal(result, msg) {
			tt.Errorf("Did not receive expected message, got %+v, wanted %+v", result, msg)
		}
	}(t)

	// Broadcast to peers and wait.
	require.NoError(t, p.BroadcastAttestation(t.Context(), subnet, msg))
	if util.WaitTimeout(&wg, 4*time.Second) {
		t.Error("Failed to receive pubsub within 4s")
	}
}

func TestService_BroadcastSyncCommittee(t *testing.T) {
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	if len(p1.BHost.Network().Peers()) == 0 {
		t.Fatal("No peers")
	}

	p := &Service{
		host:                  p1.BHost,
		pubsub:                p1.PubSub(),
		joinedTopics:          map[string]*pubsub.Topic{},
		cfg:                   &Config{},
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
		subnetsLock:           make(map[uint64]*sync.RWMutex),
		subnetsLockLock:       sync.Mutex{},
		peers: peers.NewStatus(t.Context(), &peers.StatusConfig{
			ScorerParams: &scorers.Config{},
		}),
	}

	msg := util.HydrateSyncCommittee(&ethpb.SyncCommitteeMessage{})
	subnet := uint64(5)

	topic := SyncCommitteeSubnetTopicFormat
	GossipTypeMapping[reflect.TypeFor[*ethpb.SyncCommitteeMessage]()] = topic
	digest, err := p.currentForkDigest()
	require.NoError(t, err)
	topic = fmt.Sprintf(topic, digest, subnet)

	// External peer subscribes to the topic.
	topic += p.Encoding().ProtocolSuffix()
	sub, err := p2.SubscribeToTopic(topic)
	require.NoError(t, err)

	// Wait for libp2p mesh to establish
	require.Eventually(t, func() bool {
		return len(p.pubsub.ListPeers(topic)) > 0
	}, 5*time.Second, 10*time.Millisecond, "libp2p mesh did not establish")

	// Async listen for the pubsub, must be before the broadcast.
	var wg sync.WaitGroup
	wg.Add(1)
	go func(tt *testing.T) {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(t.Context(), 1*time.Second)
		defer cancel()

		incomingMessage, err := sub.Next(ctx)
		require.NoError(t, err)

		result := &ethpb.SyncCommitteeMessage{}
		require.NoError(t, p.Encoding().DecodeGossip(incomingMessage.Data, result))
		if !proto.Equal(result, msg) {
			tt.Errorf("Did not receive expected message, got %+v, wanted %+v", result, msg)
		}
	}(t)

	// Broadcasting nil should fail.
	ctx := t.Context()
	require.ErrorContains(t, "attempted to broadcast nil", p.BroadcastSyncCommitteeMessage(ctx, subnet, nil))

	// Broadcast to peers and wait.
	require.NoError(t, p.BroadcastSyncCommitteeMessage(ctx, subnet, msg))
	if util.WaitTimeout(&wg, 1*time.Second) {
		t.Error("Failed to receive pubsub within 1s")
	}
}

func TestService_BroadcastBlob(t *testing.T) {
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	require.NotEqual(t, 0, len(p1.BHost.Network().Peers()), "No peers")

	p := &Service{
		host:                  p1.BHost,
		pubsub:                p1.PubSub(),
		joinedTopics:          map[string]*pubsub.Topic{},
		cfg:                   &Config{},
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
		subnetsLock:           make(map[uint64]*sync.RWMutex),
		subnetsLockLock:       sync.Mutex{},
		peers: peers.NewStatus(t.Context(), &peers.StatusConfig{
			ScorerParams: &scorers.Config{},
		}),
	}

	header := util.HydrateSignedBeaconHeader(&ethpb.SignedBeaconBlockHeader{})
	commitmentInclusionProof := make([][]byte, 17)
	for i := range commitmentInclusionProof {
		commitmentInclusionProof[i] = bytesutil.PadTo([]byte{}, 32)
	}
	blobSidecar := &ethpb.BlobSidecar{
		Index:                    1,
		Blob:                     bytesutil.PadTo([]byte{'C'}, fieldparams.BlobLength),
		KzgCommitment:            bytesutil.PadTo([]byte{'D'}, fieldparams.BLSPubkeyLength),
		KzgProof:                 bytesutil.PadTo([]byte{'E'}, fieldparams.BLSPubkeyLength),
		SignedBlockHeader:        header,
		CommitmentInclusionProof: commitmentInclusionProof,
	}
	subnet := uint64(0)

	topic := BlobSubnetTopicFormat
	GossipTypeMapping[reflect.TypeFor[*ethpb.BlobSidecar]()] = topic
	digest, err := p.currentForkDigest()
	require.NoError(t, err)
	topic = fmt.Sprintf(topic, digest, subnet)

	// External peer subscribes to the topic.
	topic += p.Encoding().ProtocolSuffix()
	sub, err := p2.SubscribeToTopic(topic)
	require.NoError(t, err)

	// Wait for libp2p mesh to establish
	require.Eventually(t, func() bool {
		return len(p.pubsub.ListPeers(topic)) > 0
	}, 5*time.Second, 10*time.Millisecond, "libp2p mesh did not establish")

	// Async listen for the pubsub, must be before the broadcast.
	var wg sync.WaitGroup
	wg.Add(1)
	go func(tt *testing.T) {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(t.Context(), 1*time.Second)
		defer cancel()

		incomingMessage, err := sub.Next(ctx)
		require.NoError(t, err)

		result := &ethpb.BlobSidecar{}
		require.NoError(t, p.Encoding().DecodeGossip(incomingMessage.Data, result))
		require.DeepEqual(t, result, blobSidecar)
	}(t)

	// Attempt to broadcast nil object should fail.
	ctx := t.Context()
	require.ErrorContains(t, "attempted to broadcast nil", p.BroadcastBlob(ctx, subnet, nil))

	// Broadcast to peers and wait.
	require.NoError(t, p.BroadcastBlob(ctx, subnet, blobSidecar))
	require.Equal(t, false, util.WaitTimeout(&wg, 1*time.Second), "Failed to receive pubsub within 1s")
}

func TestService_BroadcastLightClientOptimisticUpdate(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig().Copy()
	config.SyncMessageDueBPS = 60 // ~72 millisecond
	params.OverrideBeaconConfig(config)

	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	require.NotEqual(t, 0, len(p1.BHost.Network().Peers()))

	p := &Service{
		host:                  p1.BHost,
		pubsub:                p1.PubSub(),
		joinedTopics:          map[string]*pubsub.Topic{},
		cfg:                   &Config{},
		genesisTime:           time.Now().Add(-33 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second), // the signature slot of the mock update is 33
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
		subnetsLock:           make(map[uint64]*sync.RWMutex),
		subnetsLockLock:       sync.Mutex{},
		peers: peers.NewStatus(t.Context(), &peers.StatusConfig{
			ScorerParams: &scorers.Config{},
		}),
	}

	msg, err := util.MockOptimisticUpdate()
	require.NoError(t, err)

	GossipTypeMapping[reflect.TypeOf(msg)] = LightClientOptimisticUpdateTopicFormat
	topic := fmt.Sprintf(LightClientOptimisticUpdateTopicFormat, params.ForkDigest(slots.ToEpoch(msg.AttestedHeader().Beacon().Slot)))

	// External peer subscribes to the topic.
	topic += p.Encoding().ProtocolSuffix()
	sub, err := p2.SubscribeToTopic(topic)
	require.NoError(t, err)

	// Wait for libp2p mesh to establish
	require.Eventually(t, func() bool {
		return len(p.pubsub.ListPeers(topic)) > 0
	}, 5*time.Second, 10*time.Millisecond, "libp2p mesh did not establish")

	// Async listen for the pubsub, must be before the broadcast.
	var wg sync.WaitGroup
	wg.Add(1)
	go func(tt *testing.T) {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
		defer cancel()

		incomingMessage, err := sub.Next(ctx)
		require.NoError(t, err)

		slotStartTime, err := slots.StartTime(p.genesisTime, msg.SignatureSlot())
		require.NoError(t, err)
		expectedDelay := params.BeaconConfig().SlotComponentDuration(params.BeaconConfig().SyncMessageDueBPS)
		if time.Now().Before(slotStartTime.Add(expectedDelay)) {
			tt.Errorf("Message received too early, now %v, expected at least %v", time.Now(), slotStartTime.Add(expectedDelay))
		}

		result := &ethpb.LightClientOptimisticUpdateAltair{}
		require.NoError(t, p.Encoding().DecodeGossip(incomingMessage.Data, result))
		if !proto.Equal(result, msg.Proto()) {
			tt.Errorf("Did not receive expected message, got %+v, wanted %+v", result, msg)
		}
	}(t)

	// Broadcasting nil should fail.
	ctx := t.Context()
	require.ErrorContains(t, "attempted to broadcast nil", p.BroadcastLightClientOptimisticUpdate(ctx, nil))
	var nilUpdate interfaces.LightClientOptimisticUpdate
	require.ErrorContains(t, "attempted to broadcast nil", p.BroadcastLightClientOptimisticUpdate(ctx, nilUpdate))

	// Broadcast to peers and wait.
	require.NoError(t, p.BroadcastLightClientOptimisticUpdate(ctx, msg))
	if util.WaitTimeout(&wg, 1*time.Second) {
		t.Error("Failed to receive pubsub within 1s")
	}
}

func TestService_BroadcastLightClientFinalityUpdate(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig().Copy()
	config.SyncMessageDueBPS = 60 // ~72 millisecond
	params.OverrideBeaconConfig(config)

	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	require.NotEqual(t, 0, len(p1.BHost.Network().Peers()))

	p := &Service{
		host:                  p1.BHost,
		pubsub:                p1.PubSub(),
		joinedTopics:          map[string]*pubsub.Topic{},
		cfg:                   &Config{},
		genesisTime:           time.Now().Add(-33 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second), // the signature slot of the mock update is 33
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
		subnetsLock:           make(map[uint64]*sync.RWMutex),
		subnetsLockLock:       sync.Mutex{},
		peers: peers.NewStatus(t.Context(), &peers.StatusConfig{
			ScorerParams: &scorers.Config{},
		}),
	}

	msg, err := util.MockFinalityUpdate()
	require.NoError(t, err)

	GossipTypeMapping[reflect.TypeOf(msg)] = LightClientFinalityUpdateTopicFormat
	topic := fmt.Sprintf(LightClientFinalityUpdateTopicFormat, params.ForkDigest(slots.ToEpoch(msg.AttestedHeader().Beacon().Slot)))

	// External peer subscribes to the topic.
	topic += p.Encoding().ProtocolSuffix()
	sub, err := p2.SubscribeToTopic(topic)
	require.NoError(t, err)

	// Wait for libp2p mesh to establish
	require.Eventually(t, func() bool {
		return len(p.pubsub.ListPeers(topic)) > 0
	}, 5*time.Second, 10*time.Millisecond, "libp2p mesh did not establish")

	// Async listen for the pubsub, must be before the broadcast.
	var wg sync.WaitGroup
	wg.Add(1)
	go func(tt *testing.T) {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
		defer cancel()

		incomingMessage, err := sub.Next(ctx)
		require.NoError(t, err)

		slotStartTime, err := slots.StartTime(p.genesisTime, msg.SignatureSlot())
		require.NoError(t, err)
		expectedDelay := params.BeaconConfig().SlotComponentDuration(params.BeaconConfig().SyncMessageDueBPS)
		if time.Now().Before(slotStartTime.Add(expectedDelay)) {
			tt.Errorf("Message received too early, now %v, expected at least %v", time.Now(), slotStartTime.Add(expectedDelay))
		}

		result := &ethpb.LightClientFinalityUpdateAltair{}
		require.NoError(t, p.Encoding().DecodeGossip(incomingMessage.Data, result))
		if !proto.Equal(result, msg.Proto()) {
			tt.Errorf("Did not receive expected message, got %+v, wanted %+v", result, msg)
		}
	}(t)

	// Broadcasting nil should fail.
	ctx := t.Context()
	require.ErrorContains(t, "attempted to broadcast nil", p.BroadcastLightClientFinalityUpdate(ctx, nil))
	var nilUpdate interfaces.LightClientFinalityUpdate
	require.ErrorContains(t, "attempted to broadcast nil", p.BroadcastLightClientFinalityUpdate(ctx, nilUpdate))

	// Broadcast to peers and wait.
	require.NoError(t, p.BroadcastLightClientFinalityUpdate(ctx, msg))
	if util.WaitTimeout(&wg, 1*time.Second) {
		t.Error("Failed to receive pubsub within 1s")
	}
}

func TestService_BroadcastDataColumn(t *testing.T) {
	const (
		port        = 2000
		columnIndex = 12
		topicFormat = DataColumnSubnetTopicFormat
	)

	ctx := t.Context()

	// Load the KZG trust setup.
	err := kzg.Start()
	require.NoError(t, err)

	gFlags := new(flags.GlobalFlags)
	gFlags.MinimumPeersPerSubnet = 1
	flags.Init(gFlags)

	// Reset config.
	defer flags.Init(new(flags.GlobalFlags))

	// Create two peers and connect them.
	p1, p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
	p1.Connect(p2)

	// Test the peers are connected.
	require.NotEqual(t, 0, len(p1.BHost.Network().Peers()), "No peers")

	// Create a host.
	_, pkey, ipAddr := createHost(t, port)

	// Create a shared DB for the service
	db := testDB.SetupDB(t)

	// Create and close the custody info channel immediately since custodyInfo is already set
	custodyInfoSet := make(chan struct{})
	close(custodyInfoSet)

	service := &Service{
		ctx:                   ctx,
		host:                  p1.BHost,
		pubsub:                p1.PubSub(),
		joinedTopics:          map[string]*pubsub.Topic{},
		cfg:                   &Config{DB: db},
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
		subnetsLock:           make(map[uint64]*sync.RWMutex),
		subnetsLockLock:       sync.Mutex{},
		peers:                 peers.NewStatus(ctx, &peers.StatusConfig{ScorerParams: &scorers.Config{}}),
		custodyInfo:           &custodyInfo{},
		custodyInfoSet:        custodyInfoSet,
	}

	// Create a listener.
	listener, err := service.startDiscoveryV5(ipAddr, pkey)
	require.NoError(t, err)

	service.dv5Listener = listener

	digest, err := service.currentForkDigest()
	require.NoError(t, err)

	subnet := peerdas.ComputeSubnetForDataColumnSidecar(columnIndex)
	topic := fmt.Sprintf(topicFormat, digest, subnet) + service.Encoding().ProtocolSuffix()

	_, verifiedRoSidecars := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{{Index: columnIndex}})
	verifiedRoSidecar := verifiedRoSidecars[0]

	// Subscribe to the topic.
	sub, err := p2.SubscribeToTopic(topic)
	require.NoError(t, err)

	// Wait for libp2p mesh to establish
	require.Eventually(t, func() bool {
		return len(service.pubsub.ListPeers(topic)) > 0
	}, 5*time.Second, 10*time.Millisecond, "libp2p mesh did not establish")

	// Broadcast to peers and wait.
	err = service.BroadcastDataColumnSidecars(ctx, []blocks.VerifiedRODataColumn{verifiedRoSidecar})
	require.NoError(t, err)

	// Receive the message.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	msg, err := sub.Next(ctx)
	require.NoError(t, err)

	var result ethpb.DataColumnSidecar
	require.NoError(t, service.Encoding().DecodeGossip(msg.Data, &result))
	require.DeepEqual(t, &result, verifiedRoSidecar)
}

type topicInvoked struct {
	topic string
	pid   peer.ID
}

// rpcOrderTracer is a RawTracer implementation that captures the order of SendRPC calls.
// It records the topics of messages sent via pubsub to verify round-robin ordering.
type rpcOrderTracer struct {
	mu      sync.Mutex
	invoked []*topicInvoked
	byTopic map[string][]peer.ID
}

func (t *rpcOrderTracer) SendRPC(rpc *pubsub.RPC, pid peer.ID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, msg := range rpc.GetPublish() {
		invoked := &topicInvoked{topic: msg.GetTopic(), pid: pid}
		t.invoked = append(t.invoked, invoked)
		t.byTopic[invoked.topic] = append(t.byTopic[invoked.topic], invoked.pid)
	}
}

func newRpcOrderTracer() *rpcOrderTracer {
	return &rpcOrderTracer{byTopic: make(map[string][]peer.ID)}
}

func (t *rpcOrderTracer) getTopics() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]string, len(t.invoked))
	for i := range t.invoked {
		result[i] = t.invoked[i].topic
	}
	return result
}

// No-op implementations for other RawTracer methods.
func (*rpcOrderTracer) AddPeer(peer.ID, protocol.ID)          {}
func (*rpcOrderTracer) RemovePeer(peer.ID)                    {}
func (*rpcOrderTracer) Join(string)                           {}
func (*rpcOrderTracer) Leave(string)                          {}
func (*rpcOrderTracer) Graft(peer.ID, string)                 {}
func (*rpcOrderTracer) Prune(peer.ID, string)                 {}
func (*rpcOrderTracer) ValidateMessage(*pubsub.Message)       {}
func (*rpcOrderTracer) DeliverMessage(*pubsub.Message)        {}
func (*rpcOrderTracer) RejectMessage(*pubsub.Message, string) {}
func (*rpcOrderTracer) DuplicateMessage(*pubsub.Message)      {}
func (*rpcOrderTracer) ThrottlePeer(peer.ID)                  {}
func (*rpcOrderTracer) RecvRPC(*pubsub.RPC)                   {}
func (*rpcOrderTracer) DropRPC(*pubsub.RPC, peer.ID)          {}
func (*rpcOrderTracer) UndeliverableMessage(*pubsub.Message)  {}

// TestService_BroadcastDataColumnRoundRobin verifies that when broadcasting multiple
// data column sidecars, messages are interleaved in round-robin order by column index
// rather than sending all copies of one column before the next.
//
// Without batch publishing: A,A,A,A,B,B,B,B (all peers for column A, then all for column B)
// With batch publishing:    A,B,A,B,A,B,A,B (interleaved by message ID)
func TestService_BroadcastDataColumnRoundRobin(t *testing.T) {
	const (
		port        = 2100
		topicFormat = DataColumnSubnetTopicFormat
	)

	ctx := t.Context()

	// Load the KZG trust setup.
	err := kzg.Start()
	require.NoError(t, err)

	gFlags := new(flags.GlobalFlags)
	gFlags.MinimumPeersPerSubnet = 1
	flags.Init(gFlags)
	defer flags.Init(new(flags.GlobalFlags))

	// Create a tracer to capture the order of SendRPC calls.
	tracer := newRpcOrderTracer()

	// Create the publisher node with the tracer injected.
	p1 := p2ptest.NewTestP2PWithPubsubOptions(t, []pubsub.Option{pubsub.WithRawTracer(tracer)})

	// Create subscriber peers.
	expectedPeers := []*p2ptest.TestP2P{
		p2ptest.NewTestP2P(t),
		p2ptest.NewTestP2P(t),
	}

	// Connect peers.
	for _, p := range expectedPeers {
		p1.Connect(p)
	}
	require.NotEqual(t, 0, len(p1.BHost.Network().Peers()), "No peers")

	// Create a host for discovery.
	_, pkey, ipAddr := createHost(t, port)

	// Create a shared DB for the service.
	db := testDB.SetupDB(t)

	// Create and close the custody info channel immediately since custodyInfo is already set.
	custodyInfoSet := make(chan struct{})
	close(custodyInfoSet)

	service := &Service{
		ctx:                   ctx,
		host:                  p1.BHost,
		pubsub:                p1.PubSub(),
		joinedTopics:          map[string]*pubsub.Topic{},
		cfg:                   &Config{DB: db},
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
		subnetsLock:           make(map[uint64]*sync.RWMutex),
		subnetsLockLock:       sync.Mutex{},
		peers:                 peers.NewStatus(ctx, &peers.StatusConfig{ScorerParams: &scorers.Config{}}),
		custodyInfo:           &custodyInfo{},
		custodyInfoSet:        custodyInfoSet,
	}

	// Create a listener for discovery.
	listener, err := service.startDiscoveryV5(ipAddr, pkey)
	require.NoError(t, err)
	service.dv5Listener = listener

	digest, err := service.currentForkDigest()
	require.NoError(t, err)

	// Create multiple data column sidecars with different column indices.
	// Use indices that map to different subnets: 0, 32, 64 (assuming 128 columns and 64 subnets).
	columnIndices := []uint64{0, 32, 64}
	params := make([]util.DataColumnParam, len(columnIndices))
	for i, idx := range columnIndices {
		params[i] = util.DataColumnParam{Index: idx}
	}
	_, verifiedRoSidecars := util.CreateTestVerifiedRoDataColumnSidecars(t, params)

	expectedTopics := make(map[string]bool)
	// Subscribe peers to the relevant topics.
	for _, idx := range columnIndices {
		subnet := peerdas.ComputeSubnetForDataColumnSidecar(idx)
		topic := fmt.Sprintf(topicFormat, digest, subnet) + service.Encoding().ProtocolSuffix()
		for _, p := range expectedPeers {
			_, err = p.SubscribeToTopic(topic)
			require.NoError(t, err)
		}
		expectedTopics[topic] = true
	}
	// libp2p needs some time to establish mesh connections.
	time.Sleep(100 * time.Millisecond)

	// Broadcast all sidecars.
	err = service.BroadcastDataColumnSidecars(ctx, verifiedRoSidecars)
	require.NoError(t, err)
	// Give some time for messages to be sent.
	time.Sleep(100 * time.Millisecond)

	topics := tracer.getTopics()
	if len(topics) == 0 {
		t.Fatal("Expected at least one message for each topic to be sent to each peer")
	}

	unseen := make(map[string]bool)
	for k := range expectedTopics {
		unseen[k] = true
	}
	// Verify round-robin invariant: before all message IDs are seen, no message ID may be repeated.
	// In round-robin order, we should see each topic once before any topic repeats.
	for _, topic := range topics {
		if !expectedTopics[topic] {
			continue
		}
		if !unseen[topic] {
			t.Errorf("Topic %s repeated before all topics were seen once. This violates round-robin ordering.", topic)
		}
		delete(unseen, topic)
		if len(unseen) == 0 {
			break // all have been seen
		}
	}
	require.Equal(t, 0, len(unseen))

	// Verify that we actually saw all expected topics.
	for topic := range expectedTopics {
		require.Equal(t, len(expectedPeers), len(tracer.byTopic[topic]))
	}
}
