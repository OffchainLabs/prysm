package blocks

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	field_params "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	consensus_types "github.com/prysmaticlabs/prysm/v5/consensus-types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/ssz"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
)

type signedExecutionPayloadEnvelope struct {
	s *enginev1.SignedExecutionPayloadEnvelope
}

type executionPayloadEnvelope struct {
	p    *enginev1.ExecutionPayloadEnvelope
	slot primitives.Slot
}

// WrappedROSignedExecutionPayloadEnvelope is a constructor which wraps a
// protobuf signed execution payload envelope into an interface.
func WrappedROSignedExecutionPayloadEnvelope(s *enginev1.SignedExecutionPayloadEnvelope) (interfaces.ROSignedExecutionPayloadEnvelope, error) {
	w := signedExecutionPayloadEnvelope{s: s}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

// WrappedROExecutionPayloadEnvelope is a constructor which wraps a
// protobuf execution payload envelope into an interface.
func WrappedROExecutionPayloadEnvelope(p *enginev1.ExecutionPayloadEnvelope) (interfaces.ROExecutionPayloadEnvelope, error) {
	w := &executionPayloadEnvelope{p: p}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

// Envelope returns the wrapped object as an interface
func (s signedExecutionPayloadEnvelope) Envelope() (interfaces.ROExecutionPayloadEnvelope, error) {
	return WrappedROExecutionPayloadEnvelope(s.s.Message)
}

// Signature returns the wrapped value
func (s signedExecutionPayloadEnvelope) Signature() [field_params.BLSSignatureLength]byte {
	return [field_params.BLSSignatureLength]byte(s.s.Signature)
}

// IsNil returns whether the wrapped value is nil
func (s signedExecutionPayloadEnvelope) IsNil() bool {
	if s.s == nil {
		return true
	}
	if len(s.s.Signature) != field_params.BLSSignatureLength {
		return true
	}
	w := executionPayloadEnvelope{p: s.s.Message}
	return w.IsNil()
}

// SigningRoot returns the signing root for the given domain
func (s signedExecutionPayloadEnvelope) SigningRoot(domain []byte) (root [32]byte, err error) {
	return signing.ComputeSigningRoot(s.s.Message, domain)
}

// IsNil returns whether the wrapped value is nil
func (p *executionPayloadEnvelope) IsNil() bool {
	if p.p == nil {
		return true
	}
	if p.p.Payload == nil {
		return true
	}
	if len(p.p.BeaconBlockRoot) != field_params.RootLength {
		return true
	}
	if p.p.BlobKzgCommitments == nil {
		return true
	}
	if p.p.StateRoot == nil {
		return true
	}
	return false
}

// IsBlinded returns whether the wrapped value is blinded
func (p *executionPayloadEnvelope) IsBlinded() bool {
	return !p.IsNil() && p.p.Payload == nil
}

// Execution returns the wrapped payload as an interface
func (p *executionPayloadEnvelope) Execution() (interfaces.ExecutionData, error) {
	if p.IsBlinded() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return WrappedExecutionPayloadElectra(p.p.Payload)
}

// ExecutionRequests returns the execution requests in the payload envelope
func (p *executionPayloadEnvelope) ExecutionRequests() *enginev1.ExecutionRequests {
	return p.p.ExecutionRequests
}

// BuilderIndex returns the wrapped value
func (p *executionPayloadEnvelope) BuilderIndex() primitives.ValidatorIndex {
	return p.p.BuilderIndex
}

// BeaconBlockRoot returns the wrapped value
func (p *executionPayloadEnvelope) BeaconBlockRoot() [field_params.RootLength]byte {
	return [field_params.RootLength]byte(p.p.BeaconBlockRoot)
}

// BlobKzgCommitments returns the wrapped value
func (p *executionPayloadEnvelope) BlobKzgCommitments() [][]byte {
	commitments := make([][]byte, len(p.p.BlobKzgCommitments))
	for i, commit := range p.p.BlobKzgCommitments {
		commitments[i] = make([]byte, len(commit))
		copy(commitments[i], commit)
	}
	return commitments
}

// PayloadWithheld returns the wrapped value
func (p *executionPayloadEnvelope) PayloadWithheld() bool {
	return p.p.PayloadWithheld
}

// StateRoot returns the wrapped value
func (p *executionPayloadEnvelope) StateRoot() [field_params.RootLength]byte {
	return [field_params.RootLength]byte(p.p.StateRoot)
}

// VersionedHashes returns the Versioned Hashes of the KZG commitments within
// the envelope
func (p *executionPayloadEnvelope) VersionedHashes() []common.Hash {
	commitments := p.p.BlobKzgCommitments
	versionedHashes := make([]common.Hash, len(commitments))
	for i, commitment := range commitments {
		versionedHashes[i] = primitives.ConvertKzgCommitmentToVersionedHash(commitment)
	}
	return versionedHashes
}

// BlobKzgCommitmentsRoot returns the HTR of the KZG commitments in the payload
func (p *executionPayloadEnvelope) BlobKzgCommitmentsRoot() ([field_params.RootLength]byte, error) {
	if p.IsNil() || p.p.BlobKzgCommitments == nil {
		return [field_params.RootLength]byte{}, consensus_types.ErrNilObjectWrapped
	}

	return ssz.KzgCommitmentsRoot(p.p.BlobKzgCommitments)
}

// SetSlot initializes the internal member variable
func (p *executionPayloadEnvelope) SetSlot(slot primitives.Slot) {
	p.slot = slot
}

// Slot returns the wrapped value
func (p *executionPayloadEnvelope) Slot() primitives.Slot {
	return p.slot
}
