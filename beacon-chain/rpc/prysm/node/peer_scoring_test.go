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
	tp.PeerScoring().RecordStrike(pid, peerscoring.SourceRPCStatus, "status timeout")
	tp.GossipRejections().Record(pid, "/eth2/0000/beacon_block/ssz_snappy", "lighthouse/v5.0.0", errors.New("bad signature"))
	require.NoError(t, tp.BHost.Peerstore().Put(pid, "AgentVersion", "lighthouse/v5.0.0"))

	writer := getScoring(t, s, "http://example.com/prysm/v1/node/peers/"+scoringTestPeerID+"/scoring", scoringTestPeerID)
	assert.Equal(t, http.StatusOK, writer.Code)

	resp := &peerscoring.PeerScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.NotNil(t, resp.Data)
	assert.Equal(t, scoringTestPeerID, resp.Data.PeerID)
	assert.Equal(t, "lighthouse/v5.0.0", resp.Data.Agent)
	assert.Equal(t, peerscoring.AgentTypeLighthouse, resp.Data.AgentType)
	assert.Equal(t, "DISCONNECTED", resp.Data.ConnectionState)
	assert.Equal(t, "UNKNOWN", resp.Data.Direction)
	assert.Equal(t, false, resp.Data.GreyListed)
	require.IsNil(t, resp.Data.Gossip.TopicScores)
	assert.Equal(t, 1, resp.Data.Strikes.StandingCount)
	require.Equal(t, 1, len(resp.Data.Strikes.History))
	assert.Equal(t, "rpc-status", resp.Data.Strikes.History[0].Source)
	assert.Equal(t, "status timeout", resp.Data.Strikes.History[0].Reason)
	require.Equal(t, 1, len(resp.Data.Gossip.Rejections))
	assert.Equal(t, "lighthouse/v5.0.0", resp.Data.Gossip.Rejections[0].Agent)
	assert.Equal(t, peerscoring.AgentTypeLighthouse, resp.Data.Gossip.Rejections[0].AgentType)
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
		tp.PeerScoring().RecordStrike(pid, peerscoring.SourceRateLimit, "spam")
	}

	writer := getScoring(t, s, "http://example.com/x", scoringTestPeerID)
	assert.Equal(t, http.StatusOK, writer.Code)
	resp := &peerscoring.PeerScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	assert.Equal(t, true, resp.Data.GreyListed)
	require.NotNil(t, resp.Data.GreyListDetails)
	require.StringContains(t, "rate-limit/spam", resp.Data.GreyListDetails.Strikes)
	assert.DeepEqual(t, map[string]string{peerscoring.AspectStrikes: "1h0m0s"}, resp.Data.GreyListRecovery)
	assert.Equal(t, false, bytes.Contains(writer.Body.Bytes(), []byte(`"time_to_white_listing"`)))

	t.Run("mixed recovery", func(t *testing.T) {
		tp.PeerScoring().SetGossipScore(pid, -16001, 0, nil)
		writer := getScoring(t, s, "http://example.com/x", scoringTestPeerID)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &peerscoring.PeerScoringDebugResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.DeepEqual(t, map[string]string{
			peerscoring.AspectStrikes: "1h0m0s",
			peerscoring.AspectGossip:  "unknown",
		}, resp.Data.GreyListRecovery)
	})

	t.Run("trusted exemption", func(t *testing.T) {
		tp.Peers().SetTrustedPeers([]peer.ID{pid})
		writer := getScoring(t, s, "http://example.com/x", scoringTestPeerID)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &peerscoring.PeerScoringDebugResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, false, resp.Data.GreyListed)
		assert.Equal(t, peerscoring.GreyListExemptionTrusted, resp.Data.GreyListExemption)
		require.NotNil(t, resp.Data.GreyListDetails)
		assert.Equal(t, false, bytes.Contains(writer.Body.Bytes(), []byte(`"grey_list_recovery"`)))
	})
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

	tp.PeerScoring().RecordStrike(good, peerscoring.SourceSync, "one")
	for range 5 {
		tp.PeerScoring().RecordStrike(bad, peerscoring.SourceRateLimit, "spam")
	}
	tp.GossipRejections().Record(rejOnly, "/eth2/0000/beacon_block/ssz_snappy", "grandine/1.0", nil)

	writer := getScoring(t, s, "http://example.com/x", "")
	assert.Equal(t, http.StatusOK, writer.Code)
	resp := &peerscoring.PeersScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.Equal(t, 3, len(resp.Data))
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

	tp.PeerScoring().RecordStrike(pid, peerscoring.SourceSync, "x")
	tp.PeerScoring().RecordStrike(other, peerscoring.SourceSync, "y")
	require.NoError(t, tp.BHost.Peerstore().Put(pid, "AgentVersion", "teku/v25.6.0/linux-x86_64"))

	// Case-insensitive substring match.
	writer := getScoring(t, s, "http://example.com/x?agent=TEKU", "")
	resp := &peerscoring.PeersScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.Equal(t, 1, len(resp.Data))
	assert.Equal(t, scoringTestPeerID, resp.Data[0].PeerID)
	assert.Equal(t, "teku/v25.6.0/linux-x86_64", resp.Data[0].Agent)
	assert.Equal(t, peerscoring.AgentTypeTeku, resp.Data[0].AgentType)

	// Empty agent param is a no-op: all peers match, including those with no known agent.
	writer = getScoring(t, s, "http://example.com/x?agent=", "")
	resp = &peerscoring.PeersScoringDebugResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	require.Equal(t, 2, len(resp.Data))
}

