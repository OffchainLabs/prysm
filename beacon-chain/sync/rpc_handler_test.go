package sync

import (
	"context"
	"testing"
	"time"

	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
)

type rpcHandlerTest struct {
	t       *testing.T
	topic   protocol.ID
	timeout time.Duration
	err     error
	s       *Service
}

func (rt *rpcHandlerTest) testHandler(streamHandler network.StreamHandler, rpcHandler rpcHandler, message interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), rt.timeout)
	defer func() {
		cancel()
	}()

	w := util.NewWaiter()
	server := p2ptest.NewTestP2P(rt.t)

	client, ok := rt.s.cfg.p2p.(*p2ptest.TestP2P)
	require.Equal(rt.t, true, ok)

	client.Connect(server)
	defer func() {
		require.NoError(rt.t, client.Disconnect(server.PeerID()))
	}()

	require.Equal(rt.t, 1, len(client.BHost.Network().Peers()), "Expected peers to be connected")
	handler := func(stream network.Stream) {
		defer w.Done()
		streamHandler(stream)
	}

	server.BHost.SetStreamHandler(rt.topic, handler)
	stream, err := client.BHost.NewStream(ctx, server.BHost.ID(), rt.topic)
	require.NoError(rt.t, err)

	err = rpcHandler(ctx, message, stream)
	if rt.err == nil {
		require.NoError(rt.t, err)
	} else {
		require.ErrorIs(rt.t, err, rt.err)
	}

	w.RequireDoneBeforeCancel(ctx, rt.t)
}
