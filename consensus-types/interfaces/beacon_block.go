package interfaces

import (
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	field_params "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	"google.golang.org/protobuf/proto"
)

var ErrIncompatibleFork = errors.New("Can't convert to fork-specific interface")

// ReadOnlySignedBeaconBlock is an interface describing the method set of
// a signed beacon block.
type ReadOnlySignedBeaconBlock interface {
	Block() ReadOnlyBeaconBlock
	Signature() [field_params.BLSSignatureLength]byte
	IsNil() bool
	Copy() (SignedBeaconBlock, error)
	Proto() (proto.Message, error)
	PbGenericBlock() (*ethpb.GenericSignedBeaconBlock, error)
	ToBlinded() (ReadOnlySignedBeaconBlock, error)
	ssz.Marshaler
	ssz.Unmarshaler
	Version() int
	IsBlinded() bool
	Header() (*ethpb.SignedBeaconBlockHeader, error)
}

// ReadOnlyBeaconBlock describes an interface which states the methods
// employed by an object that is a beacon block.
type ReadOnlyBeaconBlock interface {
	Slot() primitives.Slot
	ProposerIndex() primitives.ValidatorIndex
	ParentRoot() [field_params.RootLength]byte
	StateRoot() [field_params.RootLength]byte
	Body() ReadOnlyBeaconBlockBody
	IsNil() bool
	IsBlinded() bool
	HashTreeRoot() ([field_params.RootLength]byte, error)
	Proto() (proto.Message, error)
	ssz.Marshaler
	ssz.Unmarshaler
	ssz.HashRoot
	Version() int
	AsSignRequestObject() (validatorpb.SignRequestObject, error)
	Copy() (ReadOnlyBeaconBlock, error)
}

// ReadOnlyBeaconBlockBody describes the method set employed by an object
// that is a beacon block body.
type ReadOnlyBeaconBlockBody interface {
	Version() int
	RandaoReveal() [field_params.BLSSignatureLength]byte
	Eth1Data() *ethpb.Eth1Data
	Graffiti() [field_params.RootLength]byte
	ProposerSlashings() []*ethpb.ProposerSlashing
	AttesterSlashings() []ethpb.AttSlashing
	Attestations() []ethpb.Att
	Deposits() []*ethpb.Deposit
	VoluntaryExits() []*ethpb.SignedVoluntaryExit
	SyncAggregate() (*ethpb.SyncAggregate, error)
	IsNil() bool
	HashTreeRoot() ([field_params.RootLength]byte, error)
	Proto() (proto.Message, error)
	Execution() (ExecutionData, error)
	BLSToExecutionChanges() ([]*ethpb.SignedBLSToExecutionChange, error)
	BlobKzgCommitments() ([][]byte, error)
	ExecutionRequests() (*enginev1.ExecutionRequests, error)
	PayloadAttestations() ([]*ethpb.PayloadAttestation, error)
	SignedExecutionPayloadHeader() (ROSignedExecutionPayloadHeader, error)
}

type SignedBeaconBlock interface {
	ReadOnlySignedBeaconBlock
	SetExecution(ExecutionData) error
	SetBLSToExecutionChanges([]*ethpb.SignedBLSToExecutionChange) error
	SetBlobKzgCommitments(c [][]byte) error
	SetSyncAggregate(*ethpb.SyncAggregate) error
	SetVoluntaryExits([]*ethpb.SignedVoluntaryExit)
	SetDeposits([]*ethpb.Deposit)
	SetAttestations([]ethpb.Att) error
	SetAttesterSlashings([]ethpb.AttSlashing) error
	SetProposerSlashings([]*ethpb.ProposerSlashing)
	SetGraffiti([]byte)
	SetEth1Data(*ethpb.Eth1Data)
	SetRandaoReveal([]byte)
	SetStateRoot([]byte)
	SetParentRoot([]byte)
	SetProposerIndex(idx primitives.ValidatorIndex)
	SetSlot(slot primitives.Slot)
	SetSignature(sig []byte)
	SetExecutionRequests(er *enginev1.ExecutionRequests) error
	Unblind(e ExecutionData) error
	SetSignedExecutionPayloadHeader(h *enginev1.SignedExecutionPayloadHeader) error
	SetPayloadAttestations([]*ethpb.PayloadAttestation) error
}

// ExecutionData represents execution layer information that is contained
// within post-Bellatrix beacon block bodies.
type ExecutionData interface {
	ssz.Marshaler
	ssz.Unmarshaler
	ssz.HashRoot
	IsNil() bool
	IsBlinded() bool
	Proto() proto.Message
	ParentHash() []byte
	FeeRecipient() []byte
	StateRoot() []byte
	ReceiptsRoot() []byte
	LogsBloom() []byte
	PrevRandao() []byte
	BlockNumber() uint64
	GasLimit() uint64
	GasUsed() uint64
	Timestamp() uint64
	ExtraData() []byte
	BaseFeePerGas() []byte
	BlobGasUsed() (uint64, error)
	ExcessBlobGas() (uint64, error)
	BlockHash() []byte
	Transactions() ([][]byte, error)
	TransactionsRoot() ([]byte, error)
	Withdrawals() ([]*enginev1.Withdrawal, error)
	WithdrawalsRoot() ([]byte, error)
}
