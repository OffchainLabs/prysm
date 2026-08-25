package peerscoring

import (
	"errors"
	"math"
	"time"

	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

var _ GreyListerAndScorer = rpcStatusScorer{}

// terminalStatusErrors greylist a peer until a later status exchange clears them.
var terminalStatusErrors = []error{
	p2ptypes.ErrWrongForkDigestVersion,
	p2ptypes.ErrInvalidFinalizedRoot,
	p2ptypes.ErrInvalidRequest,
}

// rpcStatusScorer judges the chain view a peer advertised in its last status exchange.
type rpcStatusScorer struct{}

// Score returns the weighted status score: how close the peer's advertised head is to the highest known head.
func (rpcStatusScorer) Score(_ peer.ID, si *scoringInfo) float64 {
	status := si.peerInfo.rpcStatus
	if status == nil || status.chainState == nil {
		return 0
	}
	if status.chainState.HeadSlot < si.ourHeadSlot || si.highestKnownHeadSlot == 0 {
		return 0
	}
	score := float64(status.chainState.HeadSlot) / float64(si.highestKnownHeadSlot)
	score = math.Round(score*scoreRoundingFactor) / scoreRoundingFactor
	return score * si.params.peerStatusWeight
}

// IsPeerGreylisted reports whether the peer's last status failed validation with a terminal error.
func (rpcStatusScorer) IsPeerGreylisted(_ peer.ID, si *scoringInfo) bool {
	status := si.peerInfo.rpcStatus
	if status == nil {
		return false
	}
	for _, terminal := range terminalStatusErrors {
		if errors.Is(status.validationError, terminal) {
			return true
		}
	}
	return false
}

// TimeToWhitelisting returns 0: status greylisting only clears once a later status exchange validates.
func (rpcStatusScorer) TimeToWhitelisting(peer.ID, *scoringInfo) time.Duration {
	return 0
}
