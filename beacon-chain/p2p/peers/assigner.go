package peers

import (
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// StatusProvider describes the minimum capability that Assigner needs from peer status tracking.
// That is, the ability to retrieve the best peers by finalized checkpoint.
type StatusProvider interface {
	BestFinalized(ourFinalized primitives.Epoch) (primitives.Epoch, []peer.ID)
}

// FinalizedCheckpointer describes the minimum capability that Assigner needs from forkchoice.
// That is, the ability to retrieve the latest finalized checkpoint to help with peer evaluation.
type FinalizedCheckpointer interface {
	FinalizedCheckpoint() *forkchoicetypes.Checkpoint
}

// NewAssigner assists in the correct construction of an Assigner by code in other packages,
// assuring all the important private member fields are given values.
// The StatusProvider is used to retrieve best peers, and FinalizedCheckpointer is used to retrieve the latest finalized checkpoint each time peers are requested.
// Peers that report an older finalized checkpoint are filtered out.
func NewAssigner(s StatusProvider, fc FinalizedCheckpointer) *Assigner {
	return &Assigner{
		ps: s,
		fc: fc,
	}
}

// Assigner uses the "BestFinalized" peer scoring method to pick the next-best peer to receive rpc requests.
type Assigner struct {
	ps StatusProvider
	fc FinalizedCheckpointer
}

// ErrInsufficientSuitable is a sentinel error, signaling that a peer couldn't be assigned because there are currently
// not enough peers that match our selection criteria to serve rpc requests. It is the responsibility of the caller to
// look for this error and continue to try calling Assign with appropriate backoff logic.
var ErrInsufficientSuitable = errors.New("no suitable peers")

func (a *Assigner) freshPeers() ([]peer.ID, error) {
	required := min(flags.Get().MinimumSyncPeers, min(flags.Get().MinimumSyncPeers, params.BeaconConfig().MaxPeersToSync))
	_, peers := a.ps.BestFinalized(a.fc.FinalizedCheckpoint().Epoch)
	if len(peers) < required {
		log.WithFields(logrus.Fields{
			"suitable": len(peers),
			"required": required}).Trace("Unable to assign peer while suitable peers < required")
		return nil, ErrInsufficientSuitable
	}
	return peers, nil
}

// AssignmentFilter describes a function that takes a list of peer.IDs and returns a filtered subset.
// An example is the NotBusy filter.
type AssignmentFilter func([]peer.ID) []peer.ID

// Assign uses the "BestFinalized" method to select the best peers that agree on a canonical block
// for the configured finalized epoch. At most `n` peers will be returned. The `busy` param can be used
// to filter out peers that we know we don't want to connect to, for instance if we are trying to limit
// the number of outbound requests to each peer from a given component.
func (a *Assigner) Assign(filter AssignmentFilter) ([]peer.ID, error) {
	best, err := a.freshPeers()
	if err != nil {
		return nil, err
	}
	return filter(best), nil
}

// NotBusy is a filter that returns the list of peer.IDs that are not in the `busy` map.
func NotBusy(busy map[peer.ID]bool) AssignmentFilter {
	return func(peers []peer.ID) []peer.ID {
		ps := make([]peer.ID, 0, len(peers))
		for _, p := range peers {
			if !busy[p] {
				ps = append(ps, p)
			}
		}
		return ps
	}
}
