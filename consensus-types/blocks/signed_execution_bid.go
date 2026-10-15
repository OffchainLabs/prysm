package blocks

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/methodical-ssz/ssz"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	consensus_types "github.com/OffchainLabs/prysm/v7/consensus-types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

// bidMessage is the getter set shared by the Gloas and Heze execution payload bid protobufs.
type bidMessage interface {
	proto.Message
	ssz.HashRoot
	GetParentBlockHash() []byte
	GetParentBlockRoot() []byte
	GetBlockHash() []byte
	GetPrevRandao() []byte
	GetFeeRecipient() []byte
	GetGasLimit() uint64
	GetBuilderIndex() primitives.BuilderIndex
	GetSlot() primitives.Slot
	GetValue() primitives.Gwei
	GetExecutionPayment() primitives.Gwei
	GetBlobKzgCommitments() [][]byte
	GetExecutionRequestsRoot() []byte
}

// signedBidMessage is the protobuf shape shared by the Gloas and Heze signed execution payload bids.
type signedBidMessage interface {
	proto.Message
	ssz.HashRoot
	GetSignature() []byte
}

// signedExecutionPayloadBid wraps a fork-specific signed execution payload bid
// and implements the ROSignedExecutionPayloadBid interface.
type signedExecutionPayloadBid struct {
	bid     signedBidMessage
	message bidMessage
}

// executionPayloadBid wraps a fork-specific execution payload bid
// and implements the ROExecutionPayloadBid interface.
type executionPayloadBid struct {
	payload           bidMessage
	inclusionListBits bitfield.Bitvector16 // Nil before Heze.
}

// IsNil checks if the signed execution payload bid is nil or invalid.
func (s signedExecutionPayloadBid) IsNil() bool {
	if s.bid == nil || !s.bid.ProtoReflect().IsValid() {
		return true
	}

	if _, err := s.Bid(); err != nil {
		return true
	}

	return len(s.bid.GetSignature()) != fieldparams.BLSSignatureLength
}

// IsNil checks if the execution payload bid is nil or has invalid fields.
func (h executionPayloadBid) IsNil() bool {
	if h.payload == nil || !h.payload.ProtoReflect().IsValid() {
		return true
	}

	if len(h.payload.GetParentBlockHash()) != 32 ||
		len(h.payload.GetParentBlockRoot()) != 32 ||
		len(h.payload.GetBlockHash()) != 32 ||
		len(h.payload.GetPrevRandao()) != 32 ||
		len(h.payload.GetFeeRecipient()) != 20 {
		return true
	}

	for _, commitment := range h.payload.GetBlobKzgCommitments() {
		if len(commitment) != 48 {
			return true
		}
	}

	return false
}

