package peerscoring

import (
	"errors"
	"fmt"
	"time"

	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

var _ GreyLister = rpcStatusScorer{}

// terminalStatusErrors greylist a peer until a later status exchange clears them.
var terminalStatusErrors = []error{
	p2ptypes.ErrWrongForkDigestVersion,
	p2ptypes.ErrInvalidFinalizedRoot,
	p2ptypes.ErrInvalidRequest,
}

// rpcStatusScorer judges the chain view a peer advertised in its last status exchange.
type rpcStatusScorer struct{}

// Aspect returns the grey-lister's aspect name.
func (rpcStatusScorer) Aspect() string { return AspectPeerStatus }

// IsPeerGreyListed greylists the peer when its last status failed validation with a terminal error.
func (rpcStatusScorer) IsPeerGreyListed(_ peer.ID, si *scoringInfo) error {
	status := si.peerInfo.rpcStatus
	if status == nil {
		return nil
	}
	for _, terminal := range terminalStatusErrors {
		if errors.Is(status.validationError, terminal) {
			return fmt.Errorf("%w: status validation failed: %w", ErrPeerGreyListed, status.validationError)
		}
	}
	return nil
}

// TimeToWhiteListing returns 0: status greylisting only clears once a later status exchange validates.
func (rpcStatusScorer) TimeToWhiteListing(peer.ID, *scoringInfo) time.Duration {
	return 0
}
