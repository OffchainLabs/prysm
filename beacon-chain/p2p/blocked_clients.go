package p2p

import (
	"strings"

	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

// blockedClientAgents holds the lower-cased substrings of libp2p AgentVersion
// strings whose peers we refuse to connect to. A peer is considered to run one
// of these clients when its AgentVersion contains the substring (case-insensitive).
var blockedClientAgents = []string{
	"lodestar",
}

// isBlockedClientAgent reports whether the given libp2p AgentVersion string
// belongs to a client we refuse to peer with.
func isBlockedClientAgent(agent string) bool {
	lower := strings.ToLower(agent)
	for _, blocked := range blockedClientAgents {
		if strings.Contains(lower, blocked) {
			return true
		}
	}
	return false
}

// isBlockedClientPeer reports whether the given peer was previously identified
// as running a disallowed client.
func (s *Service) isBlockedClientPeer(pid peer.ID) bool {
	s.blockedClientPeersLock.RLock()
	defer s.blockedClientPeersLock.RUnlock()
	return s.blockedClientPeers[pid]
}

// markBlockedClientPeer records that the given peer runs a disallowed client so
// that the connection gater rejects any future (re)connection from it.
func (s *Service) markBlockedClientPeer(pid peer.ID) {
	s.blockedClientPeersLock.Lock()
	defer s.blockedClientPeersLock.Unlock()
	s.blockedClientPeers[pid] = true
}

// watchForBlockedClients subscribes to libp2p peer identification events and
// disconnects from (and blocklists) any peer running a disallowed client. The
// AgentVersion is only known once the identify protocol round completes, which
// is exactly when this event fires, so this is the earliest reliable point at
// which we can filter peers by client.
func (s *Service) watchForBlockedClients() {
	sub, err := s.host.EventBus().Subscribe(new(event.EvtPeerIdentificationCompleted))
	if err != nil {
		log.WithError(err).Error("Could not subscribe to peer identification events; client filtering disabled")
		return
	}
	defer func() {
		err := sub.Close()
		if err != nil {
			log.WithError(err).Error("Could not close peer identification subscription")
		}
	}()

	for {
		select {
		case <-s.ctx.Done():
			return
		case e, ok := <-sub.Out():
			if !ok {
				return
			}
			evt, ok := e.(event.EvtPeerIdentificationCompleted)
			if !ok {
				continue
			}
			if !isBlockedClientAgent(evt.AgentVersion) {
				continue
			}

			s.markBlockedClientPeer(evt.Peer)
			log.WithFields(logrus.Fields{
				"peer":  evt.Peer.String(),
				"agent": evt.AgentVersion,
			}).Debug("Disconnecting from peer running a blocked client")

			if err := s.Disconnect(evt.Peer); err != nil {
				log.WithError(err).WithField("peer", evt.Peer.String()).
					Debug("Could not disconnect from blocked client peer")
			}
		}
	}
}