// WrappedROSignedExecutionPayloadBid creates a new read-only signed execution payload bid
// wrapper from the given Gloas protobuf message.
func WrappedROSignedExecutionPayloadBid(pb *ethpb.SignedExecutionPayloadBid) (interfaces.ROSignedExecutionPayloadBid, error) {
	if pb == nil {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return newSignedExecutionPayloadBid(pb, pb.Message)
}

// WrappedROSignedExecutionPayloadBidHeze creates a new read-only signed execution payload bid
// wrapper from the given Heze protobuf message.
func WrappedROSignedExecutionPayloadBidHeze(pb *ethpb.SignedExecutionPayloadBidHeze) (interfaces.ROSignedExecutionPayloadBid, error) {
	if pb == nil {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return newSignedExecutionPayloadBid(pb, pb.Message)
}

func newSignedExecutionPayloadBid(bid signedBidMessage, message bidMessage) (interfaces.ROSignedExecutionPayloadBid, error) {
	wrapper := signedExecutionPayloadBid{bid: bid, message: message}
	if wrapper.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return wrapper, nil
}

// WrappedROExecutionPayloadBid creates a new read-only execution payload bid
// wrapper for the Gloas fork from the given protobuf message.
func WrappedROExecutionPayloadBid(pb *ethpb.ExecutionPayloadBid) (interfaces.ROExecutionPayloadBid, error) {
	if pb == nil {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return newExecutionPayloadBid(pb, nil)
}

// WrappedROExecutionPayloadBidHeze creates a new read-only execution payload bid
// wrapper for the Heze fork from the given protobuf message.
func WrappedROExecutionPayloadBidHeze(pb *ethpb.ExecutionPayloadBidHeze) (interfaces.ROExecutionPayloadBid, error) {
	if pb == nil {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	if len(pb.InclusionListBits) != (fieldparams.InclusionListCommitteeSize+7)/8 {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return newExecutionPayloadBid(pb, pb.InclusionListBits)
}

func newExecutionPayloadBid(pb bidMessage, inclusionListBits bitfield.Bitvector16) (interfaces.ROExecutionPayloadBid, error) {
	wrapper := executionPayloadBid{payload: pb, inclusionListBits: inclusionListBits}
	if wrapper.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return wrapper, nil
}

// Bid returns the execution payload bid as a read-only interface.
func (s signedExecutionPayloadBid) Bid() (interfaces.ROExecutionPayloadBid, error) {
	switch m := s.message.(type) {
	case *ethpb.ExecutionPayloadBid:
		return WrappedROExecutionPayloadBid(m)
	case *ethpb.ExecutionPayloadBidHeze:
		return WrappedROExecutionPayloadBidHeze(m)
	default:
		return nil, consensus_types.ErrNilObjectWrapped
	}
}

// SigningRoot computes the signing root for the execution payload bid with the given domain.
func (s signedExecutionPayloadBid) SigningRoot(domain []byte) ([32]byte, error) {
	return signing.ComputeSigningRoot(s.message, domain)
}

// Signature returns the BLS signature as a 96-byte array.
func (s signedExecutionPayloadBid) Signature() [96]byte {
	return [96]byte(s.bid.GetSignature())
}

// HashTreeRoot returns the hash tree root of the signed execution payload bid.
func (s signedExecutionPayloadBid) HashTreeRoot() ([32]byte, error) {
	return s.bid.HashTreeRoot()
}

// ParentBlockHash returns the hash of the parent execution block.
func (h executionPayloadBid) ParentBlockHash() [32]byte {
	return [32]byte(h.payload.GetParentBlockHash())
}

// ParentBlockRoot returns the beacon block root of the parent block.
func (h executionPayloadBid) ParentBlockRoot() [32]byte {
	return [32]byte(h.payload.GetParentBlockRoot())
}

// PrevRandao returns the previous randao value for the execution block.
func (h executionPayloadBid) PrevRandao() [32]byte {
	return [32]byte(h.payload.GetPrevRandao())
}

// BlockHash returns the hash of the execution block.
func (h executionPayloadBid) BlockHash() [32]byte {
	return [32]byte(h.payload.GetBlockHash())
}

// GasLimit returns the gas limit for the execution block.
func (h executionPayloadBid) GasLimit() uint64 {
	return h.payload.GetGasLimit()
}

// BuilderIndex returns the builder index of the builder who created this bid.
func (h executionPayloadBid) BuilderIndex() primitives.BuilderIndex {
	return h.payload.GetBuilderIndex()
}

// Slot returns the beacon chain slot for which this bid was created.
func (h executionPayloadBid) Slot() primitives.Slot {
	return h.payload.GetSlot()
}

// Value returns the payment value offered by the builder in Gwei.
func (h executionPayloadBid) Value() primitives.Gwei {
	return h.payload.GetValue()
}

// ExecutionPayment returns the execution payment offered by the builder.
func (h executionPayloadBid) ExecutionPayment() primitives.Gwei {
	return h.payload.GetExecutionPayment()
}

// BlobKzgCommitments returns the KZG commitments for blobs.
func (h executionPayloadBid) BlobKzgCommitments() [][]byte {
	return bytesutil.SafeCopy2dBytes(h.payload.GetBlobKzgCommitments())
}

// BlobKzgCommitmentCount returns the number of blob KZG commitments.
func (h executionPayloadBid) BlobKzgCommitmentCount() uint64 {
	return uint64(len(h.payload.GetBlobKzgCommitments()))
}

// FeeRecipient returns the execution address that will receive the builder payment.
func (h executionPayloadBid) FeeRecipient() [20]byte {
	return [20]byte(h.payload.GetFeeRecipient())
}

// ExecutionRequestsRoot returns the hash tree root of the execution requests.
func (h executionPayloadBid) ExecutionRequestsRoot() [32]byte {
	if len(h.payload.GetExecutionRequestsRoot()) < 32 {
		return [32]byte{}
	}
	return [32]byte(h.payload.GetExecutionRequestsRoot())
}

// InclusionListBits returns a copy of the inclusion list bits, or nil before Heze.
func (h executionPayloadBid) InclusionListBits() bitfield.Bitvector16 {
	return bytesutil.SafeCopyBytes(h.inclusionListBits)
}

// ExecutionPayloadBidHezeProto copies bid into a Heze bid protobuf with the given inclusion list bits.
func ExecutionPayloadBidHezeProto(bid interfaces.ROExecutionPayloadBid, inclusionListBits bitfield.Bitvector16) *ethpb.ExecutionPayloadBidHeze {
	parentBlockHash := bid.ParentBlockHash()
	parentBlockRoot := bid.ParentBlockRoot()
	blockHash := bid.BlockHash()
	prevRandao := bid.PrevRandao()
	feeRecipient := bid.FeeRecipient()
	executionRequestsRoot := bid.ExecutionRequestsRoot()
	return &ethpb.ExecutionPayloadBidHeze{
		ParentBlockHash:       parentBlockHash[:],
		ParentBlockRoot:       parentBlockRoot[:],
		BlockHash:             blockHash[:],
		PrevRandao:            prevRandao[:],
		GasLimit:              bid.GasLimit(),
		BuilderIndex:          bid.BuilderIndex(),
		Slot:                  bid.Slot(),
		Value:                 bid.Value(),
		ExecutionPayment:      bid.ExecutionPayment(),
		BlobKzgCommitments:    bid.BlobKzgCommitments(),
		FeeRecipient:          feeRecipient[:],
		ExecutionRequestsRoot: executionRequestsRoot[:],
		InclusionListBits:     inclusionListBits,
	}
}
