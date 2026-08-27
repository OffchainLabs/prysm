package peerscoring

import (
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

var _ GreyLister = gossipScorer{}

// gossipScorer mirrors libp2p's opinion of the peer and greylists it below the threshold.
type gossipScorer struct{}

// IsPeerGreyListed greylists the peer when its mirrored gossip score fell below the greylist threshold.
func (gossipScorer) IsPeerGreyListed(_ peer.ID, si *scoringInfo) error {
	if si.peerInfo.gossipScore < float64(si.params.gossipGreyListThreshold) {
		return fmt.Errorf("%w: gossip score %g below threshold %d",
			ErrPeerGreyListed, si.peerInfo.gossipScore, si.params.gossipGreyListThreshold)
	}
	return nil
}

// TimeToWhiteListing returns 0: gossip recovery is libp2p-driven (decay while connected,
// retention expiry mirrored via ReconcileGossipScores after disconnect) and not locally predictable.
func (gossipScorer) TimeToWhiteListing(peer.ID, *scoringInfo) time.Duration {
	return 0
}
