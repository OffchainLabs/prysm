package node

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers/peerdata"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/proto/migration"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

// libp2pAgentVersionKey is the peerstore key used by libp2p to store a peer's
// agent version (advertised via the libp2p identify protocol).
const libp2pAgentVersionKey = "AgentVersion"

// Mapping from Prysm's scorer error categories to the beacon-API spec's
// controlled vocabulary for downscore reasons.
const (
	downscoreReasonBadResponses = "rpc_invalid_response"
	downscoreReasonPeerStatus   = "status_unviable_fork"
	downscoreReasonGossip       = "behaviour_penalty"
)

// GetPeer retrieves data about the given peer.
func (s *Server) GetPeer(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "node.GetPeer")
	defer span.End()

	rawId := r.PathValue("peer_id")
	if rawId == "" {
		httputil.HandleError(w, "peer_id is required in URL params", http.StatusBadRequest)
		return
	}

	peerStatus := s.PeersFetcher.Peers()
	id, err := peer.Decode(rawId)
	if err != nil {
		httputil.HandleError(w, "Invalid peer ID: "+err.Error(), http.StatusBadRequest)
		return
	}
	enr, err := peerStatus.ENR(id)
	if err != nil {
		if errors.Is(err, peerdata.ErrPeerUnknown) {
			httputil.HandleError(w, "Peer not found: "+err.Error(), http.StatusNotFound)
			return
		}
		httputil.HandleError(w, "Could not obtain ENR: "+err.Error(), http.StatusInternalServerError)
		return
	}
	serializedEnr, err := p2p.SerializeENR(enr)
	if err != nil {
		httputil.HandleError(w, "Could not obtain ENR: "+err.Error(), http.StatusInternalServerError)
		return
	}
	p2pAddress, err := peerStatus.Address(id)
	if err != nil {
		httputil.HandleError(w, "Could not obtain address: "+err.Error(), http.StatusInternalServerError)
		return
	}
	state, err := peerStatus.ConnectionState(id)
	if err != nil {
		httputil.HandleError(w, "Could not obtain connection state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	direction, err := peerStatus.Direction(id)
	if err != nil {
		httputil.HandleError(w, "Could not obtain direction: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if eth.PeerDirection(direction) == eth.PeerDirection_UNKNOWN {
		httputil.HandleError(w, "Peer not found", http.StatusNotFound)
		return
	}

	v1ConnState := migration.V1Alpha1ConnectionStateToV1(eth.ConnectionState(state))
	v1PeerDirection, err := migration.V1Alpha1PeerDirectionToV1(eth.PeerDirection(direction))
	if err != nil {
		httputil.HandleError(w, "Could not handle peer direction: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := &structs.Peer{
		PeerId:             rawId,
		Enr:                "enr:" + serializedEnr,
		LastSeenP2PAddress: p2pAddress.String(),
		State:              strings.ToLower(v1ConnState.String()),
		Direction:          strings.ToLower(v1PeerDirection.String()),
	}
	populatePeerScoreFields(data, peerStatus, s.peerHost(), id)

	resp := &structs.GetPeerResponse{Data: data}
	httputil.WriteJson(w, resp)
}

// GetPeers retrieves data about the node's network peers.
func (s *Server) GetPeers(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "node.GetPeers")
	defer span.End()

	states := r.URL.Query()["state"]
	directions := r.URL.Query()["direction"]

	peerStatus := s.PeersFetcher.Peers()
	emptyStateFilter, emptyDirectionFilter := handleEmptyFilters(states, directions)

	if emptyStateFilter && emptyDirectionFilter {
		allIds := peerStatus.All()
		allPeers := make([]*structs.Peer, 0, len(allIds))
		for _, id := range allIds {
			p, err := peerInfo(peerStatus, s.peerHost(), id)
			if err != nil {
				httputil.HandleError(w, "Could not get peer info: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if p == nil {
				continue
			}
			allPeers = append(allPeers, p)
		}
		resp := &structs.GetPeersResponse{
			Data: allPeers,
			Meta: structs.Meta{
				Count: len(allPeers),
			},
		}
		httputil.WriteJson(w, resp)
		return
	}

	var stateIds []peer.ID
	if emptyStateFilter {
		stateIds = peerStatus.All()
	} else {
		for _, stateFilter := range states {
			switch strings.ToUpper(stateFilter) {
			case stateConnecting:
				ids := peerStatus.Connecting()
				stateIds = append(stateIds, ids...)
			case stateConnected:
				ids := peerStatus.Connected()
				stateIds = append(stateIds, ids...)
			case stateDisconnecting:
				ids := peerStatus.Disconnecting()
				stateIds = append(stateIds, ids...)
			case stateDisconnected:
				ids := peerStatus.Disconnected()
				stateIds = append(stateIds, ids...)
			}
		}
	}

	var directionIds []peer.ID
	if emptyDirectionFilter {
		directionIds = peerStatus.All()
	} else {
		for _, directionFilter := range directions {
			switch strings.ToUpper(directionFilter) {
			case directionInbound:
				ids := peerStatus.Inbound()
				directionIds = append(directionIds, ids...)
			case directionOutbound:
				ids := peerStatus.Outbound()
				directionIds = append(directionIds, ids...)
			}
		}
	}

	var filteredIds []peer.ID
	for _, stateId := range stateIds {
		for _, directionId := range directionIds {
			if stateId.String() == directionId.String() {
				filteredIds = append(filteredIds, stateId)
				break
			}
		}
	}
	filteredPeers := make([]*structs.Peer, 0, len(filteredIds))
	for _, id := range filteredIds {
		p, err := peerInfo(peerStatus, s.peerHost(), id)
		if err != nil {
			httputil.HandleError(w, "Could not get peer info: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if p == nil {
			continue
		}
		filteredPeers = append(filteredPeers, p)
	}

	resp := &structs.GetPeersResponse{
		Data: filteredPeers,
		Meta: structs.Meta{
			Count: len(filteredPeers),
		},
	}
	httputil.WriteJson(w, resp)
}

// GetPeerCount retrieves number of known peers.
func (s *Server) GetPeerCount(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "node.PeerCount")
	defer span.End()

	peerStatus := s.PeersFetcher.Peers()

	resp := &structs.GetPeerCountResponse{
		Data: &structs.PeerCount{
			Disconnected:  strconv.FormatInt(int64(len(peerStatus.Disconnected())), 10),
			Connecting:    strconv.FormatInt(int64(len(peerStatus.Connecting())), 10),
			Connected:     strconv.FormatInt(int64(len(peerStatus.Connected())), 10),
			Disconnecting: strconv.FormatInt(int64(len(peerStatus.Disconnecting())), 10),
		},
	}
	httputil.WriteJson(w, resp)
}

func handleEmptyFilters(states []string, directions []string) (emptyState, emptyDirection bool) {
	emptyState = true
	for _, stateFilter := range states {
		normalized := strings.ToUpper(stateFilter)
		filterValid := normalized == stateConnecting || normalized == stateConnected ||
			normalized == stateDisconnecting || normalized == stateDisconnected
		if filterValid {
			emptyState = false
			break
		}
	}

	emptyDirection = true
	for _, directionFilter := range directions {
		normalized := strings.ToUpper(directionFilter)
		filterValid := normalized == directionInbound || normalized == directionOutbound
		if filterValid {
			emptyDirection = false
			break
		}
	}

	return emptyState, emptyDirection
}

func peerInfo(peerStatus *peers.Status, hst host.Host, id peer.ID) (*structs.Peer, error) {
	enr, err := peerStatus.ENR(id)
	if err != nil {
		if errors.Is(err, peerdata.ErrPeerUnknown) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "could not obtain ENR")
	}
	var serializedEnr string
	if enr != nil {
		serializedEnr, err = p2p.SerializeENR(enr)
		if err != nil {
			return nil, errors.Wrap(err, "could not serialize ENR")
		}
	}
	address, err := peerStatus.Address(id)
	if err != nil {
		if errors.Is(err, peerdata.ErrPeerUnknown) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "could not obtain address")
	}
	connectionState, err := peerStatus.ConnectionState(id)
	if err != nil {
		if errors.Is(err, peerdata.ErrPeerUnknown) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "could not obtain connection state")
	}
	direction, err := peerStatus.Direction(id)
	if err != nil {
		if errors.Is(err, peerdata.ErrPeerUnknown) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "could not obtain direction")
	}
	if eth.PeerDirection(direction) == eth.PeerDirection_UNKNOWN {
		return nil, nil
	}
	p := &structs.Peer{
		PeerId:    id.String(),
		State:     strings.ToLower(eth.ConnectionState(connectionState).String()),
		Direction: strings.ToLower(eth.PeerDirection(direction).String()),
	}
	if address != nil {
		p.LastSeenP2PAddress = address.String()
	}
	if serializedEnr != "" {
		p.Enr = "enr:" + serializedEnr
	}
	populatePeerScoreFields(p, peerStatus, hst, id)

	return p, nil
}

