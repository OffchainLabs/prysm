package peerscoring

import (
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// scoringInfo is a point-in-time snapshot of everything a grey-lister needs to judge one peer.
type scoringInfo struct {
	params   *scoringParams
	peerInfo *PeerScoringInfo
}

// GreyLister judges one aspect of a peer's behaviour and decides whether
// that aspect alone warrants greylisting the peer.
type GreyLister interface {
	// IsPeerGreyListed returns nil when this grey-lister alone does not greylist the peer, or an
	// error wrapping ErrPeerGreyListed describing why it does.
	IsPeerGreyListed(peer.ID, *scoringInfo) error
	// TimeToWhiteListing estimates how long until this grey-lister whitelists the peer on
	// its own; 0 when the peer is not greylisted or recovery is not time based.
	TimeToWhiteListing(peer.ID, *scoringInfo) time.Duration
}