func TestListPeersScoringAgentTypeFilter(t *testing.T) {
	s, tp := newScoringServer(t)
	lhLinux, err := peer.Decode(p2ptest.MockRawPeerId0)
	require.NoError(t, err)
	lhMac, err := peer.Decode(p2ptest.MockRawPeerId1)
	require.NoError(t, err)
	tekuPeer, err := peer.Decode(scoringTestPeerID)
	require.NoError(t, err)
	anon := peer.ID("anon")
	forgotten := peer.ID("forgotten")

	require.NoError(t, tp.BHost.Peerstore().Put(lhLinux, "AgentVersion", "Lighthouse/v8.2.2-e423a66/aarch64-linux"))
	require.NoError(t, tp.BHost.Peerstore().Put(lhMac, "AgentVersion", "Lighthouse/v8.2.2/aarch64-macos"))
	require.NoError(t, tp.BHost.Peerstore().Put(tekuPeer, "AgentVersion", "teku/v25.6.0"))
	for _, pid := range []peer.ID{lhLinux, lhMac, tekuPeer, anon, forgotten} {
		tp.PeerScoring().RecordStrike(pid, peerscoring.SourceSync, "x")
	}
	// libp2p forgot this peer's agent, but the scorer recorded it.
	tp.PeerScoring().SetAgent(forgotten, "nimbus/v25.9.0")

	list := func(url string) map[string]string {
		writer := getScoring(t, s, url, "")
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &peerscoring.PeersScoringDebugResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		agents := make(map[string]string, len(resp.Data))
		for _, d := range resp.Data {
			agents[d.PeerID] = d.Agent
		}
		return agents
	}

	// Neither filter: every peer.
	require.Equal(t, 5, len(list("http://example.com/x")))

	// agent alone: substring match on the agent, whatever its type.
	require.DeepEqual(t, map[string]string{lhMac.String(): "Lighthouse/v8.2.2/aarch64-macos"}, list("http://example.com/x?agent=macos"))

	// agent_type alone: distinct agents of one type all match, case-insensitively.
	require.DeepEqual(t, map[string]string{
		lhLinux.String(): "Lighthouse/v8.2.2-e423a66/aarch64-linux",
		lhMac.String():   "Lighthouse/v8.2.2/aarch64-macos",
	}, list("http://example.com/x?agent_type=LIGHTHOUSE"))

	// The recorded agent matches both filters after libp2p forgot it.
	require.DeepEqual(t, map[string]string{forgotten.String(): "nimbus/v25.9.0"}, list("http://example.com/x?agent_type=nimbus"))
	require.DeepEqual(t, map[string]string{forgotten.String(): "nimbus/v25.9.0"}, list("http://example.com/x?agent=NIMBUS"))
	require.DeepEqual(t, map[string]string{anon.String(): ""}, list("http://example.com/x?agent_type=unknown"))

	// Both: a peer must match each.
	require.DeepEqual(t, map[string]string{lhMac.String(): "Lighthouse/v8.2.2/aarch64-macos"}, list("http://example.com/x?agent_type=lighthouse&agent=macos"))
	require.Equal(t, 0, len(list("http://example.com/x?agent_type=teku&agent=macos")))
}

