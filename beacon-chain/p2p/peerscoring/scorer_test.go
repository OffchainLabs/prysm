package peerscoring

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p/core/peer"
)

const testPid = peer.ID("peer-a")

// testParams uses power-of-two weights and thresholds so expected scores stay exact floats.
func testParams() *scoringParams {
	return &scoringParams{
		decayInterval:                time.Hour,
		badResponseGreyListThreshold: 4,
		gossipGreyListThreshold:      -16000,
		gossipPositiveScoreCap:       32,
		badResponseWeight:            0.5,
		peerStatusWeight:             0.25,
		gossipWeight:                 0.25,
	}
}

// testInfo builds the snapshot handed to an individual aspect scorer.
func testInfo(pi *PeerScoringInfo, ourHead, highestHead primitives.Slot) *scoringInfo {
	return &scoringInfo{params: testParams(), peerInfo: pi, ourHeadSlot: ourHead, highestKnownHeadSlot: highestHead}
}

// newTestScorer mirrors testParams through the public options.
func newTestScorer() *Scorer {
	return NewScorer(
		WithBadResponseGreyListThreshold(4),
		WithGossipPositiveScoreCap(32),
		WithBadResponseWeight(0.5),
		WithPeerStatusWeight(0.25),
		WithGossipWeight(0.25),
	)
}

func strikes(n int) []BadResponse {
	return make([]BadResponse, n)
}

