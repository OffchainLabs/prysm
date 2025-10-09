package sync

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/abool"
	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	db "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/slashings"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/encoder"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	lruwrpr "github.com/OffchainLabs/prysm/v6/cache/lru"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"google.golang.org/protobuf/proto"
)

func TestSubscribe_ReceivesValidMessage(t *testing.T) {
	p2pService := p2ptest.NewTestP2P(t)
	gt := time.Now()
	vr := [32]byte{'A'}
	r := Service{
		ctx: t.Context(),
		cfg: &config{
			p2p:         p2pService,
			initialSync: &mockSync.Sync{IsSyncing: false},
			chain: &mockChain.ChainService{
				ValidatorsRoot: vr,
				Genesis:        gt,
			},
			clock: startup.NewClock(gt, vr),
		},
		subHandler:   newSubTopicHandler(),
		chainStarted: abool.New(),
	}
	markInitSyncComplete(t, &r)
	var err error
	require.NoError(t, err)
	nse := params.GetNetworkScheduleEntry(r.cfg.clock.CurrentEpoch())
	p2pService.Digest = nse.ForkDigest
	topic := "/eth2/%x/voluntary_exit"
	var wg sync.WaitGroup
	wg.Add(1)

	r.subscribe(topic, r.noopValidator, func(_ context.Context, msg proto.Message) error {
		m, ok := msg.(*pb.SignedVoluntaryExit)
		assert.Equal(t, true, ok, "Object is not of type *pb.SignedVoluntaryExit")
		if m.Exit == nil || m.Exit.Epoch != 55 {
			t.Errorf("Unexpected incoming message: %+v", m)
		}
		wg.Done()
		return nil
	}, nse)
	r.markForChainStart()

	p2pService.ReceivePubSub(topic, &pb.SignedVoluntaryExit{Exit: &pb.VoluntaryExit{Epoch: 55}, Signature: make([]byte, fieldparams.BLSSignatureLength)})

	if util.WaitTimeout(&wg, time.Second) {
		t.Fatal("Did not receive PubSub in 1 second")
	}
}

func markInitSyncComplete(_ *testing.T, s *Service) {
	s.initialSyncComplete = make(chan struct{})
	close(s.initialSyncComplete)
}

func TestSubscribe_UnsubscribeTopic(t *testing.T) {
	p2pService := p2ptest.NewTestP2P(t)
	gt := time.Now()
	vr := [32]byte{'A'}
	r := Service{
		ctx: t.Context(),
		cfg: &config{
			p2p:         p2pService,
			initialSync: &mockSync.Sync{IsSyncing: false},
			chain: &mockChain.ChainService{
				ValidatorsRoot: vr,
				Genesis:        gt,
			},
			clock: startup.NewClock(gt, vr),
		},
		chainStarted: abool.New(),
		subHandler:   newSubTopicHandler(),
	}
	markInitSyncComplete(t, &r)
	nse := params.GetNetworkScheduleEntry(r.cfg.clock.CurrentEpoch())
	p2pService.Digest = nse.ForkDigest
	topic := "/eth2/%x/voluntary_exit"

	r.subscribe(topic, r.noopValidator, func(_ context.Context, msg proto.Message) error {
		return nil
	}, nse)
	r.markForChainStart()

	fullTopic := fmt.Sprintf(topic, p2pService.Digest) + p2pService.Encoding().ProtocolSuffix()
	assert.Equal(t, true, r.subHandler.topicExists(fullTopic))
	topics := p2pService.PubSub().GetTopics()
	assert.Equal(t, fullTopic, topics[0])

	r.unSubscribeFromTopic(fullTopic)

	assert.Equal(t, false, r.subHandler.topicExists(fullTopic))
	assert.Equal(t, 0, len(p2pService.PubSub().GetTopics()))

}

