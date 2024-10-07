package blocks

import (
	consensus_types "github.com/prysmaticlabs/prysm/v5/consensus-types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
)

type signedExecutionPayloadHeader struct {
	s *enginev1.SignedExecutionPayloadHeader
}

type executionPayloadHeaderEPBS struct {
	p *enginev1.ExecutionPayloadHeaderEPBS
}

// IsNil returns whether the wrapped value is nil
func (s signedExecutionPayloadHeader) IsNil() bool {
	if s.s == nil {
		return true
	}
	_, err := WrappedROExecutionPayloadHeaderEPBS(s.s.Message)
	if err != nil {
		return true
	}
	return len(s.s.Signature) != 96
}

// IsNil returns whether the wrapped value is nil
func (p executionPayloadHeaderEPBS) IsNil() bool {
	if p.p == nil {
		return true
	}
	if len(p.p.ParentBlockHash) != 32 {
		return true
	}
	if len(p.p.ParentBlockRoot) != 32 {
		return true
	}
	if len(p.p.BlockHash) != 32 {
		return true
	}
	return len(p.p.BlobKzgCommitmentsRoot) != 32
}

// WrappedROSignedExecutionPayloadHeader is a constructor which wraps a
// protobuf signed execution payload header into an interface.
func WrappedROSignedExecutionPayloadHeader(s *enginev1.SignedExecutionPayloadHeader) (interfaces.ROSignedExecutionPayloadHeader, error) {
	w := signedExecutionPayloadHeader{s: s}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

// WrappedROExecutionPayloadHeaderEPBS is a constructor which wraps a
// protobuf execution payload header into an interface.
func WrappedROExecutionPayloadHeaderEPBS(p *enginev1.ExecutionPayloadHeaderEPBS) (interfaces.ROExecutionPayloadHeaderEPBS, error) {
	w := executionPayloadHeaderEPBS{p: p}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

// Header returns the wrapped object as an interface
func (s signedExecutionPayloadHeader) Header() (interfaces.ROExecutionPayloadHeaderEPBS, error) {
	return WrappedROExecutionPayloadHeaderEPBS(s.s.Message)
}

// Signature returns the wrapped signature
func (s signedExecutionPayloadHeader) Signature() [96]byte {
	return [96]byte(s.s.Signature)
}

// ParentBlockHash returns the wrapped value
func (p executionPayloadHeaderEPBS) ParentBlockHash() [32]byte {
	return [32]byte(p.p.ParentBlockHash)
}

// ParentBlockRoot returns the wrapped value
func (p executionPayloadHeaderEPBS) ParentBlockRoot() [32]byte {
	return [32]byte(p.p.ParentBlockRoot)
}

// BlockHash returns the wrapped value
func (p executionPayloadHeaderEPBS) BlockHash() [32]byte {
	return [32]byte(p.p.BlockHash)
}

// GasLimit returns the wrapped value
func (p executionPayloadHeaderEPBS) GasLimit() uint64 {
	return p.p.GasLimit
}

// BuilderIndex returns the wrapped value
func (p executionPayloadHeaderEPBS) BuilderIndex() primitives.ValidatorIndex {
	return p.p.BuilderIndex
}

// Slot returns the wrapped value
func (p executionPayloadHeaderEPBS) Slot() primitives.Slot {
	return p.p.Slot
}

// Value returns the wrapped value
func (p executionPayloadHeaderEPBS) Value() primitives.Gwei {
	return primitives.Gwei(p.p.Value)
}

// BlobKzgCommitmentsRoot returns the wrapped value
func (p executionPayloadHeaderEPBS) BlobKzgCommitmentsRoot() [32]byte {
	return [32]byte(p.p.BlobKzgCommitmentsRoot)
}