func TestListPeersScoringStateFilter(t *testing.T) {
	s, tp := newScoringServer(t)
	addr, err := ma.NewMultiaddr("/ip4/10.0.0.1/tcp/13000")
	require.NoError(t, err)
	// Peers known only to the peer store, without any scoring state.
	connected := peer.ID("connected-only")
	tp.Peers().Add(nil, connected, addr, corenet.DirInbound)
	tp.Peers().SetConnectionState(connected, peers.Connected)
	connecting := peer.ID("connecting-only")
	tp.Peers().Add(nil, connecting, addr, corenet.DirOutbound)
	tp.Peers().SetConnectionState(connecting, peers.Connecting)
	disconnected := peer.ID("disconnected-only")
	tp.Peers().Add(nil, disconnected, addr, corenet.DirOutbound)
	tp.Peers().SetConnectionState(disconnected, peers.Disconnected)
	// A grey-listed peer known only to the scorer.
	greyListed := peer.ID("grey-listed")
	for range 5 {
		tp.PeerScoring().RecordStrike(greyListed, peerscoring.SourceRateLimit, "spam")
	}

	list := func(url string) []*peerscoring.PeerScoringDebug {
		writer := getScoring(t, s, url, "")
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &peerscoring.PeersScoringDebugResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		return resp.Data
	}
	ids := func(ds []*peerscoring.PeerScoringDebug) map[string]bool {
		m := make(map[string]bool, len(ds))
		for _, d := range ds {
			m[d.PeerID] = true
		}
		return m
	}

	// Peers in every state are listed by default.
	all := list("http://example.com/x")
	require.DeepEqual(t, map[string]bool{connected.String(): true, connecting.String(): true, disconnected.String(): true, greyListed.String(): true}, ids(all))

	onlyConnected := list("http://example.com/x?state=connected")
	require.Equal(t, 1, len(onlyConnected))
	assert.Equal(t, connected.String(), onlyConnected[0].PeerID)
	assert.Equal(t, "CONNECTED", onlyConnected[0].ConnectionState)
	assert.Equal(t, "INBOUND", onlyConnected[0].Direction)
	assert.NotEqual(t, "", onlyConnected[0].ConnectedAt)
	assert.NotEqual(t, "", onlyConnected[0].Tenure, "connected peers must report a human-readable tenure")

	// States are case-insensitive, and peers unknown to the peer store are disconnected.
	onlyDisconnected := list("http://example.com/x?state=DISCONNECTED")
	require.DeepEqual(t, map[string]bool{disconnected.String(): true, greyListed.String(): true}, ids(onlyDisconnected))
	for _, d := range onlyDisconnected {
		assert.Equal(t, "", d.Tenure)
	}

	// The state filter is repeatable.
	active := list("http://example.com/x?state=connected&state=connecting")
	require.DeepEqual(t, map[string]bool{connected.String(): true, connecting.String(): true}, ids(active))

	// The state filter composes with the other filters.
	greyDisconnected := list("http://example.com/x?state=disconnected&greylisted=true")
	require.Equal(t, 1, len(greyDisconnected))
	assert.Equal(t, greyListed.String(), greyDisconnected[0].PeerID)
}

func TestListPeersScoringInvalidParams(t *testing.T) {
	s, _ := newScoringServer(t)
	for _, url := range []string{
		"http://example.com/x?greylisted=banana",
		"http://example.com/x?source=bogus",
		"http://example.com/x?sort=bogus",
		"http://example.com/x?state=banana",
		"http://example.com/x?agent_type=hermes",
		"http://example.com/x?include_topic_scores=banana",
	} {
		writer := getScoring(t, s, url, "")
		assert.Equal(t, http.StatusBadRequest, writer.Code, "url: %s", url)
	}
}