func recordStrikes(s *Scorer, n int) {
	for i := 0; i < n; i++ {
		s.RecordBadResponse(testPid, Unknown, "strike")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}

func TestNewScorer(t *testing.T) {
	s := NewScorer()

	want := scoringParams{
		decayInterval:                defaultDecayInterval,
		badResponseGreyListThreshold: defaultBadResponseGreyListThreshold,
		badResponseHistorySize:       defaultBadResponseHistorySize,
		gossipGreyListThreshold:      defaultGossipGreyListThreshold,
		gossipPositiveScoreCap:       defaultGossipPositiveScoreCap,
		badResponseWeight:            defaultBadResponseWeight,
		peerStatusWeight:             defaultPeerStatusWeight,
		gossipWeight:                 defaultGossipWeight,
	}
	require.Equal(t, want, *s.params)
	require.NotNil(t, s.info)

	// All three aspect scorers are wired.
	require.Equal(t, 3, len(s.scorers))
	scorerTypes := make(map[GreyListerAndScorer]bool)
	for _, scorer := range s.scorers {
		scorerTypes[scorer] = true
	}
	require.Equal(t, true, scorerTypes[badResponsesScorer{}])
	require.Equal(t, true, scorerTypes[rpcStatusScorer{}])
	require.Equal(t, true, scorerTypes[gossipScorer{}])
}

func TestNewScorerOptions(t *testing.T) {
	tests := []struct {
		name   string
		opt    Option
		mutate func(p *scoringParams)
	}{
		{"gossip greylist threshold", WithGossipGreyListThreshold(-42), func(p *scoringParams) { p.gossipGreyListThreshold = -42 }},
		{"gossip positive score cap", WithGossipPositiveScoreCap(64), func(p *scoringParams) { p.gossipPositiveScoreCap = 64 }},
		{"bad response greylist threshold", WithBadResponseGreyListThreshold(9), func(p *scoringParams) { p.badResponseGreyListThreshold = 9 }},
		{"bad response history size", WithBadResponseHistorySize(7), func(p *scoringParams) { p.badResponseHistorySize = 7 }},
		{"decay interval", WithDecayInterval(time.Minute), func(p *scoringParams) { p.decayInterval = time.Minute }},
		{"bad response weight", WithBadResponseWeight(0.7), func(p *scoringParams) { p.badResponseWeight = 0.7 }},
		{"peer status weight", WithPeerStatusWeight(0.2), func(p *scoringParams) { p.peerStatusWeight = 0.2 }},
		{"gossip weight", WithGossipWeight(0.1), func(p *scoringParams) { p.gossipWeight = 0.1 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want := *NewScorer().params
			tc.mutate(&want)
			require.Equal(t, want, *NewScorer(tc.opt).params)
		})
	}
}

func TestRecordBadResponse(t *testing.T) {
	s := NewScorer()

	require.Equal(t, 0, s.RecordBadResponse("", SourceDial, "no peer"))
	require.Equal(t, 0, len(s.info))
	require.Equal(t, 0, s.BadResponseCount(testPid))

	require.Equal(t, 1, s.RecordBadResponse(testPid, SourceRPCStatus, "first"))
	require.Equal(t, 2, s.RecordBadResponse(testPid, SourceRateLimit, "second"))
	require.Equal(t, 2, s.BadResponseCount(testPid))

	pi := s.info[testPid]
	require.NotNil(t, pi)
	require.Equal(t, 2, len(pi.badResponses))
	require.Equal(t, SourceRPCStatus, pi.badResponses[0].Source)
	require.Equal(t, "first", pi.badResponses[0].Reason)
	require.Equal(t, false, pi.badResponses[0].at.IsZero())

	pi.badResponseCount-- // decay reduces the standing count
	require.Equal(t, 1, s.BadResponseCount(testPid))
	require.Equal(t, 2, len(pi.badResponses)) // history is unaffected by decay
}

func TestRecordBadResponseTrimsHistory(t *testing.T) {
	s := NewScorer(WithBadResponseHistorySize(3))
	for i := 1; i <= 5; i++ {
		require.Equal(t, i, s.RecordBadResponse(testPid, Unknown, fmt.Sprintf("strike-%d", i)))
	}

	// The standing count keeps growing while the history retains only the newest 3 strikes.
	require.Equal(t, 5, s.BadResponseCount(testPid))
	history := s.info[testPid].badResponses
	require.Equal(t, 3, len(history))
	require.Equal(t, "strike-3", history[0].Reason)
	require.Equal(t, "strike-4", history[1].Reason)
	require.Equal(t, "strike-5", history[2].Reason)
}

func TestRemovePeers(t *testing.T) {
	s := newTestScorer()
	greyPid := peer.ID("grey-peer")

	recordStrikes(s, 2) // testPid stays below the greylist threshold
	s.SetPeerStatus("status-peer", &pb.StatusV2{HeadSlot: 7}, nil)
	s.SetGossipScore("gossip-peer", -5, 0, nil)
	for i := 0; i < 4; i++ {
		s.RecordBadResponse(greyPid, SourceRateLimit, "spam")
	}
	require.ErrorIs(t, s.IsPeerGreyListed(greyPid), ErrPeerGreyListed)

	s.RemovePeers([]peer.ID{testPid, "status-peer", "gossip-peer", greyPid, "unknown-peer"})

	require.Equal(t, 0, s.BadResponseCount(testPid))
	require.Equal(t, float64(0), s.Score(testPid))
	_, err := s.PeerStatus("status-peer")
	require.ErrorIs(t, err, ErrPeerUnknown)
	gScore, _, _ := s.GossipData("gossip-peer")
	require.Equal(t, float64(0), gScore)

	// Grey-listed peers are never forgotten.
	require.ErrorIs(t, s.IsPeerGreyListed(greyPid), ErrPeerGreyListed)
	require.Equal(t, 4, s.BadResponseCount(greyPid))
}

func TestBadResponseSourceString(t *testing.T) {
	sources := map[BadResponseSource]string{
		Unknown: "unknown", SourceDial: "dial", SourceRPCStatus: "rpc-status", SourceRPCPing: "rpc-ping",
		SourceRPCMetadata: "rpc-metadata", SourceRPCRequest: "rpc-request", SourceRPCResponse: "rpc-response",
		SourceRateLimit: "rate-limit", SourceGossip: "gossip", SourceSync: "sync", SourceBackfill: "backfill", SourceDAS: "das",
	}
	for src, want := range sources {
		require.Equal(t, want, src.String())
	}
}

func TestPeerStatusGetters(t *testing.T) {
	s := NewScorer()
	validationErr := errors.New("invalid status")

	// Unknown peer.
	_, err := s.PeerStatus(testPid)
	require.ErrorIs(t, err, ErrPeerUnknown)
	require.Equal(t, true, s.ChainStateLastUpdated(testPid).IsZero())
	require.NoError(t, s.ValidationError(testPid))

	// Known peer without a status exchange.
	s.RecordBadResponse(testPid, SourceDial, "strike only")
	_, err = s.PeerStatus(testPid)
	require.ErrorIs(t, err, ErrNoPeerStatus)
	require.Equal(t, true, s.ChainStateLastUpdated(testPid).IsZero())

	// Status stored with a validation error.
	chainState := &pb.StatusV2{HeadSlot: 42}
	s.SetPeerStatus(testPid, chainState, validationErr)
	got, err := s.PeerStatus(testPid)
	require.NoError(t, err)
	require.Equal(t, chainState, got)
	require.Equal(t, false, s.ChainStateLastUpdated(testPid).IsZero())
	require.Equal(t, validationErr, s.ValidationError(testPid))

	// A later exchange overwrites the verdict.
	s.SetPeerStatus(testPid, chainState, nil)
	require.NoError(t, s.ValidationError(testPid))

	// Status stored with a nil chain state reads as no status.
	s.SetPeerStatus(testPid, nil, validationErr)
	_, err = s.PeerStatus(testPid)
	require.ErrorIs(t, err, ErrNoPeerStatus)
}

func TestHighestHeadSlot(t *testing.T) {
	s := NewScorer()
	require.Equal(t, primitives.Slot(0), s.HighestHeadSlot())

	s.SetPeerStatus("peer-1", &pb.StatusV2{HeadSlot: 10}, nil)
	s.SetPeerStatus("peer-2", &pb.StatusV2{HeadSlot: 30}, errors.New("invalid")) // counted regardless of verdict
	s.SetPeerStatus("peer-3", &pb.StatusV2{HeadSlot: 20}, nil)
	require.Equal(t, primitives.Slot(30), s.HighestHeadSlot())
}

func TestGreyListedPeers(t *testing.T) {
	s := newTestScorer()
	require.Equal(t, 0, len(s.GreyListedPeers()))

	recordStrikes(s, 4)                                             // testPid over the strike threshold
	s.SetGossipScore("gossip-peer", -16000.5, 0, nil)               // below the gossip threshold
	s.SetPeerStatus("status-peer", nil, p2ptypes.ErrInvalidRequest) // terminal status error
	s.SetGossipScore("good-peer", 5, 0, nil)

	greyListed := s.GreyListedPeers()
	require.Equal(t, 3, len(greyListed))
	listed := make(map[peer.ID]bool)
	for _, pid := range greyListed {
		listed[pid] = true
	}
	require.Equal(t, true, listed[testPid])
	require.Equal(t, true, listed["gossip-peer"])
	require.Equal(t, true, listed["status-peer"])
	require.Equal(t, false, listed["good-peer"])
}

func TestSetPeerStatus(t *testing.T) {
	validationErr := errors.New("invalid status")
	tests := []struct {
		name          string
		chainState    *pb.StatusV2
		validationErr error
		startHighest  primitives.Slot
		wantHighest   primitives.Slot
	}{
		{"valid status ratchets highest head", &pb.StatusV2{HeadSlot: 100}, nil, 0, 100},
		{"lower head keeps highest", &pb.StatusV2{HeadSlot: 50}, nil, 100, 100},
		{"validation error does not ratchet", &pb.StatusV2{HeadSlot: 200}, validationErr, 100, 100},
		{"nil chain state does not ratchet", nil, nil, 100, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewScorer()
			s.highestKnownHeadSlot = tc.startHighest

			s.SetPeerStatus(testPid, tc.chainState, tc.validationErr)

			require.Equal(t, tc.wantHighest, s.highestKnownHeadSlot)
			status := s.info[testPid].rpcStatus
			require.NotNil(t, status)
			require.Equal(t, tc.chainState, status.chainState)
			require.Equal(t, tc.validationErr, status.validationError)
		})
	}
}

