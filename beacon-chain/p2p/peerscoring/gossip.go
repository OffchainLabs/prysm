package peerscoring

import (
	"fmt"
	"math"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

var _ GreyListerAndScorer = gossipScorer{}

// gossipScorer mirrors libp2p's opinion of the peer and greylists it below the threshold.
type gossipScorer struct{}

// Score returns the weighted gossip contribution: the mirrored libp2p score normalized to
// [-1, 1] — positives against the positive score cap, negatives against the greylist threshold.
func (gossipScorer) Score(_ peer.ID, si *scoringInfo) float64 {
	g := si.peerInfo.gossipScore
	if g == 0 {
		return 0
	}
	var score float64
	if g > 0 {
		score = math.Min(1, g/si.params.gossipPositiveScoreCap)
	} else {
		score = math.Max(-1, g/math.Abs(float64(si.params.gossipGreyListThreshold)))
	}
	return score * si.params.gossipWeight
}

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
