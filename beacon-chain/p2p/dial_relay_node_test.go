package p2p

import (
	"context"
	"fmt"
	"testing"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestMakePeer_InvalidMultiaddress(t *testing.T) {
	_, err := MakePeer("/ip4")
	assert.ErrorContains(t, "failed to parse multiaddr \"/ip4\"", err, "Expect error when invalid multiaddress was provided")
}

func TestMakePeer_OK(t *testing.T) {
	a, err := MakePeer("/ip4/127.0.0.1/tcp/5678/p2p/QmUn6ycS8Fu6L462uZvuEfDoSgYX6kqP4aSZWMa7z1tWAX")
	require.NoError(t, err, "Unexpected error when making a valid peer")
	assert.Equal(t, "QmUn6ycS8Fu6L462uZvuEfDoSgYX6kqP4aSZWMa7z1tWAX", a.ID.String(), "Unexpected peer ID")
}

func TestDialRelayNode_InvalidPeerString(t *testing.T) {
	err := dialRelayNode(context.Background(), nil, "/ip4")
	assert.ErrorContains(t, "failed to parse multiaddr \"/ip4\"", err, "Expected to fail with invalid peer string")
}

func TestDialRelayNode_OK(t *testing.T) {
	ctx := context.Background()
	relay, err := libp2p.New(libp2p.ResourceManager(&network.NullResourceManager{}))
	require.NoError(t, err)
	host, err := libp2p.New(libp2p.ResourceManager(&network.NullResourceManager{}))
	require.NoError(t, err)
	relayAddr := fmt.Sprintf("%s/p2p/%s", relay.Addrs()[0], relay.ID().String())

	assert.NoError(t, dialRelayNode(ctx, host, relayAddr), "Unexpected error when dialing relay node")
	assert.Equal(t, relay.ID(), host.Peerstore().PeerInfo(relay.ID()).ID, "Host peerstore does not have peer info on relay node")
}