// peerHost returns the libp2p host if the server has a PeerManager configured.
// In unit tests the PeerManager is often nil; callers must tolerate a nil host.
func (s *Server) peerHost() host.Host {
	if s.PeerManager == nil {
		return nil
	}
	return s.PeerManager.Host()
}

// populatePeerScoreFields fills in the optional score-related fields on the
// peer struct. All lookups are best-effort: missing data leaves the
// corresponding field empty so it is omitted from the JSON response.
func populatePeerScoreFields(p *structs.Peer, peerStatus *peers.Status, hst host.Host, id peer.ID) {
	if p == nil {
		return
	}
	if hst != nil {
		if agent := agentVersion(hst, id); agent != "" {
			p.AgentVersion = agent
		}
	}
	if peerStatus != nil {
		score := peerStatus.Scorers().Score(id)
		p.Score = &score
		if reasons := downscoreReasons(peerStatus, id); len(reasons) > 0 {
			p.DownscoreReasons = reasons
		}
	}
}

// agentVersion looks up the peer's advertised libp2p agent string from the
// host's peerstore. Returns an empty string when the value is unavailable.
func agentVersion(hst host.Host, id peer.ID) string {
	if hst == nil {
		return ""
	}
	raw, err := hst.Peerstore().Get(id, libp2pAgentVersionKey)
	if err != nil {
		return ""
	}
	agent, ok := raw.(string)
	if !ok {
		return ""
	}
	return agent
}

// downscoreReasons inspects each Prysm scorer to determine why a peer is
// currently considered bad. The Prysm scorers do not retain a per-event
// history, so this reflects the live state at the time of the request and
// maps it to the spec's controlled vocabulary.
func downscoreReasons(peerStatus *peers.Status, id peer.ID) []string {
	if peerStatus == nil {
		return nil
	}
	scorerSvc := peerStatus.Scorers()
	if scorerSvc == nil {
		return nil
	}
	reasons := make([]string, 0, 3)
	if err := scorerSvc.BadResponsesScorer().IsBadPeer(id); err != nil {
		reasons = append(reasons, downscoreReasonBadResponses)
	}
	if err := scorerSvc.PeerStatusScorer().IsBadPeer(id); err != nil {
		reasons = append(reasons, downscoreReasonPeerStatus)
	}
	if err := scorerSvc.GossipScorer().IsBadPeer(id); err != nil {
		reasons = append(reasons, downscoreReasonGossip)
	}
	return reasons
}
