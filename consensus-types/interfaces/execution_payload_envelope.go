package interfaces

import (
	"github.com/ethereum/go-ethereum/common"
	field_params "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
)

type ROSignedExecutionPayloadEnvelope interface {
	Envelope() (ROExecutionPayloadEnvelope, error)
	Signature() [field_params.BLSSignatureLength]byte
	SigningRoot([]byte) ([32]byte, error)
	IsNil() bool
}

type ROExecutionPayloadEnvelope interface {
	Execution() (ExecutionData, error)
	ExecutionRequests() *enginev1.ExecutionRequests
	BuilderIndex() primitives.ValidatorIndex
	BeaconBlockRoot() [field_params.RootLength]byte
	BlobKzgCommitments() [][]byte
	BlobKzgCommitmentsRoot() ([field_params.RootLength]byte, error)
	VersionedHashes() []common.Hash
	PayloadWithheld() bool
	StateRoot() [field_params.RootLength]byte
	SetSlot(primitives.Slot)
	Slot() primitives.Slot
	IsBlinded() bool
	IsNil() bool
}
