package scorers_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers/peerdata"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers/scorers"
	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestScorers_PeerStatus_Score(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name   string
		update func(scorer *scorers.PeerStatusScorer)
		check  func(scorer *scorers.PeerStatusScorer)
	}{
		{
			name: "nonexistent peer",
			update: func(scorer *scorers.PeerStatusScorer) {
				scorer.SetHeadSlot(64)
			},
			check: func(scorer *scorers.PeerStatusScorer) {
				assert.Equal(t, 0.0, scorer.Score("peer1"), "Unexpected score")
			},
		},
		{
			name: "existent bad peer",
			update: func(scorer *scorers.PeerStatusScorer) {
				scorer.SetHeadSlot(0)
				scorer.SetPeerStatus("peer1", &pb.StatusV2{
					HeadRoot: make([]byte, 32),
					HeadSlot: 64,
				}, p2ptypes.ErrWrongForkDigestVersion)
			},
			check: func(scorer *scorers.PeerStatusScorer) {
				assert.Equal(t, scorers.BadPeerScore, scorer.Score("peer1"), "Unexpected score")
			},
		},
		{
			name: "existent peer no head slot for the host node is known",
			update: func(scorer *scorers.PeerStatusScorer) {
				scorer.SetHeadSlot(0)
				scorer.SetPeerStatus("peer1", &pb.StatusV2{
					HeadRoot: make([]byte, 32),
					HeadSlot: 64,
				}, nil)
			},
			check: func(scorer *scorers.PeerStatusScorer) {
				assert.Equal(t, 1.0, scorer.Score("peer1"), "Unexpected score")
			},
		},
		{
			name: "existent peer head is before ours",
			update: func(scorer *scorers.PeerStatusScorer) {
				scorer.SetHeadSlot(128)
				scorer.SetPeerStatus("peer1", &pb.StatusV2{
					HeadRoot: make([]byte, 32),
					HeadSlot: 64,
				}, nil)
			},
			check: func(scorer *scorers.PeerStatusScorer) {
				assert.Equal(t, 0.0, scorer.Score("peer1"), "Unexpected score")
			},
		},
		{
			name: "existent peer partial score",
			update: func(scorer *scorers.PeerStatusScorer) {
				headSlot := primitives.Slot(128)
				scorer.SetHeadSlot(headSlot)
				scorer.SetPeerStatus("peer1", &pb.StatusV2{
					HeadRoot: make([]byte, 32),
					HeadSlot: headSlot + 64,
				}, nil)
				// Set another peer to a higher score.
				scorer.SetPeerStatus("peer2", &pb.StatusV2{
					HeadRoot: make([]byte, 32),
					HeadSlot: headSlot + 128,
				}, nil)
			},
			check: func(scorer *scorers.PeerStatusScorer) {
				headSlot := uint64(128)
				assert.Equal(t, float64(headSlot+64)/float64(headSlot+128), scorer.Score("peer1"), "Unexpected score")
			},
		},
		{
			name: "existent peer full score",
			update: func(scorer *scorers.PeerStatusScorer) {
				headSlot := primitives.Slot(128)
				scorer.SetHeadSlot(headSlot)
				scorer.SetPeerStatus("peer1", &pb.StatusV2{
					HeadRoot: make([]byte, 32),
					HeadSlot: headSlot + 64,
				}, nil)
			},
			check: func(scorer *scorers.PeerStatusScorer) {
				assert.Equal(t, 1.0, scorer.Score("peer1"), "Unexpected score")
			},
		},
		{
			name: "existent peer no max known slot",
			update: func(scorer *scorers.PeerStatusScorer) {
				scorer.SetHeadSlot(0)
				scorer.SetPeerStatus("peer1", &pb.StatusV2{
					HeadRoot: make([]byte, 32),
					HeadSlot: 0,
				}, nil)
			},
			check: func(scorer *scorers.PeerStatusScorer) {
				assert.Equal(t, 0.0, scorer.Score("peer1"), "Unexpected score")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			peerStatuses := peers.NewStatus(ctx, &peers.StatusConfig{
				ScorerParams: &scorers.Config{},
			})
			scorer := peerStatuses.Scorers().PeerStatusScorer()
			if tt.update != nil {
				tt.update(scorer)
			}
			tt.check(scorer)
		})
	}
}

