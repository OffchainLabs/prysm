package peerscoring

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestGossipRejectionsRecordAndRead(t *testing.T) {
	s := NewGossipRejectionsStore()
	pidB := peer.ID("peer-b")

	// Unknown peers read as zero values and empty pids are ignored.
	require.Equal(t, 0, len(s.Rejections(testPid).ByTopic))
	s.Record("", "topic-a", "agent", errors.New("dropped"))
	require.Equal(t, 0, len(s.All()))

	s.Record(testPid, "topic-a", "prysm/v7", errors.New("bad signature"))
	s.Record(testPid, "topic-b", "prysm/v7", nil)
	s.Record(testPid, "topic-a", "lighthouse/v1", errors.New("bad root"))
	s.Record(pidB, "topic-a", "teku/v2", errors.New("bad state root"))

	got := s.Rejections(testPid)

	require.Equal(t, 2, len(got.ByAgent))
	prysm := got.ByAgent["prysm/v7"]
	require.Equal(t, 2, len(prysm))
	require.Equal(t, "topic-a", prysm[0].Topic)
	require.Equal(t, "bad signature", prysm[0].Reason)
	require.Equal(t, false, prysm[0].At.IsZero())
	require.Equal(t, "topic-b", prysm[1].Topic)
	require.Equal(t, unspecifiedRejectionReason, prysm[1].Reason)

	require.Equal(t, 2, len(got.ByTopic))
	topicA := got.ByTopic["topic-a"]
	require.Equal(t, 2, len(topicA))
	require.Equal(t, "prysm/v7", topicA[0].Agent)
	require.Equal(t, "lighthouse/v1", topicA[1].Agent)
	require.Equal(t, "bad root", topicA[1].Reason)

	all := s.All()
	require.Equal(t, 2, len(all))
	require.Equal(t, 1, len(all[pidB].ByAgent["teku/v2"]))
}

func TestGossipRejectionsPerPeerFIFOCap(t *testing.T) {
	s := NewGossipRejectionsStore(WithMaxRejectionsPerPeer(3))
	for i := 0; i < 5; i++ {
		s.Record(testPid, fmt.Sprintf("topic-%d", i), "agent", fmt.Errorf("reason-%d", i))
	}
	byTopic := s.Rejections(testPid).ByTopic
	require.Equal(t, 3, len(byTopic), "only the newest maxPerPeer rejections must be retained")
	for i := 2; i < 5; i++ {
		rejections := byTopic[fmt.Sprintf("topic-%d", i)]
		require.Equal(t, 1, len(rejections))
		require.Equal(t, fmt.Sprintf("reason-%d", i), rejections[0].Reason)
	}
}

func TestGossipRejectionsRemovePeers(t *testing.T) {
	s := NewGossipRejectionsStore()
	pidB := peer.ID("peer-b")
	s.Record(testPid, "topic-a", "agent", nil)
	s.Record(pidB, "topic-a", "agent", nil)

	s.RemovePeers([]peer.ID{testPid, "peer-unknown"})
	require.Equal(t, 0, len(s.Rejections(testPid).ByTopic))
	require.Equal(t, 1, len(s.Rejections(pidB).ByTopic["topic-a"]))
}

// TestGossipRejectionsConcurrentAccess hammers every store method from concurrent goroutines
// with a tiny per-peer cap so trimming churns. Run with -race.
func TestGossipRejectionsConcurrentAccess(t *testing.T) {
	s := NewGossipRejectionsStore(WithMaxRejectionsPerPeer(3))

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
				switch i % 4 {
				case 0:
					s.Record(pid, "topic-a", "agent", errors.New("concurrent"))
				case 1:
					_ = s.Rejections(pid)
				case 2:
					_ = s.All()
				case 3:
					s.RemovePeers([]peer.ID{pid})
				}
			}
		}(g)
	}
	wg.Wait()
}