func TestListScoringAgents(t *testing.T) {
	s, tp := newScoringServer(t)
	lhLinux, err := peer.Decode(p2ptest.MockRawPeerId0)
	require.NoError(t, err)
	lhMac, err := peer.Decode(p2ptest.MockRawPeerId1)
	require.NoError(t, err)
	tekuPeer, err := peer.Decode(scoringTestPeerID)
	require.NoError(t, err)
	anon1 := peer.ID("anon1")
	anon2 := peer.ID("anon2")
	forgotten := peer.ID("forgotten")

	// Two distinct Lighthouse agents share one agent type.
	require.NoError(t, tp.BHost.Peerstore().Put(lhLinux, "AgentVersion", "Lighthouse/v8.2.2-e423a66/aarch64-linux"))
	require.NoError(t, tp.BHost.Peerstore().Put(lhMac, "AgentVersion", "Lighthouse/v8.2.2/aarch64-macos"))
	require.NoError(t, tp.BHost.Peerstore().Put(tekuPeer, "AgentVersion", "teku/v25.6.0"))
	tp.PeerScoring().RecordStrike(lhLinux, peerscoring.SourceSync, "x")
	tp.PeerScoring().RecordStrike(lhMac, peerscoring.SourceSync, "x")
	tp.PeerScoring().RecordStrike(tekuPeer, peerscoring.SourceRPCPing, "bad seq")
	tp.PeerScoring().RecordStrike(anon1, peerscoring.SourceSync, "x")
	for range 5 {
		tp.PeerScoring().RecordStrike(anon2, peerscoring.SourceRateLimit, "spam")
	}
	tp.GossipRejections().Record(tekuPeer, "topic", "teku/v25.6.0", nil)
	// libp2p forgot this peer's agent, but the scorer recorded it.
	tp.PeerScoring().RecordStrike(forgotten, peerscoring.SourceDial, "x")
	tp.PeerScoring().SetAgent(forgotten, "nimbus/v25.9.0")

	get := func(url string) ([]*peerscoring.AgentScoringDebug, int) {
		request := httptest.NewRequest("GET", url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.ListScoringAgents(writer, request)
		resp := &peerscoring.ScoringAgentsResponse{}
		if writer.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		}
		return resp.Data, writer.Code
	}

	data, code := get("http://example.com/x")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 5, len(data))
	// One row per agent: "unknown" has two peers and sorts first.
	assert.Equal(t, "unknown", data[0].Agent)
	assert.Equal(t, peerscoring.AgentTypeUnknown, data[0].AgentType)
	assert.Equal(t, 2, data[0].PeerCount)
	assert.Equal(t, 1, data[0].GreyListedPeerCount)
	assert.Equal(t, 1, data[0].StrikesBySource["sync"])
	assert.Equal(t, 5, data[0].StrikesBySource["rate-limit"])
	byAgent := make(map[string]*peerscoring.AgentScoringDebug)
	for _, g := range data[1:] {
		assert.Equal(t, 1, g.PeerCount)
		byAgent[g.Agent] = g
	}
	for agent, agentType := range map[string]string{
		"Lighthouse/v8.2.2-e423a66/aarch64-linux": peerscoring.AgentTypeLighthouse,
		"Lighthouse/v8.2.2/aarch64-macos":         peerscoring.AgentTypeLighthouse,
		"nimbus/v25.9.0":                          peerscoring.AgentTypeNimbus, // recorded after libp2p forgot it
		"teku/v25.6.0":                            peerscoring.AgentTypeTeku,
	} {
		require.NotNil(t, byAgent[agent], agent)
		assert.Equal(t, agentType, byAgent[agent].AgentType, agent)
	}
	teku := byAgent["teku/v25.6.0"]
	require.NotNil(t, teku)
	assert.Equal(t, 0, teku.GreyListedPeerCount)
	assert.Equal(t, 1, teku.StrikesBySource["rpc-ping"])
	assert.Equal(t, 1, teku.GossipRejectionsCount)

	// agent_type keeps every agent of that type, case-insensitively.
	data, code = get("http://example.com/x?agent_type=LIGHTHOUSE")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 2, len(data))
	for _, g := range data {
		assert.Equal(t, peerscoring.AgentTypeLighthouse, g.AgentType)
	}
	assert.NotEqual(t, data[0].Agent, data[1].Agent)

	data, code = get("http://example.com/x?agent_type=unknown")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 1, len(data))
	assert.Equal(t, 2, data[0].PeerCount)

	_, code = get("http://example.com/x?agent_type=hermes")
	assert.Equal(t, http.StatusBadRequest, code)
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
	assert.Equal(t, 5, resp.Data.StrikeGreyListThreshold)
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
	assert.Equal(t, b.String(), resp.Data[0].PeerID)
	assert.Equal(t, "bad root", resp.Data[0].Reason)
	assert.Equal(t, peerscoring.AgentTypeLodestar, resp.Data[0].AgentType)
	assert.Equal(t, "wrong committee", resp.Data[1].Reason)
	assert.Equal(t, peerscoring.AgentTypeTeku, resp.Data[1].AgentType)
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
	assert.Equal(t, "", resp.Data[0].AgentType, "topic groups span agent types")
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
	assert.Equal(t, peerscoring.AgentTypeTeku, resp.Data[0].AgentType)
	assert.Equal(t, "unknown", resp.Data[1].Value)
	assert.Equal(t, peerscoring.AgentTypeUnknown, resp.Data[1].AgentType)

	// group_by=agent_type.
	resp, code = get("http://example.com/x?group_by=agent_type")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 2, len(resp.Data))
	assert.Equal(t, peerscoring.AgentTypeTeku, resp.Data[0].Value)
	assert.Equal(t, 2, resp.Data[0].Count)
	assert.Equal(t, peerscoring.AgentTypeUnknown, resp.Data[1].Value)
	assert.Equal(t, 1, resp.Data[1].Count)

	// group_by=peer.
	resp, code = get("http://example.com/x?group_by=peer")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 2, len(resp.Data))
	assert.Equal(t, a.String(), resp.Data[0].Value)

	// Invalid group_by.
	_, code = get("http://example.com/x?group_by=bogus")
	assert.Equal(t, http.StatusBadRequest, code)
}
