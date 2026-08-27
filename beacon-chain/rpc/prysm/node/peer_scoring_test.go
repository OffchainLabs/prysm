package node

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peerscoring"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	corenet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
)

const scoringTestPeerID = "16Uiu2HAm1n583t4huDMMqEUUBuQs6bLts21mxCfX3tiqu9JfHvRJ"

func newScoringServer(t *testing.T) (*Server, *p2ptest.TestP2P) {
	tp := p2ptest.NewTestP2P(t)
	s := &Server{
		PeersFetcher:            tp,
		PeerManager:             tp,
		PeerScoringFetcher:      tp,
		PeerGreyLister:          tp,
		GossipRejectionsFetcher: tp,
	}
	return s, tp
}

func getScoring(t *testing.T, s *Server, url string, pathPeerID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest("GET", url, nil)
	if pathPeerID != "" {
		request.SetPathValue("peer_id", pathPeerID)
	}
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	switch {
	case pathPeerID != "":
		s.GetPeerScoring(writer, request)
	default:
		s.ListPeersScoring(writer, request)
	}
	return writer
}

func TestGetPeerScoring(t *testing.T) {
	pid, err := peer.Decode(scoringTestPeerID)
	require.NoError(t, err)

	s, tp := newScoringServer(t)
	tp.PeerScoring().RecordBadResponse(pid, peerscoring.SourceRPCStatus, "status timeout")
	tp.GossipRejections().Record(pid, "/eth2/0000/beacon_block/ssz_snappy", "lighthouse/v5.0.0", errors.New("bad signature"))
	require.NoError(t, tp.BHost.Peerstore().Put(pid, "AgentVersion", "lighthouse/v5.0.0"))

	writer := getScoring(t, s, "http://example.com/prysm/v1/node/peers/"+scoringTestPeerID+"/scoring", scoringTestPeerID)
	assert.Equal(t, http.StatusOK, writer.Code)

	resp := &peerscoring.PeerScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.NotNil(t, resp.Data)
	assert.Equal(t, scoringTestPeerID, resp.Data.PeerID)
	assert.Equal(t, "lighthouse/v5.0.0", resp.Data.Agent)
	assert.Equal(t, "DISCONNECTED", resp.Data.ConnectionState)
	assert.Equal(t, "UNKNOWN", resp.Data.Direction)
	assert.Equal(t, false, resp.Data.GreyListed)
	require.IsNil(t, resp.Data.Gossip.TopicScores)
	assert.Equal(t, 1, resp.Data.BadResponses.StandingCount)
	require.Equal(t, 1, len(resp.Data.BadResponses.History))
	assert.Equal(t, "rpc-status", resp.Data.BadResponses.History[0].Source)
	assert.Equal(t, "status timeout", resp.Data.BadResponses.History[0].Reason)
	require.Equal(t, 1, len(resp.Data.Gossip.Rejections))
	assert.Equal(t, "lighthouse/v5.0.0", resp.Data.Gossip.Rejections[0].Agent)
	assert.Equal(t, "bad signature", resp.Data.Gossip.Rejections[0].Reason)
}

func TestGetPeerScoringTopicScores(t *testing.T) {
	pid, err := peer.Decode(scoringTestPeerID)
	require.NoError(t, err)

	s, tp := newScoringServer(t)
	tp.PeerScoring().SetGossipScore(pid, 1, 0, map[string]*pb.TopicScoreSnapshot{
		"/eth2/0000/beacon_attestation_1/ssz_snappy": {TimeInMesh: 99, InvalidMessageDeliveries: 2},
	})

	writer := getScoring(t, s, "http://example.com/x?include_topic_scores=true", scoringTestPeerID)
	assert.Equal(t, http.StatusOK, writer.Code)
	resp := &peerscoring.PeerScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.Equal(t, 1, len(resp.Data.Gossip.TopicScores))
	ts := resp.Data.Gossip.TopicScores["/eth2/0000/beacon_attestation_1/ssz_snappy"]
	require.NotNil(t, ts)
	assert.Equal(t, uint64(99), ts.TimeInMeshMs)
	assert.Equal(t, float64(2), ts.InvalidMessageDeliveries)
}

