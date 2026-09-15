package peerscoring

import (
	"errors"
	"fmt"
	"time"

	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

var _ GreyLister = rpcStatusScorer{}

// terminalStatusErrors greylist a peer until a later status exchange clears them or the
// verdict expires after statusGreyListTTL.
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
			// A refused peer can never clear the verdict itself, so it expires after the
			// TTL; the peer then ages out through regular peer-store pruning.
			if time.Since(status.lastUpdated) >= si.params.statusGreyListTTL {
				return nil
			}
			return fmt.Errorf("%w: status validation failed: %w", ErrPeerGreyListed, status.validationError)
		}
	}
	return nil
}

// TimeToWhiteListing returns how long until the terminal verdict expires; 0 when this
// aspect does not greylist the peer.
func (r rpcStatusScorer) TimeToWhiteListing(pid peer.ID, si *scoringInfo) time.Duration {
	if r.IsPeerGreyListed(pid, si) == nil {
		return 0
	}
	return si.params.statusGreyListTTL - time.Since(si.peerInfo.rpcStatus.lastUpdated)
}
