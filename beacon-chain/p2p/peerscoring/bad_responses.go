package peerscoring

import (
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

var _ GreyLister = badResponsesScorer{}

// badResponsesScorer greylists a peer once its un-decayed strike count reaches the
// configured threshold.
type badResponsesScorer struct{}

// Aspect returns the grey-lister's aspect name.
func (badResponsesScorer) Aspect() string { return AspectBadResponses }

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
