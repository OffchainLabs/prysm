package epbs

import (
	"github.com/ethereum/go-ethereum/common"
	field_params "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	consensus_types "github.com/prysmaticlabs/prysm/v5/consensus-types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/ssz"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
)

type signedExecutionPayloadEnvelope struct {
	s *enginev1.SignedExecutionPayloadEnvelope
}

type executionPayloadEnvelope struct {
	p *enginev1.ExecutionPayloadEnvelope
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
	w := executionPayloadEnvelope{p: p}
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
func (s signedExecutionPayloadEnvelope) Signature() ([field_params.BLSSignatureLength]byte, error) {
	if s.IsNil() {
		return [field_params.BLSSignatureLength]byte{}, consensus_types.ErrNilObjectWrapped
	}
	return [field_params.BLSSignatureLength]byte(s.s.Signature), nil
}

// IsNil returns whether the wrapped value is nil
func (s signedExecutionPayloadEnvelope) IsNil() bool {
	return s.s == nil
}

// IsNil returns whether the wrapped value is nil
func (p executionPayloadEnvelope) IsNil() bool {
	return p.p == nil
}

// IsBlinded returns whether the wrapped value is blinded
func (p executionPayloadEnvelope) IsBlinded() bool {
	return !p.IsNil() && p.p.Payload == nil
}

// Execution returns the wrapped payload as an interface
func (p executionPayloadEnvelope) Execution() (interfaces.ExecutionData, error) {
	if p.IsBlinded() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return blocks.WrappedExecutionPayloadElectra(p.p.Payload)
}

// BuilderIndex returns the wrapped value
func (p executionPayloadEnvelope) BuilderIndex() (primitives.ValidatorIndex, error) {
	if p.IsNil() {
		return 0, consensus_types.ErrNilObjectWrapped
	}
	return p.p.BuilderIndex, nil
}

// BeaconBlockRoot returns the wrapped value
func (p executionPayloadEnvelope) BeaconBlockRoot() ([field_params.RootLength]byte, error) {
	if p.IsNil() || len(p.p.BeaconBlockRoot) == 0 {
		return [field_params.RootLength]byte{}, consensus_types.ErrNilObjectWrapped
	}
	return [field_params.RootLength]byte(p.p.BeaconBlockRoot), nil
}

// BlobKzgCommitments returns the wrapped value
func (p executionPayloadEnvelope) BlobKzgCommitments() ([][]byte, error) {
	if p.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	commitments := make([][]byte, len(p.p.BlobKzgCommitments))
	for i, commit := range p.p.BlobKzgCommitments {
		commitments[i] = make([]byte, len(commit))
		copy(commitments[i], commit)
	}
	return commitments, nil
}

// PayloadWithheld returns the wrapped value
func (p executionPayloadEnvelope) PayloadWithheld() (bool, error) {
	if p.IsBlinded() {
		return false, consensus_types.ErrNilObjectWrapped
	}
	return p.p.PayloadWithheld, nil
}

// StateRoot returns the wrapped value
func (p executionPayloadEnvelope) StateRoot() ([field_params.RootLength]byte, error) {
	if p.IsNil() || len(p.p.StateRoot) == 0 {
		return [field_params.RootLength]byte{}, consensus_types.ErrNilObjectWrapped
	}
	return [field_params.RootLength]byte(p.p.StateRoot), nil
}

// VersionedHashes returns the Versioned Hashes of the KZG commitments within
// the envelope
func (p executionPayloadEnvelope) VersionedHashes() ([]common.Hash, error) {
	if p.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}

	commitments := p.p.BlobKzgCommitments
	versionedHashes := make([]common.Hash, len(commitments))
	for i, commitment := range commitments {
		versionedHashes[i] = primitives.ConvertKzgCommitmentToVersionedHash(commitment)
	}
	return versionedHashes, nil
}

// BlobKzgCommitmentsRoot returns the HTR of the KZG commitments in the payload
func (p executionPayloadEnvelope) BlobKzgCommitmentsRoot() ([field_params.RootLength]byte, error) {
	if p.IsNil() || p.p.BlobKzgCommitments == nil {
		return [field_params.RootLength]byte{}, consensus_types.ErrNilObjectWrapped
	}

	return ssz.KzgCommitmentsRoot(p.p.BlobKzgCommitments)
}
