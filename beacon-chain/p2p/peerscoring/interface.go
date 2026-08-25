package peerscoring

import (
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/libp2p/go-libp2p/core/peer"
)

// scoringInfo is a point-in-time snapshot of everything a scorer needs to judge one peer.
type scoringInfo struct {
	params               *scoringParams
	peerInfo             *PeerScoringInfo
	ourHeadSlot          primitives.Slot
	highestKnownHeadSlot primitives.Slot
}

// GreyListerAndScorer scores one aspect of a peer's behaviour and decides whether
// that aspect alone warrants greylisting the peer.
type GreyListerAndScorer interface {
	// Score returns this scorer's weighted contribution to the composite score of a non-greylisted peer.
	Score(peer.ID, *scoringInfo) float64
	// IsPeerGreyListed returns nil when this scorer alone does not greylist the peer, or an
	// error wrapping ErrPeerGreyListed describing why it does.
	IsPeerGreyListed(peer.ID, *scoringInfo) error
	// TimeToWhiteListing estimates how long until this scorer whitelists the peer on
	// its own; 0 when the peer is not greylisted or recovery is not time based.
	TimeToWhiteListing(peer.ID, *scoringInfo) time.Duration
}
