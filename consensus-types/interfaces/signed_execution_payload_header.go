package interfaces

import (
	field_params "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

type ROSignedExecutionPayloadHeader interface {
	Header() (ROExecutionPayloadHeaderEPBS, error)
	Signature() [field_params.BLSSignatureLength]byte
	SigningRoot([]byte) ([32]byte, error)
	IsNil() bool
}

type ROExecutionPayloadHeaderEPBS interface {
	ParentBlockHash() [32]byte
	ParentBlockRoot() [32]byte
	BlockHash() [32]byte
	GasLimit() uint64
	BuilderIndex() primitives.ValidatorIndex
	Slot() primitives.Slot
	Value() primitives.Gwei
	BlobKzgCommitmentsRoot() [32]byte
	IsNil() bool
}
