package p2p

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers/peerdata"
	prysmTime "github.com/OffchainLabs/prysm/v6/time"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	agentVersionKey = "AgentVersion"

	// The time to wait for a status request.
	timeForStatus = 10 * time.Second
)

func peerMultiaddrString(conn network.Conn) string {
	remoteMultiaddr := conn.RemoteMultiaddr().String()
	remotePeerID := conn.RemotePeer().String()
	return fmt.Sprintf("%s/p2p/%s", remoteMultiaddr, remotePeerID)
}

func (s *Service) connectToPeer(conn network.Conn) {
	remotePeer := conn.RemotePeer()

	s.peers.SetConnectionState(remotePeer, peers.Connected)
	// Go through the handshake process.
	log.WithFields(logrus.Fields{
		"direction":   conn.Stat().Direction.String(),
		"multiAddr":   peerMultiaddrString(conn),
		"activePeers": len(s.peers.Active()),
		"agent":       agentString(remotePeer, s.Host()),
	}).Debug("Initiate peer connection")
}

func (s *Service) disconnectFromPeerOnError(
	conn network.Conn,
	goodByeFunc func(ctx context.Context, id peer.ID) error,
	badPeerErr error,
) {
	// Get the remote peer ID.
	remotePeerID := conn.RemotePeer()

	// Set the peer to disconnecting state.
	s.peers.SetConnectionState(remotePeerID, peers.Disconnecting)

	// Only attempt a goodbye if we are still connected to the peer.
	if s.host.Network().Connectedness(remotePeerID) == network.Connected {
		if err := goodByeFunc(context.TODO(), remotePeerID); err != nil {
			log.WithError(err).Error("Unable to disconnect from peer")
		}
	}

	log.
		WithError(badPeerErr).
		WithFields(logrus.Fields{
			"multiaddr":            peerMultiaddrString(conn),
			"direction":            conn.Stat().Direction.String(),
			"remainingActivePeers": len(s.peers.Active()),
			"agent":                agentString(remotePeerID, s.Host()),
		}).
		Debug("Initiate peer disconnection")

	s.peers.SetConnectionState(remotePeerID, peers.Disconnected)
}

// AddConnectionHandler adds a callback function which handles the connection with a
// newly added peer. It performs a handshake with that peer by sending a hello request
// and validating the response from the peer.
func (s *Service) AddConnectionHandler(reqFunc, goodByeFunc func(ctx context.Context, id peer.ID) error) {
	// Peer map and lock to keep track of current connection attempts.
	var peerLock sync.Mutex
	peerMap := make(map[peer.ID]bool)

	// This is run at the start of each connection attempt, to ensure
	// that there aren't multiple inflight connection requests for the
	// same peer at once.
	peerHandshaking := func(id peer.ID) bool {
		peerLock.Lock()
		defer peerLock.Unlock()

		if peerMap[id] {
			repeatPeerConnections.Inc()
			return true
		}

		peerMap[id] = true
		return false
	}

	peerFinished := func(id peer.ID) {
		peerLock.Lock()
		defer peerLock.Unlock()

		delete(peerMap, id)
	}

	s.host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(_ network.Network, conn network.Conn) {
			remotePeer := conn.RemotePeer()
			log := log.WithField("peer", remotePeer)
			direction := conn.Stat().Direction

			// For some reason, right after a disconnection, this `ConnectedF` callback
			// is called. We want to avoid processing this connection if the peer was
			// disconnected too recently and if we are at the initiative of this connection.
			// This is very probably a bug in libp2p.
			if direction == network.DirOutbound {
				if err := s.wasDisconnectedTooRecently(remotePeer); err != nil {
					log.WithError(err).Debug("Skipping connection handler")
					return
				}
			}

			// Connection handler must be non-blocking as part of libp2p design.
			go func() {
				if peerHandshaking(remotePeer) {
					// Exit this if there is already another connection
					// request in flight.
					return
				}
				defer peerFinished(remotePeer)

				// Handle the various pre-existing conditions that will result in us not handshaking.
				peerConnectionState, err := s.peers.ConnectionState(remotePeer)
				if err == nil && (peerConnectionState == peers.Connected || peerConnectionState == peers.Connecting) {
					log.WithField("currentState", peerConnectionState).WithField("reason", "already active").Trace("Ignoring connection request")
					return
				}

				s.peers.Add(nil /* ENR */, remotePeer, conn.RemoteMultiaddr(), conn.Stat().Direction)

				// Defensive check in the event we still get a bad peer.
				if err := s.peers.IsBad(remotePeer); err != nil {
					s.disconnectFromPeerOnError(conn, goodByeFunc, err)
					return
				}

				if direction != network.DirInbound {
					s.peers.SetConnectionState(conn.RemotePeer(), peers.Connecting)
					if err := reqFunc(context.TODO(), conn.RemotePeer()); err != nil && !errors.Is(err, io.EOF) {
						s.disconnectFromPeerOnError(conn, goodByeFunc, err)
						return
					}

					s.connectToPeer(conn)
					return
				}

				// The connection is inbound.
				_, err = s.peers.ChainState(remotePeer)
				peerExists := err == nil
				currentTime := prysmTime.Now()

				// Wait for peer to initiate handshake
				time.Sleep(timeForStatus)

				// Exit if we are disconnected with the peer.
				if s.host.Network().Connectedness(remotePeer) != network.Connected {
					return
				}

				// If peer hasn't sent a status request, we disconnect with them
				if _, err := s.peers.ChainState(remotePeer); errors.Is(err, peerdata.ErrPeerUnknown) || errors.Is(err, peerdata.ErrNoPeerStatus) {
					statusMessageMissing.Inc()
					s.disconnectFromPeerOnError(conn, goodByeFunc, errors.Wrap(err, "chain state"))
					return
				}

				if !peerExists {
					s.connectToPeer(conn)
					return
				}

				updated, err := s.peers.ChainStateLastUpdated(remotePeer)
				if err != nil {
					s.disconnectFromPeerOnError(conn, goodByeFunc, errors.Wrap(err, "chain state last updated"))
					return
				}

				// Exit if we don't receive any current status messages from peer.
				if updated.IsZero() {
					s.disconnectFromPeerOnError(conn, goodByeFunc, errors.New("is zero"))
					return
				}

				if updated.Before(currentTime) {
					s.disconnectFromPeerOnError(conn, goodByeFunc, errors.New("did not update"))
					return
				}

				s.connectToPeer(conn)
			}()
		},
	})
}

