package peerscoring

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestMeasureMemoryUsageEmpty(t *testing.T) {
	s := NewScorer()
	u := s.measureMemoryUsage(NewGossipRejectionsStore())

	require.Equal(t, false, u.MeasuredAt.IsZero())
	require.Equal(t, MemoryUsage{MeasuredAt: u.MeasuredAt}, *u)
	require.Equal(t, u, s.memoryUsage.Load())
}

func TestMeasureMemoryUsage(t *testing.T) {
	s := NewScorer()
	a, b := peer.ID("peer-a"), peer.ID("peer-b")
	s.RecordStrike(a, SourceRPCStatus, "first")
	s.RecordStrike(a, SourceRateLimit, "second")
	s.SetAgent(a, "Prysm/v7.1.8")
	s.SetPeerStatus(a, &pb.StatusV2{ForkDigest: make([]byte, 4), FinalizedRoot: make([]byte, 32), HeadRoot: make([]byte, 32)}, nil)
	s.SetPeerStatus(b, nil, errors.New("wrong fork digest"))
	s.SetGossipScore(b, -10, 1, map[string]*pb.TopicScoreSnapshot{"t1": {}, "t2": {}, "t3": nil})

	rej := NewGossipRejectionsStore()
	rej.Record(a, "t1", "agent", errors.New("invalid signature"))
	rej.Record(a, "t2", "agent", nil)
	rej.Record(peer.ID("peer-c"), "t1", "agent", nil)

	u := s.measureMemoryUsage(rej)
	require.Equal(t, 2, u.TrackedPeers)
	require.Equal(t, 2, u.StrikeEntries)
	require.Equal(t, 2, u.StatusEntries)
	require.Equal(t, 3, u.TopicScoreEntries)
	require.Equal(t, 2, u.GossipRejectionPeers)
	require.Equal(t, 3, u.GossipRejections)
	require.NotEqual(t, uint64(0), u.PeerRecordsBytes)
	require.NotEqual(t, uint64(0), u.StrikesBytes)
	require.NotEqual(t, uint64(0), u.StatusesBytes)
	require.NotEqual(t, uint64(0), u.GossipScoresBytes)
	require.NotEqual(t, uint64(0), u.GossipRejectionsBytes)
	require.Equal(t, u.PeerRecordsBytes+u.StrikesBytes+u.StatusesBytes+u.GossipScoresBytes+u.GossipRejectionsBytes, u.TotalBytes)
	require.Equal(t, u, s.memoryUsage.Load())

	// More state is reflected in the matching component only.
	s.RecordStrike(a, SourceGossip, strings.Repeat("r", 1000))
	grown := s.measureMemoryUsage(rej)
	require.Equal(t, true, grown.StrikesBytes >= u.StrikesBytes+1000)
	require.Equal(t, u.PeerRecordsBytes, grown.PeerRecordsBytes)
	require.Equal(t, u.GossipRejectionsBytes, grown.GossipRejectionsBytes)

	// Without a rejections store only the scorer is measured.
	scorerOnly := s.measureMemoryUsage(nil)
	require.Equal(t, 0, scorerOnly.GossipRejections)
	require.Equal(t, uint64(0), scorerOnly.GossipRejectionsBytes)
}

func TestTrackMemoryUsageMeasuresAtStart(t *testing.T) {
	s := NewScorer(WithMemoryUsageInterval(time.Hour))
	s.RecordStrike(testPid, SourceDial, "connectionError")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.TrackMemoryUsage(ctx, NewGossipRejectionsStore())

	// The first measurement does not wait for the interval.
	waitFor(t, func() bool { return s.memoryUsage.Load() != nil })
	require.Equal(t, 1, s.memoryUsage.Load().TrackedPeers)
}

func TestTrackMemoryUsageRefreshes(t *testing.T) {
	s := NewScorer(WithMemoryUsageInterval(5 * time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.TrackMemoryUsage(ctx, NewGossipRejectionsStore())

	waitFor(t, func() bool { return s.memoryUsage.Load() != nil })
	s.RecordStrike(testPid, SourceDial, "connectionError")
	waitFor(t, func() bool { return s.memoryUsage.Load().TrackedPeers == 1 })
}

func TestScoringConfigMemoryUsage(t *testing.T) {
	s := NewScorer()
	rej := NewGossipRejectionsStore()
	s.RecordStrike(testPid, SourceDial, "connectionError")

	c := BuildScoringConfig(s, rej)
	require.IsNil(t, c.MemoryUsage)
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	require.Equal(t, false, strings.Contains(string(raw), "memory_usage"))

	u := s.measureMemoryUsage(rej)
	c = BuildScoringConfig(s, rej)
	require.Equal(t, u, c.MemoryUsage)
	raw, err = json.Marshal(c)
	require.NoError(t, err)
	require.StringContains(t, `"tracked_peers":1`, string(raw))
}

func TestMapTableBytes(t *testing.T) {
	const slot = 24
	tests := []struct {
		entries int
		slots   uint64
	}{
		{0, 0},
		{1, 8},
		{8, 8},
		{9, 16},
		{14, 16},
		{15, 32},
		{1000, 2048},
	}
	for _, tc := range tests {
		require.Equal(t, tc.slots*(slot+1), mapTableBytes(tc.entries, slot))
	}
}
