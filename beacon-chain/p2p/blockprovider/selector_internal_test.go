package blockprovider

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/crypto/rand"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestRemovePeers(t *testing.T) {
	s := NewSelector(t.Context(), nil)
	s.IncrementProcessedBlocks("peer1", 64)
	s.IncrementProcessedBlocks("peer2", 128)
	require.Equal(t, uint64(64), s.ProcessedBlocks("peer1"))

	s.RemovePeers([]peer.ID{"peer1", "unknown-peer"})

	require.Equal(t, uint64(0), s.ProcessedBlocks("peer1"))
	require.Equal(t, uint64(128), s.ProcessedBlocks("peer2"))
	s.lock.RLock()
	defer s.lock.RUnlock()
	require.Equal(t, 1, len(s.peers))
}

// TestConcurrentAccess hammers every selector method from concurrent goroutines, with the
// decay loop churning at high frequency and the shared CSPRNG generator used exactly as the
// blocks fetcher uses it. Run with -race; it also trips the runtime's concurrent-map checks.
func TestConcurrentAccess(t *testing.T) {
	s := NewSelector(t.Context(), &SelectorConfig{DecayInterval: time.Millisecond})
	r := rand.NewGenerator() // mirrors blocksFetcher's shared, concurrency-safe generator

	pids := make([]peer.ID, 10)
	for i := range pids {
		pids[i] = peer.ID(fmt.Sprintf("peer-%d", i))
	}

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				pid := pids[(g+i)%len(pids)]
				switch i % 8 {
				case 0:
					s.IncrementProcessedBlocks(pid, 64)
				case 1:
					s.Touch(pid)
				case 2:
					_ = s.Score(pid)
				case 3:
					_ = s.ProcessedBlocks(pid)
				case 4:
					_ = s.WeightSorted(r, pids, nil)
				case 5:
					_ = s.Sorted(pids, nil)
				case 6:
					_ = s.FormatScorePretty(pid)
				case 7:
					s.RemovePeers([]peer.ID{pid})
					s.Decay()
				}
			}
		}(g)
	}
	wg.Wait()
}

func TestDecayPrunesIdlePeers(t *testing.T) {
	s := NewSelector(t.Context(), nil)
	s.IncrementProcessedBlocks("active-peer", 2*DefaultDecay) // keeps blocks after one decay step
	s.Touch("fresh-peer")                                     // no blocks, but recently touched
	s.Touch("idle-peer", time.Now().Add(-DefaultStalePeerRefreshInterval))

	s.Decay()

	// The idle entry scores identically to an unknown peer, so it is dropped; the rest stay.
	s.lock.RLock()
	defer s.lock.RUnlock()
	require.Equal(t, 2, len(s.peers))
	require.NotNil(t, s.peers["active-peer"])
	require.NotNil(t, s.peers["fresh-peer"])
	require.Equal(t, DefaultDecay, s.peers["active-peer"].processedBlocks)
}