func TestSubscribe_ReceivesAttesterSlashing(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig()
	cfg.SecondsPerSlot = 1
	params.OverrideBeaconConfig(cfg)

	p2pService := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	d := db.SetupDB(t)
	gt := time.Now()
	vr := [32]byte{'A'}
	chainService := &mockChain.ChainService{
		Genesis:        gt,
		ValidatorsRoot: vr,
	}
	r := Service{
		ctx: ctx,
		cfg: &config{
			p2p:          p2pService,
			initialSync:  &mockSync.Sync{IsSyncing: false},
			slashingPool: slashings.NewPool(),
			chain:        chainService,
			clock:        startup.NewClock(gt, vr),
			beaconDB:     d,
		},
		seenAttesterSlashingCache: make(map[uint64]bool),
		chainStarted:              abool.New(),
		subHandler:                newSubTopicHandler(),
	}
	markInitSyncComplete(t, &r)
	topic := "/eth2/%x/attester_slashing"
	var wg sync.WaitGroup
	wg.Add(1)
	nse := params.GetNetworkScheduleEntry(r.cfg.clock.CurrentEpoch())
	p2pService.Digest = nse.ForkDigest
	r.subscribe(topic, r.noopValidator, func(ctx context.Context, msg proto.Message) error {
		require.NoError(t, r.attesterSlashingSubscriber(ctx, msg))
		wg.Done()
		return nil
	}, nse)
	beaconState, privKeys := util.DeterministicGenesisState(t, 64)
	chainService.State = beaconState
	r.markForChainStart()
	attesterSlashing, err := util.GenerateAttesterSlashingForValidator(
		beaconState,
		privKeys[1],
		1, /* validator index */
	)
	require.NoError(t, err, "Error generating attester slashing")
	err = r.cfg.beaconDB.SaveState(ctx, beaconState, bytesutil.ToBytes32(attesterSlashing.FirstAttestation().GetData().BeaconBlockRoot))
	require.NoError(t, err)
	p2pService.ReceivePubSub(topic, attesterSlashing)

	if util.WaitTimeout(&wg, time.Second) {
		t.Fatal("Did not receive PubSub in 1 second")
	}
	as := r.cfg.slashingPool.PendingAttesterSlashings(ctx, beaconState, false /*noLimit*/)
	assert.Equal(t, 1, len(as), "Expected attester slashing")
}

func TestSubscribe_ReceivesProposerSlashing(t *testing.T) {
	p2pService := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	chainService := &mockChain.ChainService{
		ValidatorsRoot: [32]byte{'A'},
		Genesis:        time.Now(),
	}
	d := db.SetupDB(t)
	r := Service{
		ctx: ctx,
		cfg: &config{
			p2p:          p2pService,
			initialSync:  &mockSync.Sync{IsSyncing: false},
			slashingPool: slashings.NewPool(),
			chain:        chainService,
			beaconDB:     d,
			clock:        startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
		},
		seenProposerSlashingCache: lruwrpr.New(10),
		chainStarted:              abool.New(),
		subHandler:                newSubTopicHandler(),
	}
	markInitSyncComplete(t, &r)
	topic := "/eth2/%x/proposer_slashing"
	var wg sync.WaitGroup
	wg.Add(1)
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig())
	nse := params.GetNetworkScheduleEntry(r.cfg.clock.CurrentEpoch())
	p2pService.Digest = nse.ForkDigest
	r.subscribe(topic, r.noopValidator, func(ctx context.Context, msg proto.Message) error {
		require.NoError(t, r.proposerSlashingSubscriber(ctx, msg))
		wg.Done()
		return nil
	}, nse)
	beaconState, privKeys := util.DeterministicGenesisState(t, 64)
	chainService.State = beaconState
	r.markForChainStart()
	proposerSlashing, err := util.GenerateProposerSlashingForValidator(
		beaconState,
		privKeys[1],
		1, /* validator index */
	)
	require.NoError(t, err, "Error generating proposer slashing")

	p2pService.ReceivePubSub(topic, proposerSlashing)

	if util.WaitTimeout(&wg, time.Second) {
		t.Fatal("Did not receive PubSub in 1 second")
	}
	ps := r.cfg.slashingPool.PendingProposerSlashings(ctx, beaconState, false /*noLimit*/)
	assert.Equal(t, 1, len(ps), "Expected proposer slashing")
}

