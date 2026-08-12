package blocks

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// PartialAttestationPeerState tracks, per committee, which validators' slot
// attestations a peer is known to have (its claims plus everything we pushed
// to it), by global validator index. The slot is the partial-messages group.
// Owned by the gossipsub event loop.
type PartialAttestationPeerState struct {
	Available map[primitives.CommitteeIndex]map[uint64]struct{}
}
