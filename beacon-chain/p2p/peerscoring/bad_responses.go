package peerscoring

import (
	"fmt"
	"math"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

var _ GreyListerAndScorer = badResponsesScorer{}

// badResponsesScorer penalizes recorded bad responses and greylists a peer once its
// un-decayed strike count reaches the configured threshold.
type badResponsesScorer struct{}

// Score returns the weighted bad-responses penalty: the standing strike count normalized
// against the greylist threshold to [-1, 0].
func (badResponsesScorer) Score(_ peer.ID, si *scoringInfo) float64 {
	strikes := si.peerInfo.badResponseCount
	if strikes == 0 {
		return 0
	}
	score := -math.Min(1, float64(strikes)/float64(si.params.badResponseGreyListThreshold))
	return score * si.params.badResponseWeight
}

// IsPeerGreyListed greylists the peer once its un-decayed strikes reach the threshold.
func (badResponsesScorer) IsPeerGreyListed(_ peer.ID, si *scoringInfo) error {
	strikes := si.peerInfo.badResponseCount
	if strikes < si.params.badResponseGreyListThreshold {
		return nil
	}
	if history := si.peerInfo.badResponses; len(history) > 0 {
		last := history[len(history)-1]
		return fmt.Errorf("%w: %d standing bad responses (threshold %d), last: %s/%s",
			ErrPeerGreyListed, strikes, si.params.badResponseGreyListThreshold, last.Source, last.Reason)
	}
	return fmt.Errorf("%w: %d standing bad responses (threshold %d)",
		ErrPeerGreyListed, strikes, si.params.badResponseGreyListThreshold)
}

// TimeToWhiteListing returns how long the decay loop needs to drop the peer back under the threshold.
func (b badResponsesScorer) TimeToWhiteListing(pid peer.ID, si *scoringInfo) time.Duration {
	if b.IsPeerGreyListed(pid, si) == nil {
		return 0
	}
	decaysNeeded := si.peerInfo.badResponseCount - si.params.badResponseGreyListThreshold + 1
	return time.Duration(decaysNeeded) * si.params.decayInterval
}
