package types

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// ChunkKind to differentiate what kind of span we are working
// with for slashing detection, either min or max span.
type ChunkKind uint

const (
	MinSpan ChunkKind = iota
	MaxSpan
)

// String returns the string representation of the chunk kind.
func (c ChunkKind) String() string {
	switch c {
	case MinSpan:
		return "minspan"
	case MaxSpan:
		return "maxspan"
	default:
		return "unknown"
	}
}

// WrappedIndexedAtt is a wrapper over the IndexedAtt interface.
// The wrapper is needed to overcome the limitation of the event feed library
// which doesn't work well with interface types.
type WrappedIndexedAtt struct {
	ethpb.IndexedAtt
}

// IndexedAttestationWrapper contains an indexed attestation with its
// data root to reduce duplicated computation.
type IndexedAttestationWrapper struct {
	IndexedAttestation ethpb.IndexedAtt
	DataRoot           [32]byte
}

// AttesterDoubleVote represents a double vote instance
// which is a slashable event for attesters.
type AttesterDoubleVote struct {
	Target         primitives.Epoch
	ValidatorIndex primitives.ValidatorIndex
	Wrapper_1      *IndexedAttestationWrapper
	Wrapper_2      *IndexedAttestationWrapper
}

// DoubleBlockProposal containing an incoming and an existing proposal's signing root.
type DoubleBlockProposal struct {
	Slot                   primitives.Slot
	ValidatorIndex         primitives.ValidatorIndex
	PrevBeaconBlockWrapper *SignedBlockHeaderWrapper
	BeaconBlockWrapper     *SignedBlockHeaderWrapper
}

// SignedBlockHeaderWrapper contains an signed beacon block header with its
// header root to reduce duplicated computation.
type SignedBlockHeaderWrapper struct {
	SignedBeaconBlockHeader *ethpb.SignedBeaconBlockHeader
	HeaderRoot              [32]byte
}

// AttestedEpochForValidator encapsulates a previously attested epoch
// for a validator index.
type AttestedEpochForValidator struct {
	ValidatorIndex primitives.ValidatorIndex
	Epoch          primitives.Epoch
}
