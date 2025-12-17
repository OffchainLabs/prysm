package partialdatacolumnbroadcaster

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

// TestPublishReturnsOnContextDeadline verifies that Publish returns a
// context error when the event loop is not processing requests, rather
// than blocking indefinitely.
func TestPublishReturnsOnContextDeadline(t *testing.T) {
	b := NewBroadcaster(logrus.New())
	b.peerFeedback = func(_ string, _ peer.ID, _ pubsub.PeerFeedbackKind) error { return nil }
	b.publishPartialCol = func(_ string, _ []byte, _ *blocks.PartialDataColumn) error { return nil }

	// Deliberately do NOT start the event loop. This simulates loop() being
	// busy with another request — the effect is the same: respCh never gets a response.

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := b.Publish(ctx, func(yield func(string, blocks.PartialDataColumn) bool) {})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
