package sync

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestAttestationBlockWaiterAvailable(t *testing.T) {
	waiter := newAttestationBlockWaiter(time.Second, 1, 1, 1)
	available := atomic.Bool{}
	available.Store(true)

	require.Equal(t, true, waiter.wait(t.Context(), "peer", [32]byte{1}, func() bool {
		return available.Load()
	}))
	require.Equal(t, 0, waiter.count)
}

func TestAttestationBlockWaiterNotify(t *testing.T) {
	waiter := newAttestationBlockWaiter(time.Second, 1, 1, 1)
	root := [32]byte{1}
	available := atomic.Bool{}
	result := make(chan bool, 1)

	go func() {
		result <- waiter.wait(t.Context(), "peer", root, func() bool {
			return available.Load()
		})
	}()

	require.Eventually(t, func() bool {
		waiter.mu.Lock()
		defer waiter.mu.Unlock()
		return waiter.count == 1
	}, time.Second, time.Millisecond)

	available.Store(true)
	waiter.notify(root)
	require.Equal(t, true, <-result)
	require.Equal(t, 0, waiter.count)
}

func TestAttestationBlockWaiterTimeout(t *testing.T) {
	waiter := newAttestationBlockWaiter(time.Millisecond, 1, 1, 1)

	require.Equal(t, false, waiter.wait(t.Context(), "peer", [32]byte{1}, func() bool {
		return false
	}))
	require.Equal(t, 0, waiter.count)
}

func TestAttestationBlockWaiterCancellation(t *testing.T) {
	waiter := newAttestationBlockWaiter(time.Second, 1, 1, 1)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.Equal(t, false, waiter.wait(ctx, "peer", [32]byte{1}, func() bool {
		return false
	}))
	require.Equal(t, 0, waiter.count)
}

func TestAttestationBlockWaiterSharesRootNotification(t *testing.T) {
	waiter := newAttestationBlockWaiter(time.Second, 2, 1, 2)
	root := [32]byte{1}

	_, first, registered := waiter.register("peer-1", root)
	require.Equal(t, true, registered)
	_, second, registered := waiter.register("peer-2", root)
	require.Equal(t, true, registered)
	require.Equal(t, first, second)

	waiter.notify(root)
	select {
	case <-first:
	default:
		t.Fatal("shared root notification was not closed")
	}
	require.Equal(t, 0, waiter.count)
}

func TestServiceStartAttestationBlockWaiter(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	notifier := mock.NewSimpleStateNotifier()
	waiter := newAttestationBlockWaiter(time.Second, 1, 1, 1)
	service := &Service{
		ctx: ctx,
		cfg: &config{
			stateNotifier: notifier,
		},
		attestationBlockWaiter: waiter,
	}
	service.startAttestationBlockWaiter()

	root := [32]byte{1}
	_, ready, registered := waiter.register("peer", root)
	require.Equal(t, true, registered)
	notifier.StateFeed().Send(&feed.Event{
		Type: statefeed.BlockProcessed,
		Data: &statefeed.BlockProcessedData{
			BlockRoot: root,
		},
	})

	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("block processed event did not notify attestation waiter")
	}
	require.Equal(t, 0, waiter.count)
}

func TestAttestationBlockWaiterLimits(t *testing.T) {
	tests := []struct {
		name             string
		maxWaiters       int
		maxWaitersPeer   int
		maxWaitersRoot   int
		firstPeer        peer.ID
		firstRoot        [32]byte
		secondPeer       peer.ID
		secondRoot       [32]byte
		secondRegistered bool
	}{
		{
			name:             "global",
			maxWaiters:       1,
			maxWaitersPeer:   2,
			maxWaitersRoot:   2,
			firstPeer:        "peer-1",
			firstRoot:        [32]byte{1},
			secondPeer:       "peer-2",
			secondRoot:       [32]byte{2},
			secondRegistered: false,
		},
		{
			name:             "per peer",
			maxWaiters:       2,
			maxWaitersPeer:   1,
			maxWaitersRoot:   2,
			firstPeer:        "peer-1",
			firstRoot:        [32]byte{1},
			secondPeer:       "peer-1",
			secondRoot:       [32]byte{2},
			secondRegistered: false,
		},
		{
			name:             "per root",
			maxWaiters:       2,
			maxWaitersPeer:   2,
			maxWaitersRoot:   1,
			firstPeer:        "peer-1",
			firstRoot:        [32]byte{1},
			secondPeer:       "peer-2",
			secondRoot:       [32]byte{1},
			secondRegistered: false,
		},
		{
			name:             "different peers",
			maxWaiters:       2,
			maxWaitersPeer:   1,
			maxWaitersRoot:   1,
			firstPeer:        "peer-1",
			firstRoot:        [32]byte{1},
			secondPeer:       "peer-2",
			secondRoot:       [32]byte{2},
			secondRegistered: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			waiter := newAttestationBlockWaiter(time.Second, test.maxWaiters, test.maxWaitersPeer, test.maxWaitersRoot)
			_, _, registered := waiter.register(test.firstPeer, test.firstRoot)
			require.Equal(t, true, registered)

			_, _, registered = waiter.register(test.secondPeer, test.secondRoot)
			require.Equal(t, test.secondRegistered, registered)
		})
	}
}