func TestScorers_PeerStatus_IsBadPeer(t *testing.T) {
	peerStatuses := peers.NewStatus(t.Context(), &peers.StatusConfig{
		ScorerParams: &scorers.Config{},
	})
	pid := peer.ID("peer1")
	assert.NoError(t, peerStatuses.Scorers().IsBadPeer(pid))
	assert.NoError(t, peerStatuses.Scorers().PeerStatusScorer().IsBadPeer(pid))

	peerStatuses.Scorers().PeerStatusScorer().SetPeerStatus(pid, &pb.StatusV2{}, p2ptypes.ErrWrongForkDigestVersion)
	assert.NotNil(t, peerStatuses.Scorers().IsBadPeer(pid))
	assert.NotNil(t, peerStatuses.Scorers().PeerStatusScorer().IsBadPeer(pid))
}

func TestScorers_PeerStatus_BadPeers(t *testing.T) {
	peerStatuses := peers.NewStatus(t.Context(), &peers.StatusConfig{
		ScorerParams: &scorers.Config{},
	})
	pid1 := peer.ID("peer1")
	pid2 := peer.ID("peer2")
	pid3 := peer.ID("peer3")
	assert.NoError(t, peerStatuses.Scorers().IsBadPeer(pid1))
	assert.NoError(t, peerStatuses.Scorers().PeerStatusScorer().IsBadPeer(pid1))
	assert.NoError(t, peerStatuses.Scorers().IsBadPeer(pid2))
	assert.NoError(t, peerStatuses.Scorers().PeerStatusScorer().IsBadPeer(pid2))
	assert.NoError(t, peerStatuses.Scorers().IsBadPeer(pid3))
	assert.NoError(t, peerStatuses.Scorers().PeerStatusScorer().IsBadPeer(pid3))

	peerStatuses.Scorers().PeerStatusScorer().SetPeerStatus(pid1, &pb.StatusV2{}, p2ptypes.ErrWrongForkDigestVersion)
	peerStatuses.Scorers().PeerStatusScorer().SetPeerStatus(pid2, &pb.StatusV2{}, nil)
	peerStatuses.Scorers().PeerStatusScorer().SetPeerStatus(pid3, &pb.StatusV2{}, p2ptypes.ErrWrongForkDigestVersion)
	assert.NotNil(t, peerStatuses.Scorers().IsBadPeer(pid1))
	assert.NotNil(t, peerStatuses.Scorers().PeerStatusScorer().IsBadPeer(pid1))
	assert.NoError(t, peerStatuses.Scorers().IsBadPeer(pid2))
	assert.NoError(t, peerStatuses.Scorers().PeerStatusScorer().IsBadPeer(pid2))
	assert.NotNil(t, peerStatuses.Scorers().IsBadPeer(pid3))
	assert.NotNil(t, peerStatuses.Scorers().PeerStatusScorer().IsBadPeer(pid3))
	assert.Equal(t, 2, len(peerStatuses.Scorers().PeerStatusScorer().BadPeers()))
	assert.Equal(t, 2, len(peerStatuses.Scorers().BadPeers()))
}

func TestScorers_PeerStatus_PeerStatus(t *testing.T) {
	peerStatuses := peers.NewStatus(t.Context(), &peers.StatusConfig{
		ScorerParams: &scorers.Config{},
	})
	status, err := peerStatuses.Scorers().PeerStatusScorer().PeerStatus("peer1")
	require.ErrorContains(t, peerdata.ErrPeerUnknown.Error(), err)
	assert.Equal(t, (*pb.StatusV2)(nil), status)

	peerStatuses.Scorers().PeerStatusScorer().SetPeerStatus("peer1", &pb.StatusV2{
		HeadSlot: 128,
	}, nil)
	peerStatuses.Scorers().PeerStatusScorer().SetPeerStatus("peer2", &pb.StatusV2{
		HeadSlot: 128,
	}, p2ptypes.ErrInvalidEpoch)
	status, err = peerStatuses.Scorers().PeerStatusScorer().PeerStatus("peer1")
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(128), status.HeadSlot)
	assert.Equal(t, nil, peerStatuses.Scorers().ValidationError("peer1"))
	assert.ErrorContains(t, p2ptypes.ErrInvalidEpoch.Error(), peerStatuses.Scorers().ValidationError("peer2"))
	assert.Equal(t, nil, peerStatuses.Scorers().ValidationError("peer3"))
}
