package blocks

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	field_params "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	consensus_types "github.com/OffchainLabs/prysm/v7/consensus-types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

const xmssSignatureLength = 2404

type signedExecutionPayloadEnvelopePQ struct {
	s *ethpb.SignedExecutionPayloadEnvelopePQ
}

type executionPayloadEnvelopePQ struct {
	p *ethpb.ExecutionPayloadEnvelopePQ
}

func WrappedROSignedExecutionPayloadEnvelopePQ(s *ethpb.SignedExecutionPayloadEnvelopePQ) (interfaces.ROSignedExecutionPayloadEnvelope, error) {
	w := signedExecutionPayloadEnvelopePQ{s: s}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

func (s signedExecutionPayloadEnvelopePQ) Envelope() (interfaces.ROExecutionPayloadEnvelope, error) {
	w := &executionPayloadEnvelopePQ{p: s.s.Message}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

func (signedExecutionPayloadEnvelopePQ) Signature() [field_params.BLSSignatureLength]byte {
	return [field_params.BLSSignatureLength]byte{}
}

func (s signedExecutionPayloadEnvelopePQ) XMSSSignature() []byte {
	return s.s.Signature
}

func (s signedExecutionPayloadEnvelopePQ) IsNil() bool {
	if s.s == nil || len(s.s.Signature) != xmssSignatureLength || s.s.Message == nil || s.s.Message.ExecutionRequests == nil {
		return true
	}
	w := executionPayloadEnvelopePQ{p: s.s.Message}
	return w.IsNil()
}

func (s signedExecutionPayloadEnvelopePQ) SigningRoot(domain []byte) ([32]byte, error) {
	return signing.ComputeSigningRoot(s.s.Message, domain)
}

func (s signedExecutionPayloadEnvelopePQ) Proto() proto.Message {
	return s.s
}

func (p *executionPayloadEnvelopePQ) IsNil() bool {
	return p.p == nil || p.p.Payload == nil ||
		len(p.p.BeaconBlockRoot) != field_params.RootLength ||
		len(p.p.ParentBeaconBlockRoot) != field_params.RootLength
}

func (*executionPayloadEnvelopePQ) IsBlinded() bool {
	return false
}

func (p *executionPayloadEnvelopePQ) Execution() (interfaces.ExecutionData, error) {
	return WrappedExecutionPayloadGloas(p.p.Payload)
}

func (*executionPayloadEnvelopePQ) ExecutionRequests() *enginev1.ExecutionRequestsGloas {
	return nil
}

func (p *executionPayloadEnvelopePQ) ExecutionRequestsPQ() *enginev1.ExecutionRequestsPQ {
	return ethpb.CopyExecutionRequestsPQ(p.p.ExecutionRequests)
}

func (p *executionPayloadEnvelopePQ) BuilderIndex() primitives.BuilderIndex {
	return p.p.BuilderIndex
}

func (p *executionPayloadEnvelopePQ) BeaconBlockRoot() [field_params.RootLength]byte {
	return [field_params.RootLength]byte(p.p.BeaconBlockRoot)
}

func (p *executionPayloadEnvelopePQ) ParentBeaconBlockRoot() [field_params.RootLength]byte {
	return [field_params.RootLength]byte(p.p.ParentBeaconBlockRoot)
}

func (p *executionPayloadEnvelopePQ) Slot() primitives.Slot {
	return primitives.Slot(p.p.Payload.SlotNumber)
}

func (p *executionPayloadEnvelopePQ) BlockHash() [field_params.RootLength]byte {
	return [field_params.RootLength]byte(p.p.Payload.BlockHash)
}

func (signedExecutionPayloadEnvelope) XMSSSignature() []byte {
	return nil
}

func (*executionPayloadEnvelope) ExecutionRequestsPQ() *enginev1.ExecutionRequestsPQ {
	return nil
}

func (*blindedExecutionPayloadEnvelope) ExecutionRequestsPQ() *enginev1.ExecutionRequestsPQ {
	return nil
}