func TestSetGossipScore(t *testing.T) {
	s := NewScorer()

	gScore, bPenalty, topics := s.GossipData(testPid) // unknown peer reads as zeros
	require.Equal(t, float64(0), gScore)
	require.Equal(t, float64(0), bPenalty)
	require.Equal(t, 0, len(topics))

	snapshots := map[string]*pb.TopicScoreSnapshot{"block": {TimeInMesh: 3}}
	s.SetGossipScore(testPid, -12.5, 1.5, snapshots)
	gScore, bPenalty, topics = s.GossipData(testPid)
	require.Equal(t, -12.5, gScore)
	require.Equal(t, 1.5, bPenalty)
	require.Equal(t, uint64(3), uint64(topics["block"].TimeInMesh))

	s.SetGossipScore(testPid, 3.25, 0, nil) // latest mirror wins
	gScore, bPenalty, topics = s.GossipData(testPid)
	require.Equal(t, 3.25, gScore)
	require.Equal(t, float64(0), bPenalty)
	require.Equal(t, 0, len(topics))
}

func TestReconcileGossipScores(t *testing.T) {
	s := newTestScorer()
	greyPid := peer.ID("grey-peer")
	strikePid := peer.ID("strike-peer")
	statusPid := peer.ID("status-peer")

	snapshots := map[string]*pb.TopicScoreSnapshot{"/topic": {FirstMessageDeliveries: 2}}
	s.ReconcileGossipScores(map[peer.ID]GossipScoreUpdate{
		testPid:   {Score: -12.5, BehaviourPenalty: 1.5, TopicScores: snapshots},
		greyPid:   {Score: -16000.5},
		strikePid: {Score: -3},
		statusPid: {Score: -4},
	})

	gScore, bPenalty, topics := s.GossipData(testPid)
	require.Equal(t, -12.5, gScore)
	require.Equal(t, 1.5, bPenalty)
	require.Equal(t, float32(2), topics["/topic"].FirstMessageDeliveries)
	require.ErrorIs(t, s.IsPeerGreyListed(greyPid), ErrPeerGreyListed)

	s.RecordBadResponse(strikePid, SourceRateLimit, "spam")
	s.SetPeerStatus(statusPid, &pb.StatusV2{HeadSlot: 7}, nil)

	// The next report carries none of the four peers: libp2p purged them.
	s.ReconcileGossipScores(map[peer.ID]GossipScoreUpdate{"other-peer": {Score: 1}})

	// The gossip grey-listing lifts and gossip-only entries are dropped entirely.
	require.NoError(t, s.IsPeerGreyListed(greyPid))
	_, tracked := s.info[greyPid]
	require.Equal(t, false, tracked)
	_, tracked = s.info[testPid]
	require.Equal(t, false, tracked)

	// Strikes and statuses are app-layer state: retained, with only the gossip fields cleared.
	require.Equal(t, 1, s.BadResponseCount(strikePid))
	gScore, bPenalty, topics = s.GossipData(strikePid)
	require.Equal(t, float64(0), gScore)
	require.Equal(t, float64(0), bPenalty)
	require.Equal(t, 0, len(topics))
	_, err := s.PeerStatus(statusPid)
	require.NoError(t, err)
	gScore, _, _ = s.GossipData(statusPid)
	require.Equal(t, float64(0), gScore)

	gScore, _, _ = s.GossipData("other-peer")
	require.Equal(t, float64(1), gScore)
}