func TestSubscribe_HandlesPanic(t *testing.T) {
	p := p2ptest.NewTestP2P(t)
	chain := &mockChain.ChainService{
		Genesis:        time.Now(),
		ValidatorsRoot: [32]byte{'A'},
	}
	r := Service{
		ctx: t.Context(),
		cfg: &config{
			chain: chain,
			clock: startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			p2p:   p,
		},
		subHandler:   newSubTopicHandler(),
		chainStarted: abool.New(),
	}
	markInitSyncComplete(t, &r)

	nse := params.GetNetworkScheduleEntry(r.cfg.clock.CurrentEpoch())
	p.Digest = nse.ForkDigest

	topic := p2p.GossipTypeMapping[reflect.TypeOf(&pb.SignedVoluntaryExit{})]
	var wg sync.WaitGroup
	wg.Add(1)

	r.subscribe(topic, r.noopValidator, func(_ context.Context, msg proto.Message) error {
		defer wg.Done()
		panic("bad")
	}, nse)
	r.markForChainStart()
	p.ReceivePubSub(topic, &pb.SignedVoluntaryExit{Exit: &pb.VoluntaryExit{Epoch: 55}, Signature: make([]byte, fieldparams.BLSSignatureLength)})

	if util.WaitTimeout(&wg, time.Second) {
		t.Fatal("Did not receive PubSub in 1 second")
	}
}

func TestRevalidateSubscription_CorrectlyFormatsTopic(t *testing.T) {
	p := p2ptest.NewTestP2P(t)
	hook := logTest.NewGlobal()
	chain := &mockChain.ChainService{
		Genesis:        time.Now(),
		ValidatorsRoot: [32]byte{'A'},
	}
	r := Service{
		ctx: t.Context(),
		cfg: &config{
			chain: chain,
			clock: startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			p2p:   p,
		},
		chainStarted: abool.New(),
		subHandler:   newSubTopicHandler(),
	}
	nse := params.GetNetworkScheduleEntry(r.cfg.clock.CurrentEpoch())

	params := subscribeParameters{
		topicFormat: "/eth2/testing/%#x/committee%d",
		nse:         nse,
	}
	tracker := newSubnetTracker(params)

	// committee index 1
	c1 := uint64(1)
	fullTopic := params.fullTopic(c1, r.cfg.p2p.Encoding().ProtocolSuffix())
	_, topVal := r.wrapAndReportValidation(fullTopic, r.noopValidator)
	require.NoError(t, r.cfg.p2p.PubSub().RegisterTopicValidator(fullTopic, topVal))
	sub1, err := r.cfg.p2p.SubscribeToTopic(fullTopic)
	require.NoError(t, err)
	tracker.track(c1, sub1)

	// committee index 2
	c2 := uint64(2)
	fullTopic = params.fullTopic(c2, r.cfg.p2p.Encoding().ProtocolSuffix())
	_, topVal = r.wrapAndReportValidation(fullTopic, r.noopValidator)
	err = r.cfg.p2p.PubSub().RegisterTopicValidator(fullTopic, topVal)
	require.NoError(t, err)
	sub2, err := r.cfg.p2p.SubscribeToTopic(fullTopic)
	require.NoError(t, err)
	tracker.track(c2, sub2)

	r.pruneNotWanted(tracker, map[uint64]bool{c2: true})
	require.LogsDoNotContain(t, hook, "Could not unregister topic validator")
}

