package interfaces

import (
	field_params "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"google.golang.org/protobuf/proto"
)

type ROSignedExecutionPayloadEnvelope interface {
	Envelope() (ROExecutionPayloadEnvelope, error)
	Signature() [field_params.BLSSignatureLength]byte
	SigningRoot([]byte) ([32]byte, error)
	IsNil() bool
	Proto() proto.Message
}

// ROBlindedExecutionPayloadEnvelope contains the fields common to both
// full and blinded execution payload envelopes.
type ROBlindedExecutionPayloadEnvelope interface {
	ExecutionRequests() *enginev1.ExecutionRequests
	BuilderIndex() primitives.BuilderIndex
	BeaconBlockRoot() [field_params.RootLength]byte
	Slot() primitives.Slot
	StateRoot() [field_params.RootLength]byte
	BlockHash() [field_params.RootLength]byte
	IsBlinded() bool
	IsNil() bool
}

type ROExecutionPayloadEnvelope interface {
	ROBlindedExecutionPayloadEnvelope
	Execution() (ExecutionData, error)
}
