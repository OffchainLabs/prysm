package interfaces

import (
	field_params "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/ethereum/go-ethereum/common"
	"google.golang.org/protobuf/proto"
)

type ROSignedExecutionPayloadEnvelope interface {
	Envelope() (ROExecutionPayloadEnvelope, error)
	Signature() [field_params.BLSSignatureLength]byte
	SigningRoot([]byte) ([32]byte, error)
	IsNil() bool
	Proto() proto.Message
}

type ROExecutionPayloadEnvelope interface {
	Execution() (ExecutionData, error)
	ExecutionRequests() *enginev1.ExecutionRequests
	BuilderIndex() primitives.BuilderIndex
	BeaconBlockRoot() [field_params.RootLength]byte
	BlobKzgCommitments() [][]byte
	BlobKzgCommitmentsRoot() ([field_params.RootLength]byte, error)
	VersionedHashes() []common.Hash
	Slot() primitives.Slot
	StateRoot() [field_params.RootLength]byte
	IsBlinded() bool
	IsNil() bool
}
