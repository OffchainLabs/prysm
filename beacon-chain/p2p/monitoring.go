package p2p

import (
	"sort"
	"strings"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peerscoring"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Enforcement sites for GreyListRefusalCount.
const (
	greyListSiteDialGater = "dial_gater"
	greyListSiteHandshake = "handshake"
	greyListSiteDiscovery = "discovery"
	greyListSiteConnect   = "connect"
	// GreyListSiteDisconnect marks the sync maintenance loop disconnecting a connected grey-listed peer.
	GreyListSiteDisconnect = "disconnect"
)

var (
	knownAgentVersions = []string{
		"erigon/caplin",
		"grandine",
		"js-libp2p",
		"lighthouse",
		"lodestar",
		"nimbus",
		"prysm",
		"teku",
		"rust-libp2p",
	}
	p2pPeerCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "p2p_peer_count",
		Help: "The number of peers in a given state.",
	},
		[]string{"state"})
	p2pMaxPeers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "p2p_max_peers",
		Help: "The target maximum number of peers.",
	})
	p2pPeerCountDirectionType = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "p2p_peer_count_direction_type",
		Help: "The number of peers in a given direction and type.",
	},
		[]string{"direction", "type"})
	connectedPeersCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "connected_libp2p_peers",
		Help: "Tracks the total number of connected libp2p peers by agent string",
	},
		[]string{"agent"},
	)
	minimumPeersPerSubnet = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "p2p_minimum_peers_per_subnet",
		Help: "The minimum number of peers to connect to per subnet",
	})
	repeatPeerConnections = promauto.NewCounter(prometheus.CounterOpts{
		Name: "p2p_repeat_attempts",
		Help: "The number of repeat attempts the connection handler is triggered for a peer.",
	})
	statusMessageMissing = promauto.NewCounter(prometheus.CounterOpts{
		Name: "p2p_status_message_missing",
		Help: "The number of attempts the connection handler rejects a peer for a missing status message.",
	})
	savedAttestationBroadcasts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "p2p_attestation_subnet_recovered_broadcasts",
		Help: "The number of attestations message broadcast attempts with no peers on " +
			"the subnet. The beacon node increments this counter when the broadcast is blocked " +
			"until a subnet peer can be found.",
	})
	attestationBroadcastAttempts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "p2p_attestation_subnet_attempted_broadcasts",
		Help: "The number of attestations message broadcast attempts.",
	})
	savedSyncCommitteeBroadcasts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "p2p_sync_committee_subnet_recovered_broadcasts",
		Help: "The number of sync committee messages broadcast attempts with no peers on " +
			"the subnet. The beacon node increments this counter when the broadcast is blocked " +
			"until a subnet peer can be found.",
	})
	blobSidecarBroadcasts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "p2p_blob_sidecar_committee_broadcasts",
		Help: "The number of blob sidecar messages that were broadcast with no peer on.",
	})
	syncCommitteeBroadcastAttempts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "p2p_sync_committee_subnet_attempted_broadcasts",
		Help: "The number of sync committee message broadcast attempts.",
	})
	blobSidecarBroadcastAttempts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "p2p_blob_sidecar_committee_attempted_broadcasts",
		Help: "The number of blob sidecar message broadcast attempts.",
	})
	dataColumnSidecarBroadcasts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "p2p_data_column_sidecar_broadcasts",
		Help: "The number of data column sidecar messages that were broadcasted.",
	})
	dataColumnSidecarBroadcastAttempts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "p2p_data_column_sidecar_attempted_broadcasts",
		Help: "The number of data column sidecar message broadcast attempts.",
	})

	// Partial Data Column Metrics
	partialDataColumnBroadcasts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "p2p_partial_data_column_broadcasts",
		Help: "The number of partial data column messages that were broadcasted.",
	})
	// Gossip Tracer Metrics
	pubsubTopicsActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "p2p_pubsub_topic_active",
		Help: "The topics that the peer is participating in gossipsub.",
	},
		[]string{"topic"})
	pubsubTopicsGraft = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_pubsub_graft_total",
		Help: "The number of graft messages sent for a particular topic",
	},
		[]string{"topic"})
	pubsubTopicsPrune = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_pubsub_prune_total",
		Help: "The number of prune messages sent for a particular topic",
	},
		[]string{"topic"})
	pubsubMessageDeliver = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_pubsub_deliver_total",
		Help: "The number of messages received for delivery of a particular topic",
	},
		[]string{"topic"})
	pubsubMessageUndeliverable = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_pubsub_undeliverable_total",
		Help: "The number of messages received which weren't able to be delivered of a particular topic",
	},
		[]string{"topic"})
	pubsubMessageValidate = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_pubsub_validate_total",
		Help: "The number of messages received for validation of a particular topic",
	},
		[]string{"topic"})
	pubsubMessageDuplicate = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_pubsub_duplicate_total",
		Help: "The number of duplicate messages sent for a particular topic",
	},
		[]string{"topic"})
	pubsubMessageReject = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_pubsub_reject_total",
		Help: "The number of messages rejected of a particular topic",
	},
		[]string{"topic", "reason"})
	pubsubPeerThrottle = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_pubsub_throttle_total",
		Help: "The number of times a peer has been throttled for a particular topic",
	},
		[]string{"topic"})
	pubsubRPCRecv = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_pubsub_rpc_recv_total",
		Help: "The number of messages received via rpc for a particular control message",
	},
		[]string{"control_message"})
	pubsubRPCSubRecv = promauto.NewCounter(prometheus.CounterOpts{
		Name: "p2p_pubsub_rpc_recv_sub_total",
		Help: "The number of subscription messages received via rpc",
	})
	pubsubRPCPubRecv = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_pubsub_rpc_recv_pub_total",
		Help: "The number of publish messages received via rpc for a particular topic",
	},
		[]string{"topic"})
	pubsubRPCPubRecvSize = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_pubsub_rpc_recv_pub_bytes_total",
		Help: "The total size in bytes of publish messages received via rpc for a particular topic",
	},
		[]string{"topic", "is_partial"})
	pubsubRPCDrop = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_pubsub_rpc_drop_total",
		Help: "The number of messages dropped via rpc for a particular control message",
	},
		[]string{"control_message"})
	pubsubRPCSubDrop = promauto.NewCounter(prometheus.CounterOpts{
		Name: "p2p_pubsub_rpc_drop_sub_total",
		Help: "The number of subscription messages dropped via rpc",
	})
	pubsubRPCPubDrop = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_pubsub_rpc_drop_pub_total",
		Help: "The number of publish messages dropped via rpc for a particular topic",
	},
		[]string{"topic"})
	pubsubRPCPubDropSize = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_pubsub_rpc_drop_pub_bytes_total",
		Help: "The total size in bytes of publish messages dropped via rpc for a particular topic",
	},
		[]string{"topic", "is_partial"})
	pubsubRPCSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_pubsub_rpc_sent_total",
		Help: "The number of messages sent via rpc for a particular control message",
	},
		[]string{"control_message"})
	pubsubRPCSubSent = promauto.NewCounter(prometheus.CounterOpts{
		Name: "p2p_pubsub_rpc_sent_sub_total",
		Help: "The number of subscription messages sent via rpc",
	})
	pubsubRPCPubSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_pubsub_rpc_sent_pub_total",
		Help: "The number of publish messages sent via rpc for a particular topic",
	},
		[]string{"topic"})
	pubsubRPCPubSentSize = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gossipsub_topic_msg_sent_bytes",
		Help: "The total size of publish messages sent via rpc for a particular topic",
	},
		[]string{"topic", "partial"})
	pubsubMeshPeers = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gossipsub_mesh_peer_counts",
		Help: "The number of capable peers in mesh",
	},
		[]string{"topic", "supports_partial"})

	// Peer scoring / grey-listing metrics.
	greyListedPeersCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "p2p_greylisted_peers",
		Help: "Peers grey-listed per aspect, split by connectedness. A peer grey-listed by " +
			"several aspects is counted under each; trusted-peer exemptions are not applied.",
	},
		[]string{"aspect", "state"})
	// GreyListRefusalCount counts refusals and disconnects of grey-listed peers by
	// enforcement site and the grey-listing aspect that caused them.
	GreyListRefusalCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_greylist_refusals_total",
		Help: "Refusals and disconnects of grey-listed peers, by enforcement site and grey-listing reason.",
	},
		[]string{"site", "reason"})
	inboundPeerTenureSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "p2p_inbound_peer_tenure_seconds",
		Help: "Connection-age percentiles of inbound-connected peers.",
	},
		[]string{"pct"})
	peerScoringTrackedPeers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "p2p_peer_scoring_tracked_peers",
		Help: "Peers the peer scorer currently holds scoring state for.",
	})
	gossipRejectionsRetained = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "p2p_gossip_rejections_retained",
		Help: "Gossip rejections currently retained across all peers.",
	})
	blockProviderTrackedPeers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "p2p_blockprovider_tracked_peers",
		Help: "Peers the block provider selector currently holds stats for.",
	})
)

