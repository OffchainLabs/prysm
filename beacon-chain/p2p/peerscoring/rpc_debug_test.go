package peerscoring

import (
	"errors"
	"testing"
	"time"

	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestBuildPeerDebugUnknownPeer(t *testing.T) {
	s := NewScorer()
	rej := NewGossipRejectionsStore()
	pid := peer.ID("unknown")

	d := BuildPeerDebug(pid, PeerDebugOptions{}, s, rej)
	require.Equal(t, pid.String(), d.PeerID)
	require.Equal(t, "", d.ConnectedAt)
	require.Equal(t, "", d.Tenure)
	require.Equal(t, false, d.GreyListed)
	require.IsNil(t, d.GreyListDetails)
	require.Equal(t, "", d.GreyListExemption)
	require.Equal(t, "", d.TimeToWhiteListing)
	require.Equal(t, 0, d.BadResponses.StandingCount)
	require.Equal(t, defaultBadResponseGreyListThreshold, d.BadResponses.GreyListThreshold)
	require.Equal(t, 0, len(d.BadResponses.History))
	require.IsNil(t, d.RpcStatus)
	require.Equal(t, float64(0), d.Gossip.Score)
	require.Equal(t, 0, len(d.Gossip.Rejections))
}

func TestBuildPeerDebugFullPicture(t *testing.T) {
	s := NewScorer()
	rej := NewGossipRejectionsStore()
	pid := peer.ID("peer1")

	s.RecordBadResponse(pid, SourceRPCStatus, "status timeout")
	s.RecordBadResponse(pid, SourceSync, "bad block")
	s.SetPeerStatus(pid, &pb.StatusV2{
		ForkDigest:            []byte{1, 2, 3, 4},
		FinalizedRoot:         []byte{5, 6},
		FinalizedEpoch:        7,
		HeadRoot:              []byte{8, 9},
		HeadSlot:              256,
		EarliestAvailableSlot: 12,
	}, errors.New("some validation issue"))
	s.SetGossipScore(pid, -42.5, 1.5, nil)
	rej.Record(pid, "/eth2/0000/beacon_block/ssz_snappy", "teku/1.2.3", errors.New("bad signature"))

	connectedAt := time.Date(2026, 8, 26, 10, 30, 0, 0, time.UTC)
	opts := PeerDebugOptions{
		Agent:           "teku/1.2.3",
		ConnectionState: "CONNECTED",
		Direction:       "INBOUND",
		ConnectedAt:     connectedAt,
		Tenure:          90*time.Minute + 30*time.Second + 500*time.Millisecond,
	}
	d := BuildPeerDebug(pid, opts, s, rej)
	require.Equal(t, pid.String(), d.PeerID)
	require.Equal(t, "teku/1.2.3", d.Agent)
	require.Equal(t, "CONNECTED", d.ConnectionState)
	require.Equal(t, "INBOUND", d.Direction)
	require.Equal(t, connectedAt.Format(time.RFC3339Nano), d.ConnectedAt)
	require.Equal(t, "1h30m30s", d.Tenure)
	require.Equal(t, false, d.GreyListed)
	require.Equal(t, 2, d.BadResponses.StandingCount)
	require.Equal(t, 2, len(d.BadResponses.History))
	require.Equal(t, "rpc-status", d.BadResponses.History[0].Source)
	require.Equal(t, "status timeout", d.BadResponses.History[0].Reason)
	require.NotEqual(t, "", d.BadResponses.History[0].Timestamp)
	require.Equal(t, "sync", d.BadResponses.History[1].Source)
	require.NotNil(t, d.RpcStatus)
	require.Equal(t, "some validation issue", d.RpcStatus.ValidationError)
	require.NotEqual(t, "", d.RpcStatus.LastUpdated)
	require.NotNil(t, d.RpcStatus.ChainState)
	require.Equal(t, "0x01020304", d.RpcStatus.ChainState.ForkDigest)
	require.Equal(t, "0x0506", d.RpcStatus.ChainState.FinalizedRoot)
	require.Equal(t, "7", d.RpcStatus.ChainState.FinalizedEpoch)
	require.Equal(t, "0x0809", d.RpcStatus.ChainState.HeadRoot)
	require.Equal(t, "256", d.RpcStatus.ChainState.HeadSlot)
	require.Equal(t, "12", d.RpcStatus.ChainState.EarliestAvailableSlot)
	require.Equal(t, -42.5, d.Gossip.Score)
	require.Equal(t, 1.5, d.Gossip.BehaviourPenalty)
	require.Equal(t, 1, len(d.Gossip.Rejections))
	require.Equal(t, "/eth2/0000/beacon_block/ssz_snappy", d.Gossip.Rejections[0].Topic)
	require.Equal(t, "teku/1.2.3", d.Gossip.Rejections[0].Agent)
	require.Equal(t, "bad signature", d.Gossip.Rejections[0].Reason)
}

func TestBuildPeerDebugGreyListed(t *testing.T) {
	s := NewScorer(WithBadResponseGreyListThreshold(3), WithDecayInterval(30*time.Minute))
	pid := peer.ID("badpeer")
	for range 4 {
		s.RecordBadResponse(pid, SourceRateLimit, "spam")
	}

	require.NotNil(t, s.IsPeerGreyListed(pid))
	d := BuildPeerDebug(pid, PeerDebugOptions{GreyListed: true}, s, nil)
	require.Equal(t, true, d.GreyListed)
	require.NotNil(t, d.GreyListDetails)
	require.StringContains(t, "rate-limit/spam", d.GreyListDetails.BadResponses)
	require.Equal(t, "", d.GreyListDetails.Gossip)
	require.Equal(t, "", d.GreyListDetails.BadIP)
	require.Equal(t, "", d.GreyListExemption)
	// 4 strikes at threshold 3 need 2 decays of 30m each.
	require.Equal(t, "1h0m0s", d.TimeToWhiteListing)
	require.Equal(t, 0, len(d.Gossip.Rejections))
}

func TestBuildPeerDebugBadIPAndTrustedExemption(t *testing.T) {
	s := NewScorer(WithBadResponseGreyListThreshold(2))
	pid := peer.ID("trusted")
	s.RecordBadResponse(pid, SourceRPCRequest, "spam")
	s.RecordBadResponse(pid, SourceRPCRequest, "spam")

	// Trusted peer: the composite verdict is clean although the scorer and the IP tracker fire.
	badIP := errors.New("colocation limit exceeded: got 6 - limit 5")
	d := BuildPeerDebug(pid, PeerDebugOptions{BadIPError: badIP, Trusted: true}, s, nil)
	require.Equal(t, false, d.GreyListed)
	require.NotNil(t, d.GreyListDetails)
	require.StringContains(t, "rpc-request/spam", d.GreyListDetails.BadResponses)
	require.StringContains(t, "colocation limit exceeded", d.GreyListDetails.BadIP)
	require.Equal(t, GreyListExemptionTrusted, d.GreyListExemption)
	require.Equal(t, "1h0m0s", d.TimeToWhiteListing)

	// Same state without trust: the composite verdict fires and no exemption is reported.
	require.NotNil(t, s.IsPeerGreyListed(pid))
	d = BuildPeerDebug(pid, PeerDebugOptions{GreyListed: true, BadIPError: badIP}, s, nil)
	require.Equal(t, true, d.GreyListed)
	require.Equal(t, "", d.GreyListExemption)
	require.StringContains(t, "colocation limit exceeded", d.GreyListDetails.BadIP)
}

func TestBuildPeerDebugTopicScores(t *testing.T) {
	s := NewScorer()
	pid := peer.ID("peer1")
	s.SetGossipScore(pid, 1, 0, map[string]*pb.TopicScoreSnapshot{
		"/eth2/0000/beacon_block/ssz_snappy": {
			TimeInMesh:               1234,
			FirstMessageDeliveries:   1.5,
			MeshMessageDeliveries:    2.5,
			InvalidMessageDeliveries: 3.5,
		},
	})

	d := BuildPeerDebug(pid, PeerDebugOptions{}, s, nil)
	require.IsNil(t, d.Gossip.TopicScores)

	d = BuildPeerDebug(pid, PeerDebugOptions{IncludeTopicScores: true}, s, nil)
	require.Equal(t, 1, len(d.Gossip.TopicScores))
	ts := d.Gossip.TopicScores["/eth2/0000/beacon_block/ssz_snappy"]
	require.NotNil(t, ts)
	require.Equal(t, uint64(1234), ts.TimeInMeshMs)
	require.Equal(t, 1.5, ts.FirstMessageDeliveries)
	require.Equal(t, 2.5, ts.MeshMessageDeliveries)
	require.Equal(t, 3.5, ts.InvalidMessageDeliveries)
}

func TestTimeToWhiteListing(t *testing.T) {
	s := NewScorer(WithBadResponseGreyListThreshold(2), WithDecayInterval(time.Hour))

	clean := peer.ID("clean")
	s.RecordBadResponse(clean, SourceSync, "one")
	require.Equal(t, time.Duration(0), s.TimeToWhiteListing(clean))

	striker := peer.ID("striker")
	for range 3 {
		s.RecordBadResponse(striker, SourceSync, "x")
	}
	require.Equal(t, 2*time.Hour, s.TimeToWhiteListing(striker))

	// Gossip grey-listing is not time based.
	gossiper := peer.ID("gossiper")
	s.SetGossipScore(gossiper, float64(defaultGossipGreyListThreshold)-1, 0, nil)
	require.NotNil(t, s.IsPeerGreyListed(gossiper))
	require.Equal(t, time.Duration(0), s.TimeToWhiteListing(gossiper))

	// Status grey-listing expires with its TTL.
	wrongFork := peer.ID("wrong-fork")
	s.SetPeerStatus(wrongFork, &pb.StatusV2{HeadSlot: 1}, p2ptypes.ErrWrongForkDigestVersion)
	require.NotNil(t, s.IsPeerGreyListed(wrongFork))
	ttw := s.TimeToWhiteListing(wrongFork)
	require.Equal(t, true, ttw > 0 && ttw <= defaultStatusGreyListTTL)

	require.Equal(t, time.Duration(0), s.TimeToWhiteListing(peer.ID("unknown")))
}

func TestTrackedPeers(t *testing.T) {
	s := NewScorer()
	rej := NewGossipRejectionsStore()
	require.Equal(t, 0, len(s.TrackedPeers()))
	require.Equal(t, 0, len(rej.TrackedPeers()))

	s.RecordBadResponse(peer.ID("a"), SourceSync, "x")
	s.SetGossipScore(peer.ID("b"), 1, 0, nil)
	rej.Record(peer.ID("c"), "topic", "agent", nil)

	require.Equal(t, 2, len(s.TrackedPeers()))
	require.Equal(t, 1, len(rej.TrackedPeers()))
	require.Equal(t, peer.ID("c"), rej.TrackedPeers()[0])
}

func TestFlatRejections(t *testing.T) {
	rej := NewGossipRejectionsStore()
	require.Equal(t, 0, len(rej.FlatRejections()))

	rej.Record(peer.ID("a"), "topicA", "agentX", errors.New("bad sig"))
	rej.Record(peer.ID("a"), "topicB", "agentX", nil)
	rej.Record(peer.ID("b"), "topicA", "agentY", errors.New("bad root"))

	flat := rej.FlatRejections()
	require.Equal(t, 3, len(flat))
	perPeer := map[peer.ID]int{}
	for _, rj := range flat {
		perPeer[rj.PeerID]++
		require.NotEqual(t, "", rj.Topic)
		require.NotEqual(t, "", rj.Reason)
	}
	require.Equal(t, 2, perPeer[peer.ID("a")])
	require.Equal(t, 1, perPeer[peer.ID("b")])
}

func TestBuildScoringConfig(t *testing.T) {
	s := NewScorer(WithBadResponseGreyListThreshold(7), WithDecayInterval(30*time.Minute))
	rej := NewGossipRejectionsStore(WithMaxRejectionsPerPeer(42))
	s.SetHeadSlot(100)
	s.SetPeerStatus(peer.ID("a"), &pb.StatusV2{HeadSlot: 200}, nil)
	rej.Record(peer.ID("b"), "topic", "agent", nil)

	c := BuildScoringConfig(s, rej)
	require.Equal(t, 7, c.BadResponseGreyListThreshold)
	require.Equal(t, defaultBadResponseHistorySize, c.BadResponseHistorySize)
	require.Equal(t, "30m0s", c.DecayInterval)
	require.Equal(t, defaultGossipGreyListThreshold, c.GossipGreyListThreshold)
	require.Equal(t, "24h0m0s", c.StatusGreyListTTL)
	require.Equal(t, 42, c.MaxGossipRejectionsPerPeer)
	require.Equal(t, "100", c.OurHeadSlot)
	require.Equal(t, "200", c.HighestKnownHeadSlot)
	require.Equal(t, 1, c.TrackedPeerCount)
	require.Equal(t, 1, c.PeersWithGossipRejections)
}

func TestBadResponseSourceNames(t *testing.T) {
	names := BadResponseSourceNames()
	require.Equal(t, int(SourceDAS)+1, len(names))
	require.Equal(t, "unknown", names[0])
	require.Equal(t, "das", names[len(names)-1])
}