func TestSetHeadSlot(t *testing.T) {
	s := NewScorer()
	s.SetHeadSlot(primitives.Slot(42))
	require.Equal(t, primitives.Slot(42), s.ourHeadSlot)
}

func TestScorerScore(t *testing.T) {
	tests := []struct {
		name  string
		setup func(s *Scorer)
		want  float64
	}{
		{"unknown peer", func(*Scorer) {}, 0},
		{"known peer with no signals", func(s *Scorer) { s.SetGossipScore(testPid, 0, 0, nil) }, 0},
		{
			"strikes below threshold",
			func(s *Scorer) { recordStrikes(s, 2) }, // -(2/4) * 0.5
			-0.25,
		},
		{
			"all aspects blended",
			func(s *Scorer) {
				recordStrikes(s, 2)                                            // -(2/4) * 0.5   = -0.25
				s.SetPeerStatus("best-peer", &pb.StatusV2{HeadSlot: 100}, nil) // highest known head
				s.SetPeerStatus(testPid, &pb.StatusV2{HeadSlot: 50}, nil)      // 0.5 * 0.25     = 0.125
				s.SetGossipScore(testPid, 8, 0, nil)                           // (8/32) * 0.25  = 0.0625
			},
			-0.0625,
		},
		{
			"greylisted by strikes scores negative max",
			func(s *Scorer) { recordStrikes(s, 4) },
			-float64(math.MaxInt),
		},
		{
			"greylisted by gossip scores negative max",
			func(s *Scorer) { s.SetGossipScore(testPid, -16000.5, 0, nil) },
			-float64(math.MaxInt),
		},
		{
			"composite is rounded to four decimals",
			func(s *Scorer) {
				s.SetPeerStatus("best-peer", &pb.StatusV2{HeadSlot: 3}, nil)
				s.SetPeerStatus(testPid, &pb.StatusV2{HeadSlot: 1}, nil) // round4(1/3) * 0.25
			},
			0.0833,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestScorer()
			tc.setup(s)
			require.Equal(t, tc.want, s.Score(testPid))
		})
	}
}

func TestScorerScoreBounds(t *testing.T) {
	// Extreme non-grey-listed inputs stay within the weight-normalized [-1, 1] band.
	s := newTestScorer()
	recordStrikes(s, 3)                       // worst sub-threshold strikes: -(3/4) * 0.5 = -0.375
	s.SetGossipScore(testPid, -16000, 0, nil) // worst non-grey-listed gossip: -1 * 0.25  = -0.25
	require.Equal(t, -0.625, s.Score(testPid))

	s = newTestScorer()
	s.SetPeerStatus(testPid, &pb.StatusV2{HeadSlot: 100}, nil) // best status: 1 * 0.25          = 0.25
	s.SetGossipScore(testPid, 1e9, 0, nil)                     // positive gossip clamps: 1*0.25 = 0.25
	require.Equal(t, 0.5, s.Score(testPid))
}