func (s *Service) updateMetrics() {
	store := s.Host().Peerstore()
	connectedPeers := s.peers.Connected()

	p2pPeerCount.WithLabelValues("Connected").Set(float64(len(connectedPeers)))
	p2pPeerCount.WithLabelValues("Disconnected").Set(float64(len(s.peers.Disconnected())))
	p2pPeerCount.WithLabelValues("Connecting").Set(float64(len(s.peers.Connecting())))
	p2pPeerCount.WithLabelValues("Disconnecting").Set(float64(len(s.peers.Disconnecting())))

	// Grey-list verdicts per aspect; their union is the "Bad" count.
	byAspect := s.peerScorer.GreyListedPeersByAspect()
	uniqueGreyListed := make(map[peer.ID]bool)
	for _, pids := range byAspect {
		for _, pid := range pids {
			uniqueGreyListed[pid] = true
		}
	}
	p2pPeerCount.WithLabelValues("Bad").Set(float64(len(uniqueGreyListed)))

	badIPPeers := make([]peer.ID, 0)
	for _, pid := range s.peers.All() {
		if s.peers.IsFromBadIP(pid) != nil {
			badIPPeers = append(badIPPeers, pid)
		}
	}
	byAspect[peerscoring.AspectBadIP] = badIPPeers

	for _, aspect := range []string{peerscoring.AspectBadResponses, peerscoring.AspectPeerStatus, peerscoring.AspectGossip, peerscoring.AspectBadIP} {
		connected, disconnected := 0, 0
		for _, pid := range byAspect[aspect] {
			if state, err := s.peers.ConnectionState(pid); err == nil && state == peers.Connected {
				connected++
			} else {
				disconnected++
			}
		}
		greyListedPeersCount.WithLabelValues(aspect, "connected").Set(float64(connected))
		greyListedPeersCount.WithLabelValues(aspect, "disconnected").Set(float64(disconnected))
	}

	// Inbound peer tenure distribution: the eviction-protection input.
	inbound := s.peers.InboundConnected()
	tenures := make([]float64, 0, len(inbound))
	for _, pid := range inbound {
		connectedAt, err := s.peers.ConnectedAt(pid)
		if err != nil || connectedAt.IsZero() {
			continue
		}
		tenures = append(tenures, prysmTime.Since(connectedAt).Seconds())
	}
	sort.Float64s(tenures)
	median, p90 := 0.0, 0.0
	if n := len(tenures); n > 0 {
		median = tenures[n/2]
		p90 = tenures[min(n*9/10, n-1)]
	}
	inboundPeerTenureSeconds.WithLabelValues("50").Set(median)
	inboundPeerTenureSeconds.WithLabelValues("90").Set(p90)

	// Scoring-state footprint: flat lines mean pruning and reconciliation keep it bounded.
	peerScoringTrackedPeers.Set(float64(s.peerScorer.TrackedPeerCount()))
	gossipRejectionsRetained.Set(float64(s.gossipRejections.RetainedCount()))
	blockProviderTrackedPeers.Set(float64(s.blockProviderSelector.TrackedPeerCount()))

	upperTCP := strings.ToUpper(string(peers.TCP))
	upperQUIC := strings.ToUpper(string(peers.QUIC))

	p2pPeerCountDirectionType.WithLabelValues("inbound", upperTCP).Set(float64(len(s.peers.InboundConnectedWithProtocol(peers.TCP))))
	p2pPeerCountDirectionType.WithLabelValues("inbound", upperQUIC).Set(float64(len(s.peers.InboundConnectedWithProtocol(peers.QUIC))))
	p2pPeerCountDirectionType.WithLabelValues("outbound", upperTCP).Set(float64(len(s.peers.OutboundConnectedWithProtocol(peers.TCP))))
	p2pPeerCountDirectionType.WithLabelValues("outbound", upperQUIC).Set(float64(len(s.peers.OutboundConnectedWithProtocol(peers.QUIC))))

	connectedPeersCountByClient := make(map[string]float64)
	for _, p := range connectedPeers {
		pid, err := peer.Decode(p.String())
		if err != nil {
			log.WithError(err).Debug("Could not decode peer string")
			continue
		}

		foundName := agentFromPid(pid, store)
		connectedPeersCountByClient[foundName] += 1
	}

	connectedPeersCount.Reset() // Clear out previous results.
	for agent, total := range connectedPeersCountByClient {
		connectedPeersCount.WithLabelValues(agent).Set(total)
	}
}

func agentFromPid(pid peer.ID, store peerstore.Peerstore) string {
	// Get the agent data.
	rawAgent, err := store.Get(pid, "AgentVersion")
	agent, ok := rawAgent.(string)
	if err != nil || !ok {
		return "unknown"
	}
	foundName := "unknown"
	for _, knownAgent := range knownAgentVersions {
		// If the agent string matches one of our known agents, we set
		// the value to our own, sanitized string.
		if strings.Contains(strings.ToLower(agent), knownAgent) {
			foundName = knownAgent
		}
	}
	return foundName
}
