package sync

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peerscoring"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/libp2p/go-libp2p/core/peer"
)

// setPeerStatus records a validated chain status for the peer on the scorer under test.
func setPeerStatus(scoring *peerscoring.Scorer, pid peer.ID, st *ethpb.StatusV2) {
	scoring.SetPeerStatus(pid, st, nil)
}