// AddDisconnectionHandler disconnects from peers. It handles updating the peer status.
// This also calls the handler responsible for maintaining other parts of the sync or p2p system.
func (s *Service) AddDisconnectionHandler(handler func(ctx context.Context, id peer.ID) error) {
	s.host.Network().Notify(&network.NotifyBundle{
		DisconnectedF: func(net network.Network, conn network.Conn) {
			peerID := conn.RemotePeer()

			log := log.WithFields(logrus.Fields{
				"multiAddr": peerMultiaddrString(conn),
				"direction": conn.Stat().Direction.String(),
				"agent":     agentString(peerID, s.Host()),
			})
			// Must be handled in a goroutine as this callback cannot be blocking.
			go func() {
				// Exit early if we are still connected to the peer.
				if net.Connectedness(peerID) == network.Connected {
					return
				}

				priorState, err := s.peers.ConnectionState(peerID)
				if err != nil {
					// Can happen if the peer has already disconnected, so...
					priorState = peers.Disconnected
				}

				s.peers.SetConnectionState(peerID, peers.Disconnecting)
				if err := handler(context.TODO(), conn.RemotePeer()); err != nil {
					log.WithError(err).Error("Disconnect handler failed")
				}

				s.peers.SetConnectionState(peerID, peers.Disconnected)
				if err := s.peerDisconnectionTime.Add(peerID.String(), time.Now(), cache.DefaultExpiration); err != nil {
					// The `DisconnectedF` funcition already called for this peer less than `cache.DefaultExpiration` ago. Skip.
					// (Very probably a bug in libp2p.)
					log.WithError(err).Trace("Failed to set peer disconnection time")
					return
				}

				// Only log disconnections if we were fully connected.
				if priorState == peers.Connected {
					activePeersCount := len(s.peers.Active())
					log.WithField("remainingActivePeers", activePeersCount).Debug("Peer disconnected")
				}
			}()
		},
	})
}

// wasDisconnectedTooRecently checks if the peer was disconnected within the last second.
func (s *Service) wasDisconnectedTooRecently(peerID peer.ID) error {
	const disconnectionDurationThreshold = 1 * time.Second

	peerDisconnectionTimeObj, ok := s.peerDisconnectionTime.Get(peerID.String())
	if !ok {
		return nil
	}

	peerDisconnectionTime, ok := peerDisconnectionTimeObj.(time.Time)
	if !ok {
		return errors.New("invalid peer disconnection time type")
	}

	timeSinceDisconnection := time.Since(peerDisconnectionTime)
	if timeSinceDisconnection < disconnectionDurationThreshold {
		return errors.Errorf("peer %s was disconnected too recently: %s", peerID, timeSinceDisconnection)
	}

	return nil
}

func agentString(pid peer.ID, hst host.Host) string {
	rawVersion, storeErr := hst.Peerstore().Get(pid, agentVersionKey)

	result, ok := rawVersion.(string)
	if storeErr != nil || !ok {
		result = ""
	}

	return result
}