func TestScorerIsPeerGreyListed(t *testing.T) {
	tests := []struct {
		name  string
		setup func(s *Scorer)
		want  bool
	}{
		{"unknown peer", func(*Scorer) {}, false},
		{
			"peer in good standing",
			func(s *Scorer) {
				recordStrikes(s, 3)                                                           // below threshold
				s.SetPeerStatus(testPid, &pb.StatusV2{HeadSlot: 10}, errors.New("temporary")) // non-terminal
				s.SetGossipScore(testPid, -16000, 0, nil)                                     // at threshold, not below
			},
			false,
		},
		{"greylisted by bad responses", func(s *Scorer) { recordStrikes(s, 4) }, true},
		{
			"greylisted by terminal status error",
			func(s *Scorer) { s.SetPeerStatus(testPid, nil, p2ptypes.ErrWrongForkDigestVersion) },
			true,
		},
		{"greylisted by gossip score", func(s *Scorer) { s.SetGossipScore(testPid, -16000.5, 0, nil) }, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestScorer()
			tc.setup(s)
			err := s.IsPeerGreyListed(testPid)
			if tc.want {
				require.ErrorIs(t, err, ErrPeerGreyListed)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGreyListReasons(t *testing.T) {
	t.Run("bad responses reason names the last strike", func(t *testing.T) {
		s := NewScorer(WithBadResponseGreyListThreshold(2))
		s.RecordBadResponse(testPid, SourceRateLimit, "spam")
		s.RecordBadResponse(testPid, SourceGossip, "badBlock")

		err := s.IsPeerGreyListed(testPid)
		require.ErrorIs(t, err, ErrPeerGreyListed)
		require.ErrorContains(t, "2 standing bad responses (threshold 2)", err)
		require.ErrorContains(t, "last: gossip/badBlock", err)
	})
	t.Run("status reason wraps the validation error", func(t *testing.T) {
		s := NewScorer()
		s.SetPeerStatus(testPid, nil, p2ptypes.ErrWrongForkDigestVersion)

		err := s.IsPeerGreyListed(testPid)
		require.ErrorIs(t, err, ErrPeerGreyListed)
		require.ErrorIs(t, err, p2ptypes.ErrWrongForkDigestVersion)
	})
	t.Run("gossip reason names the score", func(t *testing.T) {
		s := NewScorer()
		s.SetGossipScore(testPid, -16000.5, 0, nil)

		err := s.IsPeerGreyListed(testPid)
		require.ErrorIs(t, err, ErrPeerGreyListed)
		require.ErrorContains(t, "gossip score -16000.5 below threshold -16000", err)
	})
}

// TestScorerConcurrentAccess hammers every scorer method from concurrent goroutines with the
// decay loop churning at high frequency. Run with -race.
func TestScorerConcurrentAccess(t *testing.T) {
	s := NewScorer(WithDecayInterval(time.Millisecond), WithBadResponseHistorySize(3))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Start(ctx)

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
				switch i % 10 {
				case 0:
					s.RecordBadResponse(pid, SourceGossip, "concurrent")
				case 1:
					s.SetPeerStatus(pid, &pb.StatusV2{HeadSlot: primitives.Slot(i)}, nil)
				case 2:
					s.SetGossipScore(pid, float64(i), 0, nil)
					s.ReconcileGossipScores(map[peer.ID]GossipScoreUpdate{pid: {Score: float64(i)}})
				case 3:
					_ = s.Score(pid)
				case 4:
					_ = s.IsPeerGreyListed(pid)
				case 5:
					_ = s.GreyListedPeers()
				case 6:
					_ = s.BadResponseCount(pid)
					_, _, _ = s.GossipData(pid)
				case 7:
					_ = s.HighestHeadSlot()
					s.SetHeadSlot(primitives.Slot(i))
				case 8:
					_, _ = s.PeerStatus(pid)
					_ = s.ValidationError(pid)
					_ = s.ChainStateLastUpdated(pid)
				case 9:
					s.RemovePeers([]peer.ID{pid})
				}
			}
		}(g)
	}
	wg.Wait()
}

func TestDecayRestoresGreyListedPeer(t *testing.T) {
	s := NewScorer(WithBadResponseGreyListThreshold(2), WithDecayInterval(5*time.Millisecond))
	recordStrikes(s, 3)
	require.ErrorIs(t, s.IsPeerGreyListed(testPid), ErrPeerGreyListed)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Start(ctx)

	waitFor(t, func() bool { return s.IsPeerGreyListed(testPid) == nil })
}
