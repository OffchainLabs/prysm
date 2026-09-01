package sync

import (
	"context"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	attestationBlockWaitTimeout   = 600 * time.Millisecond
	maxConcurrentBlockWaiters     = 128
	maxConcurrentWaitersPerPeer   = 32
	maxConcurrentWaitersPerRoot   = 64
	attestationBlockEventChanSize = 16
)

// attestationBlockWaiter coordinates bounded waits for attested block and state availability.
type attestationBlockWaiter struct {
	mu             sync.Mutex
	waiters        map[[32]byte]*attestationBlockRootWait
	waitersByPeer  map[peer.ID]int
	nextID         uint64
	count          int
	timeout        time.Duration
	maxWaiters     int
	maxWaitersPeer int
	maxWaitersRoot int
}

type attestationBlockRootWait struct {
	ready chan struct{}
	peers map[uint64]peer.ID
}

func newAttestationBlockWaiter(
	timeout time.Duration,
	maxWaiters int,
	maxWaitersPeer int,
	maxWaitersRoot int,
) *attestationBlockWaiter {
	return &attestationBlockWaiter{
		waiters:        make(map[[32]byte]*attestationBlockRootWait),
		waitersByPeer:  make(map[peer.ID]int),
		timeout:        timeout,
		maxWaiters:     maxWaiters,
		maxWaitersPeer: maxWaitersPeer,
		maxWaitersRoot: maxWaitersRoot,
	}
}

// wait checks readiness, registers within bounded budgets, waits for a block event, and rechecks readiness before returning.
func (w *attestationBlockWaiter) wait(
	ctx context.Context,
	peerID peer.ID,
	root [32]byte,
	available func() bool,
) bool {
	if available() {
		return true
	}

	id, ready, ok := w.register(peerID, root)
	if !ok {
		attestationBlockWaitTotal.WithLabelValues("capacity").Inc()
		return false
	}
	defer w.unregister(peerID, root, id)

	start := time.Now()
	outcome := "canceled"
	defer func() {
		attestationBlockWaitTotal.WithLabelValues(outcome).Inc()
		attestationBlockWaitDuration.Observe(float64(time.Since(start).Milliseconds()))
	}()

	// Recheck after registration to avoid missing availability between the first check and registration.
	if available() {
		outcome = "ready"
		return true
	}

	timer := time.NewTimer(w.timeout)
	defer timer.Stop()

	for {
		select {
		case <-ready:
			ready = nil
			// A block event is only a wake-up hint; readiness still requires both block and state.
			if available() {
				outcome = "ready"
				return true
			}
		case <-timer.C:
			if available() {
				outcome = "ready"
				return true
			}
			outcome = "timeout"
			return false
		case <-ctx.Done():
			return false
		}
	}
}

// register shares one readiness channel per root and enforces global, per-peer, and per-root limits.
func (w *attestationBlockWaiter) register(peerID peer.ID, root [32]byte) (uint64, <-chan struct{}, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	rootWait := w.waiters[root]
	rootCount := 0
	if rootWait != nil {
		rootCount = len(rootWait.peers)
	}
	if w.count >= w.maxWaiters ||
		w.waitersByPeer[peerID] >= w.maxWaitersPeer ||
		rootCount >= w.maxWaitersRoot {
		return 0, nil, false
	}

	id := w.nextID
	w.nextID++
	if rootWait == nil {
		rootWait = &attestationBlockRootWait{
			ready: make(chan struct{}),
			peers: make(map[uint64]peer.ID),
		}
		w.waiters[root] = rootWait
	}
	rootWait.peers[id] = peerID
	w.waitersByPeer[peerID]++
	w.count++
	return id, rootWait.ready, true
}

func (w *attestationBlockWaiter) unregister(peerID peer.ID, root [32]byte, id uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	rootWait, ok := w.waiters[root]
	if !ok {
		return
	}
	if _, ok := rootWait.peers[id]; !ok {
		return
	}
	delete(rootWait.peers, id)
	if len(rootWait.peers) == 0 {
		delete(w.waiters, root)
	}
	w.waitersByPeer[peerID]--
	if w.waitersByPeer[peerID] == 0 {
		delete(w.waitersByPeer, peerID)
	}
	w.count--
}

// notify wakes all validators waiting for the processed block root.
func (w *attestationBlockWaiter) notify(root [32]byte) {
	w.mu.Lock()
	defer w.mu.Unlock()

	rootWait, ok := w.waiters[root]
	if !ok {
		return
	}
	delete(w.waiters, root)
	close(rootWait.ready)
	for _, peerID := range rootWait.peers {
		w.waitersByPeer[peerID]--
		if w.waitersByPeer[peerID] == 0 {
			delete(w.waitersByPeer, peerID)
		}
		w.count--
	}
}

// waitForAttestationBlock returns false on timeout, cancellation, or capacity exhaustion so callers can preserve pending storage and ValidationIgnore.
func (s *Service) waitForAttestationBlock(ctx context.Context, peerID peer.ID, root [32]byte) bool {
	if s.attestationBlockWaiter == nil {
		return s.hasBlockAndState(ctx, root)
	}
	return s.attestationBlockWaiter.wait(ctx, peerID, root, func() bool {
		return s.hasBlockAndState(ctx, root)
	})
}

// startAttestationBlockWaiter forwards processed block roots to waiting gossip validators.
func (s *Service) startAttestationBlockWaiter() {
	if s.attestationBlockWaiter == nil || s.cfg.stateNotifier == nil {
		return
	}

	events := make(chan *feed.Event, attestationBlockEventChanSize)
	sub := s.cfg.stateNotifier.StateFeed().Subscribe(events)
	go func() {
		defer sub.Unsubscribe()
		for {
			select {
			case event := <-events:
				if event == nil || event.Type != statefeed.BlockProcessed {
					continue
				}
				data, ok := event.Data.(*statefeed.BlockProcessedData)
				if !ok || data == nil {
					continue
				}
				s.attestationBlockWaiter.notify(data.BlockRoot)
			case <-sub.Err():
				return
			case <-s.ctx.Done():
				return
			}
		}
	}()
}
