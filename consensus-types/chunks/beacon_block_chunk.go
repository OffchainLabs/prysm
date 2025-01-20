package chunks

import (
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

var _ interfaces.BeaconBlockChunk = &BeaconBlockChunk{}

type BeaconBlockChunk struct {
	chunk      *ethpb.BeaconBlockChunk
	headerRoot [32]byte
	version    int
}

// Slot returns the slot of the beacon block chunk.
func (b *BeaconBlockChunk) Slot() primitives.Slot {
	return b.chunk.Header.Slot
}

// ProposerIndex returns the proposer index of the beacon block chunk.
func (b *BeaconBlockChunk) ProposerIndex() primitives.ValidatorIndex {
	return b.chunk.Header.ProposerIndex
}

// ParentRoot returns the parent root of the beacon block chunk.
func (b *BeaconBlockChunk) ParentRoot() [32]byte {
	return [32]byte(b.chunk.Header.ParentRoot)
}

// Commitments returns the commitments of the beacon block chunk.
func (b *BeaconBlockChunk) Commitments() [][]byte {
	return b.chunk.Header.Commitments
}

// Signature returns the signature of the beacon block chunk.
func (b *BeaconBlockChunk) Signature() [96]byte {
	return [96]byte(b.chunk.Signature)
}

// IsNil returns true if the beacon block chunk is nil.
func (b *BeaconBlockChunk) IsNil() bool {
	if b == nil || b.chunk == nil || b.chunk.Header == nil {
		return true
	}
	if b.chunk.Header.ParentRoot == nil {
		return true
	}
	if b.chunk.Header.Commitments == nil {
		return true
	}
	return b.chunk.Signature == nil
}

// Version returns the version of the beacon block chunk.
func (b *BeaconBlockChunk) Version() int {
	return b.version
}

// HeaderRoot returns the root of the beacon block chunk header
func (b *BeaconBlockChunk) HeaderRoot() [32]byte {
	return b.headerRoot
}

func NewBlockChunk(i interface{}) (*BeaconBlockChunk, error) {
	switch b := i.(type) {
	case nil:
		return nil, ErrNilObject
	case *ethpb.BeaconBlockChunk:
		root, err := b.Header.HashTreeRoot()
		if err != nil {
			return nil, err
		}
		return &BeaconBlockChunk{chunk: b, headerRoot: root, version: slotToVersion(b.Header.Slot)}, nil
	default:
		return nil, ErrInvalidType
	}
}

func slotToVersion(slot primitives.Slot) int {
	epoch := slots.ToEpoch(slot)
	cfg := params.BeaconConfig()
	if epoch < cfg.AltairForkEpoch {
		return version.Phase0
	}
	if epoch < cfg.BellatrixForkEpoch {
		return version.Altair
	}
	if epoch < cfg.CapellaForkEpoch {
		return version.Bellatrix
	}
	if epoch < cfg.DenebForkEpoch {
		return version.Capella
	}
	if epoch < cfg.ElectraForkEpoch {
		return version.Deneb
	}
	return version.Electra
}

// SetParentRoot sets the parent root of the beacon block chunk.
func (b *BeaconBlockChunk) SetParentRoot(root []byte) {
	b.chunk.Header.ParentRoot = root
}

// SetCommitments sets the commitments of the beacon block chunk.
func (b *BeaconBlockChunk) SetCommitments(commitments [][]byte) {
	b.chunk.Header.Commitments = commitments
}

// SetCoefficients sets the coefficients of the beacon block chunk.
func (b *BeaconBlockChunk) SetCoefficients(coefficients [][]byte) {
	b.chunk.Coefficients = coefficients
}

// SetData sets the data of the beacon block chunk.
func (b *BeaconBlockChunk) SetData(data []byte) {
	b.chunk.Data = data
}

// SetSignature sets the signature of the beacon block chunk.
func (b *BeaconBlockChunk) SetSignature(signature [96]byte) {
	b.chunk.Signature = signature[:]
}

// SetSlot sets the slot of the beacon block chunk.
func (b *BeaconBlockChunk) SetSlot(slot primitives.Slot) {
	b.chunk.Header.Slot = slot
}

// SetProposerIndex sets the proposer index of the beacon block chunk.
func (b *BeaconBlockChunk) SetProposerIndex(index primitives.ValidatorIndex) {
	b.chunk.Header.ProposerIndex = index
}

// SetVersion sets the version of the beacon block chunk.
func (b *BeaconBlockChunk) SetVersion(version int) {
	b.version = version
}
