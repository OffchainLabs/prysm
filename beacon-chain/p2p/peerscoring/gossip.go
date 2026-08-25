package peerscoring

import (
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

// IsPeerGreylisted reports whether the mirrored gossip score fell below the greylist threshold.
func (gossipScorer) IsPeerGreylisted(_ peer.ID, si *scoringInfo) bool {
	return si.peerInfo.gossipScore < float64(si.params.gossipGreylistThreshold)
}

// TimeToWhitelisting returns 0: gossip scores recover via libp2p's own decay, which we cannot predict locally.
func (gossipScorer) TimeToWhitelisting(peer.ID, *scoringInfo) time.Duration {
	return 0
}