func TestGetPeerScoringGreyListed(t *testing.T) {
	pid, err := peer.Decode(scoringTestPeerID)
	require.NoError(t, err)

	s, tp := newScoringServer(t)
	for range 5 {
		tp.PeerScoring().RecordBadResponse(pid, peerscoring.SourceRateLimit, "spam")
	}

	writer := getScoring(t, s, "http://example.com/x", scoringTestPeerID)
	assert.Equal(t, http.StatusOK, writer.Code)
	resp := &peerscoring.PeerScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	assert.Equal(t, true, resp.Data.GreyListed)
	require.NotNil(t, resp.Data.GreyListDetails)
	require.StringContains(t, "rate-limit/spam", resp.Data.GreyListDetails.BadResponses)
	assert.Equal(t, "1h0m0s", resp.Data.TimeToWhiteListing)
}

func TestGetPeerScoringInvalidPeerID(t *testing.T) {
	s, _ := newScoringServer(t)

	writer := getScoring(t, s, "http://example.com/x", "not-a-peer")
	assert.Equal(t, http.StatusBadRequest, writer.Code)
	e := &httputil.DefaultJsonError{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
	require.StringContains(t, "Could not decode peer id", e.Message)
}

func TestListPeersScoring(t *testing.T) {
	s, tp := newScoringServer(t)
	good := peer.ID("good")
	bad := peer.ID("bad")
	rejOnly := peer.ID("rejonly")

	tp.PeerScoring().RecordBadResponse(good, peerscoring.SourceSync, "one")
	for range 5 {
		tp.PeerScoring().RecordBadResponse(bad, peerscoring.SourceRateLimit, "spam")
	}
	tp.GossipRejections().Record(rejOnly, "/eth2/0000/beacon_block/ssz_snappy", "grandine/1.0", nil)

	writer := getScoring(t, s, "http://example.com/x", "")
	assert.Equal(t, http.StatusOK, writer.Code)
	resp := &peerscoring.PeersScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.Equal(t, 3, len(resp.Data))
	require.NotNil(t, resp.Meta)
	assert.Equal(t, 3, resp.Meta.Total)
	assert.Equal(t, 3, resp.Meta.Returned)
	// Default sort: grey-listed first, then standing strike count descending.
	assert.Equal(t, bad.String(), resp.Data[0].PeerID)
	assert.Equal(t, true, resp.Data[0].GreyListed)
	assert.Equal(t, good.String(), resp.Data[1].PeerID)
	assert.Equal(t, rejOnly.String(), resp.Data[2].PeerID)
	require.Equal(t, 1, len(resp.Data[2].Gossip.Rejections))

	// greylisted filter.
	writer = getScoring(t, s, "http://example.com/x?greylisted=true", "")
	resp = &peerscoring.PeersScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.Equal(t, 1, len(resp.Data))
	assert.Equal(t, bad.String(), resp.Data[0].PeerID)

	writer = getScoring(t, s, "http://example.com/x?greylisted=false", "")
	resp = &peerscoring.PeersScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.Equal(t, 2, len(resp.Data))

	// source filter.
	writer = getScoring(t, s, "http://example.com/x?source=rate-limit", "")
	resp = &peerscoring.PeersScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.Equal(t, 1, len(resp.Data))
	assert.Equal(t, bad.String(), resp.Data[0].PeerID)

	// pagination.
	writer = getScoring(t, s, "http://example.com/x?limit=1&offset=1", "")
	resp = &peerscoring.PeersScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.Equal(t, 1, len(resp.Data))
	assert.Equal(t, good.String(), resp.Data[0].PeerID)
	assert.Equal(t, 3, resp.Meta.Total)
	assert.Equal(t, 1, resp.Meta.Offset)
	assert.Equal(t, 1, resp.Meta.Returned)

	// sort=peer_id is ordered lexicographically.
	writer = getScoring(t, s, "http://example.com/x?sort=peer_id", "")
	resp = &peerscoring.PeersScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.Equal(t, 3, len(resp.Data))
	for i := 1; i < len(resp.Data); i++ {
		require.Equal(t, true, resp.Data[i-1].PeerID < resp.Data[i].PeerID)
	}

	// Topic scores are omitted by default and included with include_topic_scores=true.
	tp.PeerScoring().SetGossipScore(good, 1, 0, map[string]*pb.TopicScoreSnapshot{
		"/eth2/0000/beacon_attestation_1/ssz_snappy": {TimeInMesh: 42},
	})
	writer = getScoring(t, s, "http://example.com/x?sort=peer_id", "")
	resp = &peerscoring.PeersScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	for _, d := range resp.Data {
		require.IsNil(t, d.Gossip.TopicScores)
	}
	writer = getScoring(t, s, "http://example.com/x?sort=peer_id&include_topic_scores=true", "")
	resp = &peerscoring.PeersScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	withTopics := 0
	for _, d := range resp.Data {
		withTopics += len(d.Gossip.TopicScores)
	}
	require.Equal(t, 1, withTopics)
}

func TestListPeersScoringAgentFilter(t *testing.T) {
	s, tp := newScoringServer(t)
	pid, err := peer.Decode(scoringTestPeerID)
	require.NoError(t, err)
	other := peer.ID("other")

	tp.PeerScoring().RecordBadResponse(pid, peerscoring.SourceSync, "x")
	tp.PeerScoring().RecordBadResponse(other, peerscoring.SourceSync, "y")
	require.NoError(t, tp.BHost.Peerstore().Put(pid, "AgentVersion", "teku/v25.6.0/linux-x86_64"))

	// Case-insensitive substring match.
	writer := getScoring(t, s, "http://example.com/x?agent=TEKU", "")
	resp := &peerscoring.PeersScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.Equal(t, 1, len(resp.Data))
	assert.Equal(t, scoringTestPeerID, resp.Data[0].PeerID)
	assert.Equal(t, "teku/v25.6.0/linux-x86_64", resp.Data[0].Agent)

	// Empty agent param is a no-op: all peers match, including those with no known agent.
	writer = getScoring(t, s, "http://example.com/x?agent=", "")
	resp = &peerscoring.PeersScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.Equal(t, 2, len(resp.Data))
}

func TestListPeersScoringIncludesConnectedPeers(t *testing.T) {
	s, tp := newScoringServer(t)
	pid := peer.ID("connected-only")
	addr, err := ma.NewMultiaddr("/ip4/10.0.0.1/tcp/13000")
	require.NoError(t, err)
	tp.Peers().Add(nil, pid, addr, corenet.DirInbound)
	tp.Peers().SetConnectionState(pid, peers.Connected)

	writer := getScoring(t, s, "http://example.com/x", "")
	resp := &peerscoring.PeersScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.Equal(t, 1, len(resp.Data))
	assert.Equal(t, pid.String(), resp.Data[0].PeerID)
	assert.Equal(t, "CONNECTED", resp.Data[0].ConnectionState)
	assert.Equal(t, "INBOUND", resp.Data[0].Direction)
	assert.NotEqual(t, "", resp.Data[0].ConnectedAt)
	assert.NotEqual(t, "", resp.Data[0].Tenure, "connected peers must report a human-readable tenure")
}

func TestListPeersScoringInvalidParams(t *testing.T) {
	s, _ := newScoringServer(t)
	for _, url := range []string{
		"http://example.com/x?greylisted=banana",
		"http://example.com/x?source=bogus",
		"http://example.com/x?sort=bogus",
		"http://example.com/x?limit=0",
		"http://example.com/x?limit=nope",
		"http://example.com/x?offset=-1",
		"http://example.com/x?include_topic_scores=banana",
	} {
		writer := getScoring(t, s, url, "")
		assert.Equal(t, http.StatusBadRequest, writer.Code, "url: %s", url)
	}
}

func TestListScoringAgents(t *testing.T) {
	s, tp := newScoringServer(t)
	pid, err := peer.Decode(scoringTestPeerID)
	require.NoError(t, err)
	anon1 := peer.ID("anon1")
	anon2 := peer.ID("anon2")

	require.NoError(t, tp.BHost.Peerstore().Put(pid, "AgentVersion", "teku/v25.6.0"))
	tp.PeerScoring().RecordBadResponse(pid, peerscoring.SourceRPCPing, "bad seq")
	tp.PeerScoring().RecordBadResponse(anon1, peerscoring.SourceSync, "x")
	for range 5 {
		tp.PeerScoring().RecordBadResponse(anon2, peerscoring.SourceRateLimit, "spam")
	}
	tp.GossipRejections().Record(pid, "topic", "teku/v25.6.0", nil)

	request := httptest.NewRequest("GET", "http://example.com/x", nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	s.ListScoringAgents(writer, request)
	assert.Equal(t, http.StatusOK, writer.Code)

	resp := &peerscoring.ScoringAgentsResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.Equal(t, 2, len(resp.Data))
	assert.Equal(t, 2, resp.Meta.Total)
	// "unknown" has two peers and sorts first.
	assert.Equal(t, "unknown", resp.Data[0].Agent)
	assert.Equal(t, 2, resp.Data[0].PeerCount)
	assert.Equal(t, 1, resp.Data[0].GreyListedPeerCount)
	assert.Equal(t, 1, resp.Data[0].BadResponsesBySource["sync"])
	assert.Equal(t, 5, resp.Data[0].BadResponsesBySource["rate-limit"])

	assert.Equal(t, "teku/v25.6.0", resp.Data[1].Agent)
	assert.Equal(t, 1, resp.Data[1].PeerCount)
	assert.Equal(t, 0, resp.Data[1].GreyListedPeerCount)
	assert.Equal(t, 1, resp.Data[1].BadResponsesBySource["rpc-ping"])
	assert.Equal(t, 1, resp.Data[1].GossipRejectionsCount)
}

func TestGetPeerScoringConfig(t *testing.T) {
	s, tp := newScoringServer(t)
	tp.PeerScoring().SetHeadSlot(123)

	request := httptest.NewRequest("GET", "http://example.com/x", nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	s.GetPeerScoringConfig(writer, request)
	assert.Equal(t, http.StatusOK, writer.Code)

	resp := &peerscoring.ScoringConfigResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.NotNil(t, resp.Data)
	assert.Equal(t, 5, resp.Data.BadResponseGreyListThreshold)
	assert.Equal(t, -16000, resp.Data.GossipGreyListThreshold)
	assert.Equal(t, "123", resp.Data.OurHeadSlot)
	assert.Equal(t, 100, resp.Data.MaxGossipRejectionsPerPeer)
}

func listRejections(t *testing.T, s *Server, url string) *peerscoring.GossipRejectionsResponse {
	request := httptest.NewRequest("GET", url, nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	s.ListGossipRejections(writer, request)
	require.Equal(t, http.StatusOK, writer.Code)
	resp := &peerscoring.GossipRejectionsResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	return resp
}

func TestListGossipRejections(t *testing.T) {
	s, tp := newScoringServer(t)
	a, err := peer.Decode(p2ptest.MockRawPeerId0)
	require.NoError(t, err)
	b, err := peer.Decode(p2ptest.MockRawPeerId1)
	require.NoError(t, err)

	tp.GossipRejections().Record(a, "/eth2/0000/beacon_block/ssz_snappy", "teku/v25", errors.New("bad sig"))
	time.Sleep(2 * time.Millisecond)
	sinceMark := time.Now().UTC()
	time.Sleep(2 * time.Millisecond)
	tp.GossipRejections().Record(a, "/eth2/0000/beacon_attestation_3/ssz_snappy", "teku/v25", errors.New("wrong committee"))
	time.Sleep(2 * time.Millisecond)
	tp.GossipRejections().Record(b, "/eth2/0000/beacon_block/ssz_snappy", "lodestar/v1", errors.New("bad root"))

	// Unfiltered: all three, newest first.
	resp := listRejections(t, s, "http://example.com/x")
	require.Equal(t, 3, len(resp.Data))
	assert.Equal(t, 3, resp.Meta.Total)
	assert.Equal(t, b.String(), resp.Data[0].PeerID)
	assert.Equal(t, "bad root", resp.Data[0].Reason)
	assert.Equal(t, "wrong committee", resp.Data[1].Reason)
	assert.Equal(t, "bad sig", resp.Data[2].Reason)

	// Topic substring filter.
	resp = listRejections(t, s, "http://example.com/x?topic=beacon_block")
	require.Equal(t, 2, len(resp.Data))

	// Agent substring filter, case-insensitive.
	resp = listRejections(t, s, "http://example.com/x?agent=LODESTAR")
	require.Equal(t, 1, len(resp.Data))
	assert.Equal(t, b.String(), resp.Data[0].PeerID)

	// Peer filter.
	resp = listRejections(t, s, "http://example.com/x?peer_id="+a.String())
	require.Equal(t, 2, len(resp.Data))

	// Since filter keeps only entries at or after the mark.
	resp = listRejections(t, s, "http://example.com/x?since="+sinceMark.Format(time.RFC3339Nano))
	require.Equal(t, 2, len(resp.Data))
	assert.Equal(t, "bad root", resp.Data[0].Reason)
	assert.Equal(t, "wrong committee", resp.Data[1].Reason)

	// Pagination.
	resp = listRejections(t, s, "http://example.com/x?limit=1&offset=1")
	require.Equal(t, 1, len(resp.Data))
	assert.Equal(t, "wrong committee", resp.Data[0].Reason)
	assert.Equal(t, 3, resp.Meta.Total)

	// Invalid params.
	for _, url := range []string{
		"http://example.com/x?since=not-a-time",
		"http://example.com/x?peer_id=not-a-peer",
	} {
		request := httptest.NewRequest("GET", url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.ListGossipRejections(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code, "url: %s", url)
	}
}

func TestGetGossipRejectionsSummary(t *testing.T) {
	s, tp := newScoringServer(t)
	a := peer.ID("peerA")
	b := peer.ID("peerB")

	tp.GossipRejections().Record(a, "topicX", "teku/v25", errors.New("bad sig"))
	tp.GossipRejections().Record(a, "topicX", "teku/v25", errors.New("bad sig"))
	tp.GossipRejections().Record(b, "topicY", "", errors.New("bad root"))

	get := func(url string) (*peerscoring.GossipRejectionsSummaryResponse, int) {
		request := httptest.NewRequest("GET", url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.GetGossipRejectionsSummary(writer, request)
		resp := &peerscoring.GossipRejectionsSummaryResponse{}
		if writer.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		}
		return resp, writer.Code
	}

	// Default group_by=topic, largest group first.
	resp, code := get("http://example.com/x")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "topic", resp.Meta.GroupBy)
	assert.Equal(t, 3, resp.Meta.TotalRejections)
	require.Equal(t, 2, len(resp.Data))
	assert.Equal(t, "topicX", resp.Data[0].Value)
	assert.Equal(t, 2, resp.Data[0].Count)
	assert.Equal(t, "topicY", resp.Data[1].Value)

	// group_by=reason.
	resp, code = get("http://example.com/x?group_by=reason")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 2, len(resp.Data))
	assert.Equal(t, "bad sig", resp.Data[0].Value)
	assert.Equal(t, 2, resp.Data[0].Count)

	// group_by=agent labels empty agents "unknown".
	resp, code = get("http://example.com/x?group_by=agent")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 2, len(resp.Data))
	assert.Equal(t, "teku/v25", resp.Data[0].Value)
	assert.Equal(t, "unknown", resp.Data[1].Value)

	// group_by=peer.
	resp, code = get("http://example.com/x?group_by=peer")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 2, len(resp.Data))
	assert.Equal(t, a.String(), resp.Data[0].Value)

	// Invalid group_by.
	_, code = get("http://example.com/x?group_by=bogus")
	assert.Equal(t, http.StatusBadRequest, code)
}
