package p2p

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/peers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/peers/peerdata"
	prysmTime "github.com/prysmaticlabs/prysm/v5/time"
	"github.com/sirupsen/logrus"
)

const (
	// The time to wait for a status request.
	timeForStatus = 10 * time.Second
)

func peerMultiaddrString(conn network.Conn) string {
	remoteMultiaddr := conn.RemoteMultiaddr().String()
	remotePeerID := conn.RemotePeer().String()
	return fmt.Sprintf("%s/p2p/%s", remoteMultiaddr, remotePeerID)
}

func (s *Service) connectToPeer(conn network.Conn) {
	s.peers.SetConnectionState(conn.RemotePeer(), peers.PeerConnected)
	// Go through the handshake process.
	log.WithFields(logrus.Fields{
		"direction":   conn.Stat().Direction.String(),
		"multiAddr":   peerMultiaddrString(conn),
		"activePeers": len(s.peers.Active()),
	}).Debug("Initiate peer connection")
}

func (s *Service) disconnectFromPeer(
	conn network.Conn,
	goodByeFunc func(ctx context.Context, id peer.ID) error,
	badPeerErr error,
) {
	// Get the remote peer ID.
	remotePeerID := conn.RemotePeer()

	// Get the direction of the connection.
	direction := conn.Stat().Direction.String()

	// Get the remote peer multiaddr.
	remotePeerMultiAddr := peerMultiaddrString(conn)

	// Set the peer to disconnecting state.
	s.peers.SetConnectionState(remotePeerID, peers.PeerDisconnecting)

	// Only attempt a goodbye if we are still connected to the peer.
	if s.host.Network().Connectedness(remotePeerID) == network.Connected {
		if err := goodByeFunc(context.TODO(), remotePeerID); err != nil {
			log.WithError(err).Error("Unable to disconnect from peer")
		}
	}

	// Get the remaining active peers.
	activePeerCount := len(s.peers.Active())
	log.
		WithError(badPeerErr).
		WithFields(logrus.Fields{
			"multiaddr":            remotePeerMultiAddr,
			"direction":            direction,
			"remainingActivePeers": activePeerCount,
		}).
		Debug("Initiate peer disconnection")

	s.peers.SetConnectionState(remotePeerID, peers.PeerDisconnected)
}

// AddConnectionHandler adds a callback function which handles the connection with a
// newly added peer. It performs a handshake with that peer by sending a hello request
// and validating the response from the peer.
func (s *Service) AddConnectionHandler(reqFunc, goodByeFunc func(ctx context.Context, id peer.ID) error) {
	// Peer map and lock to keep track of current connection attempts.
	peerMap := make(map[peer.ID]bool)
	peerLock := new(sync.Mutex)

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
		ConnectedF: func(net network.Network, conn network.Conn) {
			remotePeer := conn.RemotePeer()

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
				if err == nil && (peerConnectionState == peers.PeerConnected || peerConnectionState == peers.PeerConnecting) {
					log.WithField("currentState", peerConnectionState).WithField("reason", "already active").Trace("Ignoring connection request")
					return
				}

				s.peers.Add(nil /* ENR */, remotePeer, conn.RemoteMultiaddr(), conn.Stat().Direction)

				// Defensive check in the event we still get a bad peer.
				if err := s.peers.IsBad(remotePeer); err != nil {
					s.disconnectFromPeer(conn, goodByeFunc, err)
					return
				}

				// Do not perform handshake on inbound dials.
				if conn.Stat().Direction == network.DirInbound {
					_, err := s.peers.ChainState(remotePeer)
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
						s.disconnectFromPeer(conn, goodByeFunc, errors.Wrap(err, "chain state"))
						return
					}

					if peerExists {
						updated, err := s.peers.ChainStateLastUpdated(remotePeer)
						if err != nil {
							s.disconnectFromPeer(conn, goodByeFunc, errors.Wrap(err, "chain state last updated"))
							return
						}

						// Exit if we don't receive any current status messages from peer.
						if updated.IsZero() {
							s.disconnectFromPeer(conn, goodByeFunc, errors.New("is zero"))
							return
						}

						if !updated.After(currentTime) {
							s.disconnectFromPeer(conn, goodByeFunc, errors.New("did not update"))
							return
						}
					}

					s.connectToPeer(conn)
					return
				}

				s.peers.SetConnectionState(conn.RemotePeer(), peers.PeerConnecting)
				if err := reqFunc(context.TODO(), conn.RemotePeer()); err != nil && !errors.Is(err, io.EOF) {
					s.disconnectFromPeer(conn, goodByeFunc, err)
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
			remotePeerMultiAddr := peerMultiaddrString(conn)
			peerID := conn.RemotePeer()
			direction := conn.Stat().Direction.String()

			log := log.WithFields(logrus.Fields{
				"multiAddr": remotePeerMultiAddr,
				"direction": direction,
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
					priorState = peers.PeerDisconnected
				}

				s.peers.SetConnectionState(peerID, peers.PeerDisconnecting)
				if err := handler(context.TODO(), conn.RemotePeer()); err != nil {
					log.WithError(err).Error("Disconnect handler failed")
				}

				s.peers.SetConnectionState(peerID, peers.PeerDisconnected)

				// Only log disconnections if we were fully connected.
				if priorState == peers.PeerConnected {
					activePeersCount := len(s.peers.Active())
					log.WithField("remainingActivePeers", activePeersCount).Debug("Peer disconnected")
				}
			}()
		},
	})
}
