package sync

import (
	"context"
	"testing"

	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
)

func TestValidateLightClientOptimisticUpdate_NilMessageOrTopic(t *testing.T) {
	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	s := &Service{cfg: &config{p2p: p, initialSync: &mockSync.Sync{}}}

	_, err := s.validateLightClientOptimisticUpdate(ctx, "", nil)
	require.ErrorIs(t, err, errNilPubsubMessage)

	_, err = s.validateLightClientOptimisticUpdate(ctx, "", &pubsub.Message{Message: &pb.Message{}})
	require.ErrorIs(t, err, errNilPubsubMessage)

	emptyTopic := ""
	_, err = s.validateLightClientOptimisticUpdate(ctx, "", &pubsub.Message{Message: &pb.Message{
		Topic: &emptyTopic,
	}})
	require.ErrorIs(t, err, errNilPubsubMessage)
}

func TestValidateLightClientOptimisticUpdate_valid(t *testing.T) {
	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	s := &Service{cfg: &config{p2p: p, initialSync: &mockSync.Sync{}}}

	_, err := s.validateLightClientOptimisticUpdate(ctx, "", nil)
	require.ErrorIs(t, err, errNilPubsubMessage)
}