func Test_wrapAndReportValidation(t *testing.T) {
	mChain := &mockChain.ChainService{
		Genesis:        time.Now(),
		ValidatorsRoot: [32]byte{0x01},
	}
	clock := startup.NewClock(mChain.Genesis, mChain.ValidatorsRoot)
	fd := params.ForkDigest(clock.CurrentEpoch())
	mockTopic := fmt.Sprintf(p2p.BlockSubnetTopicFormat, fd) + encoder.SszNetworkEncoder{}.ProtocolSuffix()
	type args struct {
		topic        string
		v            wrappedVal
		chainstarted bool
		pid          peer.ID
		msg          *pubsub.Message
	}
	tests := []struct {
		name string
		args args
		want pubsub.ValidationResult
	}{
		{
			name: "validator Before chainstart",
			args: args{
				topic: "foo",
				v: func(ctx context.Context, id peer.ID, message *pubsub.Message) (pubsub.ValidationResult, error) {
					return pubsub.ValidationAccept, nil
				},
				msg: &pubsub.Message{
					Message: &pubsubpb.Message{
						Topic: func() *string {
							s := "foo"
							return &s
						}(),
					},
				},
				chainstarted: false,
			},
			want: pubsub.ValidationIgnore,
		},
		{
			name: "validator panicked",
			args: args{
				topic: "foo",
				v: func(ctx context.Context, id peer.ID, message *pubsub.Message) (pubsub.ValidationResult, error) {
					panic("oh no!")
				},
				chainstarted: true,
				msg: &pubsub.Message{
					Message: &pubsubpb.Message{
						Topic: func() *string {
							s := "foo"
							return &s
						}(),
					},
				},
			},
			want: pubsub.ValidationIgnore,
		},
		{
			name: "validator OK",
			args: args{
				topic: mockTopic,
				v: func(ctx context.Context, id peer.ID, message *pubsub.Message) (pubsub.ValidationResult, error) {
					return pubsub.ValidationAccept, nil
				},
				chainstarted: true,
				msg: &pubsub.Message{
					Message: &pubsubpb.Message{
						Topic: func() *string {
							s := mockTopic
							return &s
						}(),
					},
				},
			},
			want: pubsub.ValidationAccept,
		},
		{
			name: "nil topic",
			args: args{
				topic: "foo",
				v: func(ctx context.Context, id peer.ID, message *pubsub.Message) (pubsub.ValidationResult, error) {
					return pubsub.ValidationAccept, nil
				},
				msg: &pubsub.Message{
					Message: &pubsubpb.Message{
						Topic: nil,
					},
				},
			},
			want: pubsub.ValidationReject,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chainStarted := abool.New()
			chainStarted.SetTo(tt.args.chainstarted)
			s := &Service{
				chainStarted: chainStarted,
				cfg: &config{
					chain: mChain,
					clock: startup.NewClock(mChain.Genesis, mChain.ValidatorsRoot),
				},
				subHandler: newSubTopicHandler(),
			}
			_, v := s.wrapAndReportValidation(tt.args.topic, tt.args.v)
			got := v(t.Context(), tt.args.pid, tt.args.msg)
			if got != tt.want {
				t.Errorf("wrapAndReportValidation() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterSubnetPeers(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig()
	cfg.SecondsPerSlot = 1
	params.OverrideBeaconConfig(cfg)

	gFlags := new(flags.GlobalFlags)
	gFlags.MinimumPeersPerSubnet = 4
	flags.Init(gFlags)
	// Reset config.
	defer flags.Init(new(flags.GlobalFlags))
	p := p2ptest.NewTestP2P(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	currSlot := primitives.Slot(100)

	gt := time.Now()
	genPlus100 := func() time.Time {
		return gt.Add(time.Second * time.Duration(uint64(currSlot)*params.BeaconConfig().SecondsPerSlot))
	}
	chain := &mockChain.ChainService{
		Genesis:        gt,
		ValidatorsRoot: [32]byte{'A'},
		FinalizedRoots: map[[32]byte]bool{
			{}: true,
		},
	}
	clock := startup.NewClock(chain.Genesis, chain.ValidatorsRoot, startup.WithNower(genPlus100))
	require.Equal(t, currSlot, clock.CurrentSlot())
	r := Service{
		ctx: ctx,
		cfg: &config{
			chain: chain,
			clock: clock,
			p2p:   p,
		},
		chainStarted: abool.New(),
		subHandler:   newSubTopicHandler(),
	}
	markInitSyncComplete(t, &r)
	// Empty cache at the end of the test.
	defer cache.SubnetIDs.EmptyAllCaches()
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	defaultTopic := "/eth2/%x/beacon_attestation_%d" + r.cfg.p2p.Encoding().ProtocolSuffix()
	subnet10 := r.addDigestAndIndexToTopic(defaultTopic, digest, 10)
	cache.SubnetIDs.AddAggregatorSubnetID(currSlot, 10)

	subnet20 := r.addDigestAndIndexToTopic(defaultTopic, digest, 20)
	cache.SubnetIDs.AddAttesterSubnetID(currSlot, 20)

	p1 := createPeer(t, subnet10)
	p2 := createPeer(t, subnet10, subnet20)
	p3 := createPeer(t)

	// Connect to all peers.
	p.Connect(p1)
	p.Connect(p2)
	p.Connect(p3)

	// Sleep a while to allow peers to connect.
	time.Sleep(100 * time.Millisecond)

	wantedPeers := []peer.ID{p1.PeerID(), p2.PeerID(), p3.PeerID()}
	// Expect Peer 3 to be marked as suitable.
	recPeers := r.filterNeededPeers(wantedPeers)
	assert.DeepEqual(t, []peer.ID{p3.PeerID()}, recPeers)

	// Try with only peers from subnet 20.
	wantedPeers = []peer.ID{p2.BHost.ID()}
	// Connect an excess amount of peers in the particular subnet.
	for i := 1; i <= flags.Get().MinimumPeersPerSubnet; i++ {
		nPeer := createPeer(t, subnet20)
		p.Connect(nPeer)
		wantedPeers = append(wantedPeers, nPeer.BHost.ID())
		time.Sleep(100 * time.Millisecond)
	}

	recPeers = r.filterNeededPeers(wantedPeers)
	assert.Equal(t, 1, len(recPeers), "expected at least 1 suitable peer to prune")
}

func TestSubscribeWithSyncSubnets_DynamicOK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig()
	cfg.SecondsPerSlot = 1
	params.OverrideBeaconConfig(cfg)

	p := p2ptest.NewTestP2P(t)
	ctx, cancel := context.WithCancel(t.Context())
	gt := time.Now()
	vr := [32]byte{'A'}
	r := Service{
		ctx: ctx,
		cfg: &config{
			chain: &mockChain.ChainService{
				Genesis:        gt,
				ValidatorsRoot: vr,
			},
			p2p:   p,
			clock: startup.NewClock(gt, vr),
		},
		chainStarted: abool.New(),
		subHandler:   newSubTopicHandler(),
	}
	markInitSyncComplete(t, &r)
	// Empty cache at the end of the test.
	defer cache.SyncSubnetIDs.EmptyAllCaches()
	slot := r.cfg.clock.CurrentSlot()
	currEpoch := slots.ToEpoch(slot)
	cache.SyncSubnetIDs.AddSyncCommitteeSubnets([]byte("pubkey"), currEpoch, []uint64{0, 1}, 10*time.Second)
	nse := params.GetNetworkScheduleEntry(r.cfg.clock.CurrentEpoch())
	go r.subscribeWithParameters(subscribeParameters{
		topicFormat:      p2p.SyncCommitteeSubnetTopicFormat,
		nse:              nse,
		getSubnetsToJoin: r.activeSyncSubnetIndices,
	})
	time.Sleep(2 * time.Second)
	assert.Equal(t, 2, len(r.cfg.p2p.PubSub().GetTopics()))
	topicMap := map[string]bool{}
	for _, t := range r.cfg.p2p.PubSub().GetTopics() {
		topicMap[t] = true
	}
	firstSub := fmt.Sprintf(p2p.SyncCommitteeSubnetTopicFormat, nse.ForkDigest, 0) + r.cfg.p2p.Encoding().ProtocolSuffix()
	assert.Equal(t, true, topicMap[firstSub])

	secondSub := fmt.Sprintf(p2p.SyncCommitteeSubnetTopicFormat, nse.ForkDigest, 1) + r.cfg.p2p.Encoding().ProtocolSuffix()
	assert.Equal(t, true, topicMap[secondSub])
	cancel()
}

func TestSubscribeWithSyncSubnets_DynamicSwitchFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().InitializeForkSchedule()
	p := p2ptest.NewTestP2P(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	vr := params.BeaconConfig().GenesisValidatorsRoot
	mockNow := &startup.MockNower{}
	clock := startup.NewClock(time.Now(), vr, startup.WithNower(mockNow.Now))
	denebSlot, err := slots.EpochStart(params.BeaconConfig().DenebForkEpoch)
	require.NoError(t, err)
	mockNow.SetSlot(t, clock, denebSlot)
	r := Service{
		ctx: ctx,
		cfg: &config{
			chain: &mockChain.ChainService{},
			clock: clock,
			p2p:   p,
		},
		chainStarted: abool.New(),
		subHandler:   newSubTopicHandler(),
	}
	markInitSyncComplete(t, &r)
	// Empty cache at the end of the test.
	defer cache.SyncSubnetIDs.EmptyAllCaches()
	cache.SyncSubnetIDs.AddSyncCommitteeSubnets([]byte("pubkey"), 0, []uint64{0, 1}, 10*time.Second)
	nse := params.GetNetworkScheduleEntry(r.cfg.clock.CurrentEpoch())
	require.Equal(t, [4]byte(params.BeaconConfig().DenebForkVersion), nse.ForkVersion)
	require.Equal(t, params.BeaconConfig().DenebForkEpoch, nse.Epoch)

	sp := newSubnetTracker(subscribeParameters{
		topicFormat:      p2p.SyncCommitteeSubnetTopicFormat,
		nse:              nse,
		getSubnetsToJoin: r.activeSyncSubnetIndices,
	})
	r.trySubscribeSubnets(sp)
	assert.Equal(t, 2, len(r.cfg.p2p.PubSub().GetTopics()))
	topicMap := map[string]bool{}
	for _, t := range r.cfg.p2p.PubSub().GetTopics() {
		topicMap[t] = true
	}
	firstSub := fmt.Sprintf(p2p.SyncCommitteeSubnetTopicFormat, nse.ForkDigest, 0) + r.cfg.p2p.Encoding().ProtocolSuffix()
	assert.Equal(t, true, topicMap[firstSub])

	secondSub := fmt.Sprintf(p2p.SyncCommitteeSubnetTopicFormat, nse.ForkDigest, 1) + r.cfg.p2p.Encoding().ProtocolSuffix()
	assert.Equal(t, true, topicMap[secondSub])

	electraSlot, err := slots.EpochStart(params.BeaconConfig().ElectraForkEpoch)
	require.NoError(t, err)
	mockNow.SetSlot(t, clock, electraSlot)
	nse = params.GetNetworkScheduleEntry(r.cfg.clock.CurrentEpoch())
	require.Equal(t, [4]byte(params.BeaconConfig().ElectraForkVersion), nse.ForkVersion)
	require.Equal(t, params.BeaconConfig().ElectraForkEpoch, nse.Epoch)

	sp.nse = nse
	// clear the cache and re-subscribe to subnets.
	// this should result in the subscriptions being removed
	cache.SyncSubnetIDs.EmptyAllCaches()
	r.trySubscribeSubnets(sp)
	assert.Equal(t, 0, len(r.cfg.p2p.PubSub().GetTopics()))
}

func TestIsDigestValid(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().InitializeForkSchedule()
	clock := startup.NewClock(time.Now().Add(-100*time.Second), params.BeaconConfig().GenesisValidatorsRoot)
	digest, err := signing.ComputeForkDigest(params.BeaconConfig().GenesisForkVersion, params.BeaconConfig().GenesisValidatorsRoot[:])
	assert.NoError(t, err)
	valid, err := isDigestValid(digest, clock)
	assert.NoError(t, err)
	assert.Equal(t, true, valid)

	// Compute future fork digest that will be invalid currently.
	digest, err = signing.ComputeForkDigest(params.BeaconConfig().AltairForkVersion, params.BeaconConfig().GenesisValidatorsRoot[:])
	assert.NoError(t, err)
	valid, err = isDigestValid(digest, clock)
	assert.NoError(t, err)
	assert.Equal(t, false, valid)
}

// Create peer and register them to provided topics.
func createPeer(t *testing.T, topics ...string) *p2ptest.TestP2P {
	p := p2ptest.NewTestP2P(t)
	for _, tp := range topics {
		jTop, err := p.PubSub().Join(tp)
		if err != nil {
			t.Fatal(err)
		}
		_, err = jTop.Subscribe()
		if err != nil {
			t.Fatal(err)
		}
	}
	return p
}
