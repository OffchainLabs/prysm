package peerscoring

import (
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

var _ GreyListerAndScorer = badResponsesScorer{}

// badResponsesPenaltyFactor scales the per-strike penalty relative to the greylist threshold.
const badResponsesPenaltyFactor = 10

// badResponsesScorer penalizes recorded bad responses and greylists a peer once its
// un-decayed strike count reaches the configured threshold.
type badResponsesScorer struct{}

// Score returns the weighted bad-responses penalty, proportional to the standing strike count.
func (badResponsesScorer) Score(_ peer.ID, si *scoringInfo) float64 {
	strikes := effectiveBadResponses(si.peerInfo)
	if strikes == 0 {
		return 0
	}
	score := -badResponsesPenaltyFactor * float64(strikes) / float64(si.params.badResponseGreylistThreshold)
	return score * si.params.badResponseWeight
}

// IsPeerGreylisted reports whether the peer's un-decayed strikes reached the threshold.
func (badResponsesScorer) IsPeerGreylisted(_ peer.ID, si *scoringInfo) bool {
	return effectiveBadResponses(si.peerInfo) >= si.params.badResponseGreylistThreshold
}

// TimeToWhitelisting returns how long the decay loop needs to drop the peer back under the threshold.
func (b badResponsesScorer) TimeToWhitelisting(pid peer.ID, si *scoringInfo) time.Duration {
	if !b.IsPeerGreylisted(pid, si) {
		return 0
	}
	decaysNeeded := effectiveBadResponses(si.peerInfo) - si.params.badResponseGreylistThreshold + 1
	return time.Duration(decaysNeeded) * si.params.decayInterval
}

// effectiveBadResponses is the strike count still standing after decay.
func effectiveBadResponses(pi *PeerScoringInfo) int {
	return len(pi.badResponses) - pi.nDecays
}
