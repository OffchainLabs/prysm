package peerscoring

import (
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

var _ GreyLister = strikesScorer{}

// strikesScorer greylists a peer once its un-decayed strike count reaches the
// configured threshold.
type strikesScorer struct{}

// Aspect returns the grey-lister's aspect name.
func (strikesScorer) Aspect() string { return AspectStrikes }

// IsPeerGreyListed greylists the peer once its un-decayed strikes reach the threshold.
func (strikesScorer) IsPeerGreyListed(_ peer.ID, si *scoringInfo) error {
	strikes := si.peerInfo.strikeCount
	if strikes < si.params.strikeGreyListThreshold {
		return nil
	}
	if history := si.peerInfo.strikes; len(history) > 0 {
		last := history[len(history)-1]
		return fmt.Errorf("%w: %d standing strikes (threshold %d), last: %s/%s",
			ErrPeerGreyListed, strikes, si.params.strikeGreyListThreshold, last.Source, last.Reason)
	}
	return fmt.Errorf("%w: %d standing strikes (threshold %d)",
		ErrPeerGreyListed, strikes, si.params.strikeGreyListThreshold)
}

// TimeToWhiteListing returns how long the decay loop needs to drop the peer back under the threshold.
func (b strikesScorer) TimeToWhiteListing(pid peer.ID, si *scoringInfo) time.Duration {
	if b.IsPeerGreyListed(pid, si) == nil {
		return 0
	}
	decaysNeeded := si.peerInfo.strikeCount - si.params.strikeGreyListThreshold + 1
	return time.Duration(decaysNeeded) * si.params.decayInterval
}
