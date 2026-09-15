package p2p

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/encoder"
	testp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
)

func TestService_PublishToTopic_Concurrent(t *testing.T) {
	cs := startup.NewClockSynchronizer()
	s, err := NewService(t.Context(), &Config{
		StateNotifier: &mock.MockStateNotifier{},
		ClockWaiter:   cs,
		DB:            testDB.SetupDB(t),
	})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	go s.awaitStateInitialized()
	fd := initializeStateWithForkDigest(ctx, t, cs)

	if !s.isInitialized() {
		t.Fatal("service was not initialized")
	}

	// Set up two connected test hosts.
	p0 := testp2p.NewTestP2P(t)
	p1 := testp2p.NewTestP2P(t)
	p0.Connect(p1)
	s.host = p0.BHost
	s.pubsub = p0.PubSub()

	topic := fmt.Sprintf(BlockSubnetTopicFormat, fd) + "/" + encoder.ProtocolSuffixSSZSnappy

	// Establish the remote peer to be subscribed to the outgoing topic.
	_, err = p1.SubscribeToTopic(topic)
	require.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(10)
	for i := range 10 {
		go func(i int) {
			assert.NoError(t, s.PublishToTopic(ctx, topic, []byte{}))
			wg.Done()
		}(i)
	}
	wg.Wait()
}

func TestService_PublishToTopic_CancelWhileWaitingForPeers(t *testing.T) {
	defer flags.Init(flags.Get())
	flags.Init(&flags.GlobalFlags{MinimumSyncPeers: 1})

	p := testp2p.NewTestP2P(t)
	t.Cleanup(func() { require.NoError(t, p.BHost.Close()) })
	s := &Service{pubsub: p.PubSub(), joinedTopics: make(map[string]*pubsub.Topic)}
	topic := fmt.Sprintf(BlockSubnetTopicFormat, [4]byte{}) + "/" + encoder.ProtocolSuffixSSZSnappy
	handle, err := s.JoinTopic(topic)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.LeaveTopic(topic)) })
	require.Equal(t, 0, len(handle.ListPeers()))

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// Cancel during the 100ms peer wait, with room for scheduler delays.
	timer := time.AfterFunc(10*time.Millisecond, cancel)
	defer timer.Stop()
	start := time.Now()
	err = s.PublishToTopic(ctx, topic, []byte{})
	elapsed := time.Since(start)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorContains(t, fmt.Sprintf("unable to find requisite number of peers for topic %s, 0 peers found to publish to", topic), err)
	assert.Equal(t, true, elapsed < 75*time.Millisecond, "cancellation took %s", elapsed)
}

func TestExtractGossipDigest(t *testing.T) {
	tests := []struct {
		name    string
		topic   string
		want    [4]byte
		wantErr bool
		error   error
	}{
		{
			name:    "empty topic",
			topic:   "",
			want:    [4]byte{},
			wantErr: true,
			error:   errors.New("invalid topic format"),
		},
		{
			name:    "too short topic",
			topic:   "/eth2/",
			want:    [4]byte{},
			wantErr: true,
			error:   errors.New("invalid topic format"),
		},
		{
			name:    "bogus topic prefix",
			topic:   "/eth3/b5303f2a/beacon_coin",
			want:    [4]byte{},
			wantErr: true,
			error:   errors.New("invalid topic format"),
		},
		{
			name:    "invalid digest in topic",
			topic:   "/eth2/zzxxyyaa/beacon_block" + "/" + encoder.ProtocolSuffixSSZSnappy,
			want:    [4]byte{},
			wantErr: true,
			error:   errors.New("encoding/hex: invalid byte"),
		},
		{
			name:    "short digest",
			topic:   fmt.Sprintf(BlockSubnetTopicFormat, []byte{0xb5, 0x30, 0x3f}) + "/" + encoder.ProtocolSuffixSSZSnappy,
			want:    [4]byte{},
			wantErr: true,
			error:   errors.New("invalid digest length wanted"),
		},
		{
			name:    "too short topic, missing suffixes",
			topic:   "/eth2/b5303f2a",
			want:    [4]byte{},
			wantErr: true,
			error:   errors.New("invalid topic format"),
		},
		{
			name:    "valid topic",
			topic:   fmt.Sprintf(BlockSubnetTopicFormat, []byte{0xb5, 0x30, 0x3f, 0x2a}) + "/" + encoder.ProtocolSuffixSSZSnappy,
			want:    [4]byte{0xb5, 0x30, 0x3f, 0x2a},
			wantErr: false,
			error:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractGossipDigest(tt.topic)
			assert.Equal(t, err != nil, tt.wantErr)
			if tt.wantErr {
				assert.ErrorContains(t, tt.error.Error(), err)
			}
			assert.DeepEqual(t, tt.want, got)
		})
	}
}

func BenchmarkExtractGossipDigest(b *testing.B) {
	topic := fmt.Sprintf(BlockSubnetTopicFormat, []byte{0xb5, 0x30, 0x3f, 0x2a}) + "/" + encoder.ProtocolSuffixSSZSnappy

	for b.Loop() {
		_, err := ExtractGossipDigest(topic)
		if err != nil {
			b.Fatal(err)
		}
	}
}
