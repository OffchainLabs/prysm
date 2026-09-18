package testing

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// TestSimNetGossip verifies that two TestP2P instances on a simulated network can
// deliver a gossipsub message inside a synctest bubble.
func TestSimNetGossip(t *testing.T) {
	SynctestTest(t, func(t *testing.T) {
		p1 := NewTestP2P(t)
		p2 := NewTestP2P(t)
		require.NotNil(t, p1.sim, "expected simulated host inside bubble")
		p1.Connect(p2)

		const topic = "/test/synctest"
		sub, err := p2.SubscribeToTopic(topic)
		require.NoError(t, err)
		_, err = p1.JoinTopic(topic)
		require.NoError(t, err)

		start := time.Now()
		// Note: This sleep is on the fake clock so it should not delay the test.
		time.Sleep(2 * time.Second)

		require.NoError(t, p1.PublishToTopic(context.Background(), topic, []byte("hello")))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		msg, err := sub.Next(ctx)
		require.NoError(t, err)
		require.DeepEqual(t, []byte("hello"), msg.Data)

		require.Equal(t, true, time.Since(start) >= 2*time.Second)

		sub.Cancel()
	})
}

// TestSimNet_Connect verifies the ephemeral-host path works on the simulated network.
func TestSimNet_Connect(t *testing.T) {
	SynctestTest(t, func(t *testing.T) {
		p := NewTestP2P(t)

		h := p.ephemeralHost()
		require.NoError(t, connect(h, p.BHost))
		require.Equal(t, 1, len(p.BHost.Network().Peers()))
	})
}
