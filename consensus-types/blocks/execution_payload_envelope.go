package blocks

import (
	"bytes"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	field_params "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	consensus_types "github.com/OffchainLabs/prysm/v7/consensus-types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

type signedExecutionPayloadEnvelope struct {
	s *ethpb.SignedExecutionPayloadEnvelope
}

type executionPayloadEnvelope struct {
	p *ethpb.ExecutionPayloadEnvelope
}

// WrappedROSignedExecutionPayloadEnvelope wraps a signed execution payload envelope proto in a read-only interface.
func WrappedROSignedExecutionPayloadEnvelope(s *ethpb.SignedExecutionPayloadEnvelope) (interfaces.ROSignedExecutionPayloadEnvelope, error) {
	w := signedExecutionPayloadEnvelope{s: s}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

// WrappedROExecutionPayloadEnvelope wraps an execution payload envelope proto in a read-only interface.
func WrappedROExecutionPayloadEnvelope(p *ethpb.ExecutionPayloadEnvelope) (interfaces.ROExecutionPayloadEnvelope, error) {
	w := &executionPayloadEnvelope{p: p}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

// Envelope returns the execution payload envelope as a read-only interface.
func (s signedExecutionPayloadEnvelope) Envelope() (interfaces.ROExecutionPayloadEnvelope, error) {
	return WrappedROExecutionPayloadEnvelope(s.s.Message)
}

// Signature returns the BLS signature as a 96-byte array.
func (s signedExecutionPayloadEnvelope) Signature() [field_params.BLSSignatureLength]byte {
	return [field_params.BLSSignatureLength]byte(s.s.Signature)
}

// IsNil reports whether the signed envelope or its contents are invalid.
func (s signedExecutionPayloadEnvelope) IsNil() bool {
	if s.s == nil {
		return true
	}
	if len(s.s.Signature) != field_params.BLSSignatureLength {
		return true
	}
	if s.s.Message == nil {
		return true
	}
	if len(s.s.Message.BeaconBlockRoot) != field_params.RootLength {
		return true
	}
	if len(s.s.Message.StateRoot) != field_params.RootLength {
		return true
	}
	if s.s.Message.ExecutionRequests == nil {
		return true
	}
	if s.s.Message.Payload == nil {
		return true
	}
	w := executionPayloadEnvelope{p: s.s.Message}
	return w.IsNil()
}

// SigningRoot computes the signing root for the signed envelope with the provided domain.
func (s signedExecutionPayloadEnvelope) SigningRoot(domain []byte) (root [32]byte, err error) {
	return signing.ComputeSigningRoot(s.s.Message, domain)
}

// Proto returns the underlying protobuf message.
func (s signedExecutionPayloadEnvelope) Proto() proto.Message {
	return s.s
}

// IsNil reports whether the envelope or its required fields are invalid.
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
	return false
}

// IsBlinded reports whether the envelope contains a blinded payload.
func (p *executionPayloadEnvelope) IsBlinded() bool {
	return false
}

// Execution returns the execution payload as a read-only interface.
func (p *executionPayloadEnvelope) Execution() (interfaces.ExecutionData, error) {
	return WrappedExecutionPayloadGloas(p.p.Payload)
}

// ExecutionRequests returns the execution requests attached to the envelope.
func (p *executionPayloadEnvelope) ExecutionRequests() *enginev1.ExecutionRequests {
	return ethpb.CopyExecutionRequests(p.p.ExecutionRequests)
}

// BuilderIndex returns the proposer/builder index for the envelope.
func (p *executionPayloadEnvelope) BuilderIndex() primitives.BuilderIndex {
	return p.p.BuilderIndex
}

// BeaconBlockRoot returns the beacon block root referenced by the envelope.
func (p *executionPayloadEnvelope) BeaconBlockRoot() [field_params.RootLength]byte {
	return [field_params.RootLength]byte(p.p.BeaconBlockRoot)
}

// Slot returns the slot derived from the payload's slot_number field.
func (p *executionPayloadEnvelope) Slot() primitives.Slot {
	return primitives.Slot(p.p.Payload.SlotNumber)
}

// StateRoot returns the state root carried by the envelope.
func (p *executionPayloadEnvelope) StateRoot() [field_params.RootLength]byte {
	return [field_params.RootLength]byte(p.p.StateRoot)
}

// BlockHash returns the block hash from the execution payload.
func (p *executionPayloadEnvelope) BlockHash() [field_params.RootLength]byte {
	return [field_params.RootLength]byte(p.p.Payload.BlockHash)
}

type blindedExecutionPayloadEnvelope struct {
	p *ethpb.BlindedExecutionPayloadEnvelope
}

// WrappedROBlindedExecutionPayloadEnvelope wraps a blinded execution payload envelope proto in a read-only interface.
func WrappedROBlindedExecutionPayloadEnvelope(p *ethpb.BlindedExecutionPayloadEnvelope) (interfaces.ROBlindedExecutionPayloadEnvelope, error) {
	w := &blindedExecutionPayloadEnvelope{p: p}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

func (p *blindedExecutionPayloadEnvelope) IsNil() bool {
	if p.p == nil {
		return true
	}
	if len(p.p.BeaconBlockRoot) != field_params.RootLength {
		return true
	}
	if len(p.p.BlockHash) != field_params.RootLength {
		return true
	}
	return false
}

func (p *blindedExecutionPayloadEnvelope) IsBlinded() bool {
	return true
}

func (p *blindedExecutionPayloadEnvelope) ExecutionRequests() *enginev1.ExecutionRequests {
	return ethpb.CopyExecutionRequests(p.p.ExecutionRequests)
}

func (p *blindedExecutionPayloadEnvelope) BuilderIndex() primitives.BuilderIndex {
	return p.p.BuilderIndex
}

func (p *blindedExecutionPayloadEnvelope) BeaconBlockRoot() [field_params.RootLength]byte {
	return [field_params.RootLength]byte(p.p.BeaconBlockRoot)
}

func (p *blindedExecutionPayloadEnvelope) Slot() primitives.Slot {
	return p.p.Slot
}

func (p *blindedExecutionPayloadEnvelope) StateRoot() [field_params.RootLength]byte {
	return [field_params.RootLength]byte(p.p.StateRoot)
}

func (p *blindedExecutionPayloadEnvelope) BlockHash() [field_params.RootLength]byte {
	return [field_params.RootLength]byte(p.p.BlockHash)
}

// BlockBuiltOnEnvelope checks if the block's parent hash matches the envelope's execution block hash.
func BlockBuiltOnEnvelope(env interfaces.ROSignedExecutionPayloadEnvelope, blk ROBlock) (bool, error) {
	msg, err := env.Envelope()
	if err != nil {
		return false, err
	}
	ex, err := msg.Execution()
	if err != nil {
		return false, err
	}
	ph, err := blk.ParentHash()
	if err != nil {
		return false, err
	}
	return bytes.Equal(ex.BlockHash(), ph[:]), nil
}
