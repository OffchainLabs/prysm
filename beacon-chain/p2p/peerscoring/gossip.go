package peerscoring

import (
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

var _ GreyListerAndScorer = gossipScorer{}

// gossipScorer mirrors libp2p's opinion of the peer and greylists it below the threshold.
type gossipScorer struct{}

// Score returns the weighted mirrored gossip score.
func (gossipScorer) Score(_ peer.ID, si *scoringInfo) float64 {
	return si.peerInfo.gossipScore * si.params.gossipWeight
}

// IsPeerGreyListed greylists the peer when its mirrored gossip score fell below the greylist threshold.
func (gossipScorer) IsPeerGreyListed(_ peer.ID, si *scoringInfo) error {
	if si.peerInfo.gossipScore < float64(si.params.gossipGreyListThreshold) {
		return fmt.Errorf("%w: gossip score %g below threshold %d",
			ErrPeerGreyListed, si.peerInfo.gossipScore, si.params.gossipGreyListThreshold)
	}
	return nil
}

// TimeToWhiteListing returns 0: gossip scores recover via libp2p's own decay, which we cannot predict locally.
func (gossipScorer) TimeToWhiteListing(peer.ID, *scoringInfo) time.Duration {
	return 0
}
