package peerscoring

import (
	"context"
	"errors"
	"math"
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
		badResponseGreylistThreshold: 4,
		gossipGreylistThreshold:      -16000,
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
		WithBadResponseGreylistThreshold(4),
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
		badResponseGreylistThreshold: defaultBadResponseGreylistThreshold,
		gossipGreylistThreshold:      defaultGossipGreylistThreshold,
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
		{"gossip greylist threshold", WithGossipGreylistThreshold(-42), func(p *scoringParams) { p.gossipGreylistThreshold = -42 }},
		{"bad response greylist threshold", WithBadResponseGreylistThreshold(9), func(p *scoringParams) { p.badResponseGreylistThreshold = 9 }},
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

	s.RecordBadResponse("", Unknown, "no peer")
	require.Equal(t, 0, len(s.info))

	s.RecordBadResponse(testPid, Unknown, "first")
	s.RecordBadResponse(testPid, Unknown, "second")

	pi := s.info[testPid]
	require.NotNil(t, pi)
	require.Equal(t, 2, len(pi.badResponses))
	require.Equal(t, Unknown, pi.badResponses[0].Source)
	require.Equal(t, "first", pi.badResponses[0].Reason)
	require.Equal(t, false, pi.badResponses[0].at.IsZero())
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

	s.SetGossipScore(testPid, -12.5, 0, nil)
	require.Equal(t, -12.5, s.info[testPid].gossipScore)

	s.SetGossipScore(testPid, 3.25, 0, nil) // latest mirror wins
	require.Equal(t, 3.25, s.info[testPid].gossipScore)
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
			func(s *Scorer) { recordStrikes(s, 2) }, // -(2/4*10) * 0.5
			-2.5,
		},
		{
			"all aspects blended",
			func(s *Scorer) {
				recordStrikes(s, 2)                                            // -5 * 0.5   = -2.5
				s.SetPeerStatus("best-peer", &pb.StatusV2{HeadSlot: 100}, nil) // highest known head
				s.SetPeerStatus(testPid, &pb.StatusV2{HeadSlot: 50}, nil)      // 0.5 * 0.25 = 0.125
				s.SetGossipScore(testPid, 8, 0, nil)                           // 8 * 0.25   = 2
			},
			-0.375,
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

func TestScorerIsPeerGreylisted(t *testing.T) {
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
			require.Equal(t, tc.want, s.IsPeerGreylisted(testPid))
		})
	}
}

func TestDecayRestoresGreylistedPeer(t *testing.T) {
	s := NewScorer(WithBadResponseGreylistThreshold(2), WithDecayInterval(5*time.Millisecond))
	recordStrikes(s, 3)
	require.Equal(t, true, s.IsPeerGreylisted(testPid))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Start(ctx)

	waitFor(t, func() bool { return !s.IsPeerGreylisted(testPid) })
}
