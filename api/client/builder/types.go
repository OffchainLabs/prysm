package builder

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	types "github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/math"
	v1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

var errInvalidUint256 = errors.New("invalid Uint256")
var errDecodeUint256 = errors.New("unable to decode into Uint256")

// Uint256 a wrapper representation of big.Int
type Uint256 struct {
	*big.Int
}

func stringToUint256(s string) (Uint256, error) {
	bi := new(big.Int)
	_, ok := bi.SetString(s, 10)
	if !ok || !math.IsValidUint256(bi) {
		return Uint256{}, errors.Wrapf(errDecodeUint256, "value=%s", s)
	}
	return Uint256{Int: bi}, nil
}

// sszBytesToUint256 creates a Uint256 from a ssz-style (little-endian byte slice) representation.
func sszBytesToUint256(b []byte) (Uint256, error) {
	bi := bytesutil.LittleEndianBytesToBigInt(b)
	if !math.IsValidUint256(bi) {
		return Uint256{}, errors.Wrapf(errDecodeUint256, "value=%s", b)
	}
	return Uint256{Int: bi}, nil
}

// SSZBytes creates an ssz-style (little-endian byte slice) representation of the Uint256.
func (s Uint256) SSZBytes() []byte {
	if s.Int == nil {
		s.Int = big.NewInt(0)
	}
	if !math.IsValidUint256(s.Int) {
		return []byte{}
	}
	return bytesutil.PadTo(bytesutil.ReverseByteOrder(s.Int.Bytes()), 32)
}

// UnmarshalJSON takes in a byte array and unmarshals the value in Uint256
func (s *Uint256) UnmarshalJSON(t []byte) error {
	end := len(t)
	if len(t) < 2 {
		return errors.Errorf("provided Uint256 json string is too short: %s", string(t))
	}
	if t[0] != '"' || t[end-1] != '"' {
		return errors.Errorf("provided Uint256 json string is malformed: %s", string(t))
	}
	return s.UnmarshalText(t[1 : end-1])
}

// UnmarshalText takes in a byte array and unmarshals the text in Uint256
func (s *Uint256) UnmarshalText(t []byte) error {
	if s.Int == nil {
		s.Int = big.NewInt(0)
	}
	z, ok := s.SetString(string(t), 10)
	if !ok {
		return errors.Wrapf(errDecodeUint256, "value=%s", t)
	}
	if !math.IsValidUint256(z) {
		return errors.Wrapf(errDecodeUint256, "value=%s", t)
	}
	s.Int = z
	return nil
}

// MarshalJSON returns a json byte representation of Uint256.
func (s Uint256) MarshalJSON() ([]byte, error) {
	t, err := s.MarshalText()
	if err != nil {
		return nil, err
	}
	t = append([]byte{'"'}, t...)
	t = append(t, '"')
	return t, nil
}

// MarshalText returns a text byte representation of Uint256.
func (s Uint256) MarshalText() ([]byte, error) {
	if s.Int == nil {
		s.Int = big.NewInt(0)
	}
	if !math.IsValidUint256(s.Int) {
		return nil, errors.Wrapf(errInvalidUint256, "value=%s", s.Int)
	}
	return []byte(s.String()), nil
}

// Uint64String is a custom type that allows marshalling from text to uint64 and vice versa.
type Uint64String uint64

// UnmarshalText takes a byte array and unmarshals the text in Uint64String.
func (s *Uint64String) UnmarshalText(t []byte) error {
	u, err := strconv.ParseUint(string(t), 10, 64)
	*s = Uint64String(u)
	return err
}

// MarshalText returns a byte representation of the text from Uint64String.
func (s Uint64String) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("%d", s)), nil
}

// VersionResponse is a JSON representation of a field in the builder API header response.
type VersionResponse struct {
	Version string `json:"version"`
}

// ExecHeaderResponse is a JSON representation of the builder API header response for Bellatrix.
type ExecHeaderResponse struct {
	Version string `json:"version"`
	Data    struct {
		Signature hexutil.Bytes `json:"signature"`
		Message   *BuilderBid   `json:"message"`
	} `json:"data"`
}

// ToProto returns a SignedBuilderBid from ExecHeaderResponse for Bellatrix.
func (ehr *ExecHeaderResponse) ToProto() (*eth.SignedBuilderBid, error) {
	bb, err := ehr.Data.Message.ToProto()
	if err != nil {
		return nil, err
	}
	return &eth.SignedBuilderBid{
		Message:   bb,
		Signature: ehr.Data.Signature,
	}, nil
}

// ToProto returns a BuilderBid Proto for Bellatrix.
func (bb *BuilderBid) ToProto() (*eth.BuilderBid, error) {
	header, err := bb.Header.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.BuilderBid{
		Header: header,
		// Note that SSZBytes() reverses byte order for the little-endian representation.
		// Uint256.Bytes() is big-endian, SSZBytes takes this value and reverses it.
		Value:  bb.Value.SSZBytes(),
		Pubkey: bb.Pubkey,
	}, nil
}

// BuilderBid is part of ExecHeaderResponse for Bellatrix.
type BuilderBid struct {
	Header *structs.ExecutionPayloadHeader `json:"header"`
	Value  Uint256                         `json:"value"`
	Pubkey hexutil.Bytes                   `json:"pubkey"`
}

// ExecHeaderResponseCapella is the response of builder API /eth/v1/builder/header/{slot}/{parent_hash}/{pubkey} for Capella.
type ExecHeaderResponseCapella struct {
	Version string `json:"version"`
	Data    struct {
		Signature hexutil.Bytes      `json:"signature"`
		Message   *BuilderBidCapella `json:"message"`
	} `json:"data"`
}

// ToProto returns a SignedBuilderBidCapella Proto from ExecHeaderResponseCapella.
func (ehr *ExecHeaderResponseCapella) ToProto() (*eth.SignedBuilderBidCapella, error) {
	bb, err := ehr.Data.Message.ToProto()
	if err != nil {
		return nil, err
	}
	return &eth.SignedBuilderBidCapella{
		Message:   bb,
		Signature: bytesutil.SafeCopyBytes(ehr.Data.Signature),
	}, nil
}

// ToProto returns a BuilderBidCapella Proto.
func (bb *BuilderBidCapella) ToProto() (*eth.BuilderBidCapella, error) {
	header, err := bb.Header.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.BuilderBidCapella{
		Header: header,
		// Note that SSZBytes() reverses byte order for the little-endian representation.
		// Uint256.Bytes() is big-endian, SSZBytes takes this value and reverses it.
		Value:  bytesutil.SafeCopyBytes(bb.Value.SSZBytes()),
		Pubkey: bytesutil.SafeCopyBytes(bb.Pubkey),
	}, nil
}

// BuilderBidCapella is field of ExecHeaderResponseCapella.
type BuilderBidCapella struct {
	Header *structs.ExecutionPayloadHeaderCapella `json:"header"`
	Value  Uint256                                `json:"value"`
	Pubkey hexutil.Bytes                          `json:"pubkey"`
}

// ExecPayloadResponseCapella is the builder API /eth/v1/builder/blinded_blocks for Capella.
type ExecPayloadResponseCapella struct {
	Version string                          `json:"version"`
	Data    structs.ExecutionPayloadCapella `json:"data"`
}

// ExecutionPayloadResponse allows for unmarshaling just the Version field of the payload.
// This allows it to return different ExecutionPayload types based on the version field.
type ExecutionPayloadResponse struct {
	Version string          `json:"version"`
	Data    json.RawMessage `json:"data"`
}

// ParsedPayload can retrieve the underlying protobuf message for the given execution payload response.
type ParsedPayload interface {
	PayloadProto() (proto.Message, error)
}

// BlobBundler can retrieve the underlying blob bundle protobuf message for the given execution payload response.
type BlobBundler interface {
	BundleProto() (*v1.BlobsBundle, error)
}

// ParsedExecutionRequests can retrieve the underlying execution requests for the given execution payload response.
type ParsedExecutionRequests interface {
	ExecutionRequestsProto() (*v1.ExecutionRequests, error)
}

func (r *ExecutionPayloadResponse) ParsePayload() (ParsedPayload, error) {
	var toProto ParsedPayload
	v, err := version.FromString(strings.ToLower(r.Version))
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("unsupported version %s", strings.ToLower(r.Version)))
	}
	if v >= version.Deneb {
		toProto = &ExecutionPayloadDenebAndBlobsBundle{}
	} else if v >= version.Capella {
		toProto = &structs.ExecutionPayloadCapella{}
	} else if v >= version.Bellatrix {
		toProto = &structs.ExecutionPayload{}
	} else {
		return nil, fmt.Errorf("unsupported version %s", strings.ToLower(r.Version))
	}
	if len(r.Data) == 0 {
		return nil, errors.Wrap(consensusblocks.ErrNilObject, "empty payload data response")
	}
	if err := json.Unmarshal(r.Data, toProto); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal the response .Data field with the stated version schema")
	}
	return toProto, nil
}

// ToProto returns a ExecutionPayloadCapella Proto.
func (r *ExecPayloadResponseCapella) ToProto() (*v1.ExecutionPayloadCapella, error) {
	return r.Data.ToConsensus()
}

// Withdrawal is a field of ExecutionPayloadCapella.
type Withdrawal struct {
	Index          Uint256       `json:"index"`
	ValidatorIndex Uint256       `json:"validator_index"`
	Address        hexutil.Bytes `json:"address"`
	Amount         Uint256       `json:"amount"`
}

// SignedBlindedBeaconBlockBellatrix is the request object for builder API /eth/v1/builder/blinded_blocks.
type SignedBlindedBeaconBlockBellatrix struct {
	*eth.SignedBlindedBeaconBlockBellatrix
}

// ProposerSlashing is a field in BlindedBeaconBlockBodyCapella.
type ProposerSlashing struct {
	*eth.ProposerSlashing
}

// MarshalJSON returns a JSON byte array representation of ProposerSlashing.
func (s *ProposerSlashing) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		SignedHeader1 *SignedBeaconBlockHeader `json:"signed_header_1"`
		SignedHeader2 *SignedBeaconBlockHeader `json:"signed_header_2"`
	}{
		SignedHeader1: &SignedBeaconBlockHeader{s.ProposerSlashing.Header_1},
		SignedHeader2: &SignedBeaconBlockHeader{s.ProposerSlashing.Header_2},
	})
}

// SignedBeaconBlockHeader is a field of ProposerSlashing.
type SignedBeaconBlockHeader struct {
	*eth.SignedBeaconBlockHeader
}

// MarshalJSON returns a JSON byte array representation of SignedBeaconBlockHeader.
func (h *SignedBeaconBlockHeader) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Header    *BeaconBlockHeader `json:"message"`
		Signature hexutil.Bytes      `json:"signature"`
	}{
		Header:    &BeaconBlockHeader{h.SignedBeaconBlockHeader.Header},
		Signature: h.SignedBeaconBlockHeader.Signature,
	})
}

// BeaconBlockHeader is a field of SignedBeaconBlockHeader.
type BeaconBlockHeader struct {
	*eth.BeaconBlockHeader
}

// MarshalJSON returns a JSON byte array representation of BeaconBlockHeader.
func (h *BeaconBlockHeader) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Slot          string        `json:"slot"`
		ProposerIndex string        `json:"proposer_index"`
		ParentRoot    hexutil.Bytes `json:"parent_root"`
		StateRoot     hexutil.Bytes `json:"state_root"`
		BodyRoot      hexutil.Bytes `json:"body_root"`
	}{
		Slot:          fmt.Sprintf("%d", h.BeaconBlockHeader.Slot),
		ProposerIndex: fmt.Sprintf("%d", h.BeaconBlockHeader.ProposerIndex),
		ParentRoot:    h.BeaconBlockHeader.ParentRoot,
		StateRoot:     h.BeaconBlockHeader.StateRoot,
		BodyRoot:      h.BeaconBlockHeader.BodyRoot,
	})
}

// ExecHeaderResponseDeneb is the header response for builder API /eth/v1/builder/header/{slot}/{parent_hash}/{pubkey}.
type ExecHeaderResponseDeneb struct {
	Version string `json:"version"`
	Data    struct {
		Signature hexutil.Bytes    `json:"signature"`
		Message   *BuilderBidDeneb `json:"message"`
	} `json:"data"`
}

// ToProto creates a SignedBuilderBidDeneb Proto from ExecHeaderResponseDeneb.
func (ehr *ExecHeaderResponseDeneb) ToProto() (*eth.SignedBuilderBidDeneb, error) {
	bb, err := ehr.Data.Message.ToProto()
	if err != nil {
		return nil, err
	}
	return &eth.SignedBuilderBidDeneb{
		Message:   bb,
		Signature: bytesutil.SafeCopyBytes(ehr.Data.Signature),
	}, nil
}

// ToProto creates a BuilderBidDeneb Proto from BuilderBidDeneb.
func (bb *BuilderBidDeneb) ToProto() (*eth.BuilderBidDeneb, error) {
	header, err := bb.Header.ToConsensus()
	if err != nil {
		return nil, err
	}
	if len(bb.BlobKzgCommitments) > params.BeaconConfig().DeprecatedMaxBlobsPerBlock {
		return nil, fmt.Errorf("too many blob commitments: %d", len(bb.BlobKzgCommitments))
	}
	kzgCommitments := make([][]byte, len(bb.BlobKzgCommitments))
	for i, commit := range bb.BlobKzgCommitments {
		if len(commit) != fieldparams.BLSPubkeyLength {
			return nil, fmt.Errorf("commitment length %d is not %d", len(commit), fieldparams.BLSPubkeyLength)
		}
		kzgCommitments[i] = bytesutil.SafeCopyBytes(commit)
	}
	return &eth.BuilderBidDeneb{
		Header:             header,
		BlobKzgCommitments: kzgCommitments,
		// Note that SSZBytes() reverses byte order for the little-endian representation.
		// Uint256.Bytes() is big-endian, SSZBytes takes this value and reverses it.
		Value:  bytesutil.SafeCopyBytes(bb.Value.SSZBytes()),
		Pubkey: bytesutil.SafeCopyBytes(bb.Pubkey),
	}, nil
}

// BuilderBidDeneb is a field of ExecHeaderResponseDeneb.
type BuilderBidDeneb struct {
	Header             *structs.ExecutionPayloadHeaderDeneb `json:"header"`
	BlobKzgCommitments []hexutil.Bytes                      `json:"blob_kzg_commitments"`
	Value              Uint256                              `json:"value"`
	Pubkey             hexutil.Bytes                        `json:"pubkey"`
}

// ExecPayloadResponseDeneb the response to the build API /eth/v1/builder/blinded_blocks that includes the version, execution payload object , and blobs bundle object.
type ExecPayloadResponseDeneb struct {
	Version string                               `json:"version"`
	Data    *ExecutionPayloadDenebAndBlobsBundle `json:"data"`
}

// ExecutionPayloadDenebAndBlobsBundle the main field used in ExecPayloadResponseDeneb.
type ExecutionPayloadDenebAndBlobsBundle struct {
	ExecutionPayload *structs.ExecutionPayloadDeneb `json:"execution_payload"`
	BlobsBundle      *BlobsBundle                   `json:"blobs_bundle"`
}

// BlobsBundle is a field in ExecutionPayloadDenebAndBlobsBundle.
type BlobsBundle struct {
	Commitments []hexutil.Bytes `json:"commitments"`
	Proofs      []hexutil.Bytes `json:"proofs"`
	Blobs       []hexutil.Bytes `json:"blobs"`
}

// ToProto returns a BlobsBundle Proto.
func (b BlobsBundle) ToProto() (*v1.BlobsBundle, error) {
	if len(b.Blobs) > fieldparams.MaxBlobCommitmentsPerBlock {
		return nil, fmt.Errorf("blobs length %d is more than max %d", len(b.Blobs), fieldparams.MaxBlobCommitmentsPerBlock)
	}
	if len(b.Commitments) != len(b.Blobs) {
		return nil, fmt.Errorf("commitments length %d does not equal blobs length %d", len(b.Commitments), len(b.Blobs))
	}
	if len(b.Proofs) != len(b.Blobs) {
		return nil, fmt.Errorf("proofs length %d does not equal blobs length %d", len(b.Proofs), len(b.Blobs))
	}

	commitments := make([][]byte, len(b.Commitments))
	for i := range b.Commitments {
		if len(b.Commitments[i]) != fieldparams.BLSPubkeyLength {
			return nil, fmt.Errorf("commitment length %d is not %d", len(b.Commitments[i]), fieldparams.BLSPubkeyLength)
		}
		commitments[i] = bytesutil.SafeCopyBytes(b.Commitments[i])
	}
	proofs := make([][]byte, len(b.Proofs))
	for i := range b.Proofs {
		if len(b.Proofs[i]) != fieldparams.BLSPubkeyLength {
			return nil, fmt.Errorf("proof length %d is not %d", len(b.Proofs[i]), fieldparams.BLSPubkeyLength)
		}
		proofs[i] = bytesutil.SafeCopyBytes(b.Proofs[i])
	}
	blobs := make([][]byte, len(b.Blobs))
	for i := range b.Blobs {
		if len(b.Blobs[i]) != fieldparams.BlobLength {
			return nil, fmt.Errorf("blob length %d is not %d", len(b.Blobs[i]), fieldparams.BlobLength)
		}
		blobs[i] = bytesutil.SafeCopyBytes(b.Blobs[i])
	}
	return &v1.BlobsBundle{
		KzgCommitments: commitments,
		Proofs:         proofs,
		Blobs:          blobs,
	}, nil
}

// FromBundleProto converts the proto bundle type to the builder
// type.
func FromBundleProto(bundle *v1.BlobsBundle) *BlobsBundle {
	commitments := make([]hexutil.Bytes, len(bundle.KzgCommitments))
	for i := range bundle.KzgCommitments {
		commitments[i] = bytesutil.SafeCopyBytes(bundle.KzgCommitments[i])
	}
	proofs := make([]hexutil.Bytes, len(bundle.Proofs))
	for i := range bundle.Proofs {
		proofs[i] = bytesutil.SafeCopyBytes(bundle.Proofs[i])
	}
	blobs := make([]hexutil.Bytes, len(bundle.Blobs))
	for i := range bundle.Blobs {
		blobs[i] = bytesutil.SafeCopyBytes(bundle.Blobs[i])
	}
	return &BlobsBundle{
		Commitments: commitments,
		Proofs:      proofs,
		Blobs:       blobs,
	}
}

// ToProto returns ExecutionPayloadDeneb Proto and BlobsBundle Proto separately.
func (r *ExecPayloadResponseDeneb) ToProto() (*v1.ExecutionPayloadDeneb, *v1.BlobsBundle, error) {
	if r.Data == nil {
		return nil, nil, errors.New("data field in response is empty")
	}
	if r.Data.ExecutionPayload == nil {
		return nil, nil, errors.Wrap(consensusblocks.ErrNilObject, "nil execution payload")
	}
	if r.Data.BlobsBundle == nil {
		return nil, nil, errors.Wrap(consensusblocks.ErrNilObject, "nil blobs bundle")
	}
	payload, err := r.Data.ExecutionPayload.ToConsensus()
	if err != nil {
		return nil, nil, err
	}
	bundle, err := r.Data.BlobsBundle.ToProto()
	if err != nil {
		return nil, nil, err
	}
	return payload, bundle, nil
}

func (r *ExecutionPayloadDenebAndBlobsBundle) PayloadProto() (proto.Message, error) {
	if r.ExecutionPayload == nil {
		return nil, errors.Wrap(consensusblocks.ErrNilObject, "nil execution payload in combined deneb payload")
	}
	pb, err := r.ExecutionPayload.ToConsensus()
	return pb, err
}

func (r *ExecutionPayloadDenebAndBlobsBundle) BundleProto() (*v1.BlobsBundle, error) {
	if r.BlobsBundle == nil {
		return nil, errors.Wrap(consensusblocks.ErrNilObject, "nil blobs bundle")
	}
	return r.BlobsBundle.ToProto()
}

// ExecHeaderResponseElectra is the header response for builder API /eth/v1/builder/header/{slot}/{parent_hash}/{pubkey}.
type ExecHeaderResponseElectra struct {
	Version string `json:"version"`
	Data    struct {
		Signature hexutil.Bytes      `json:"signature"`
		Message   *BuilderBidElectra `json:"message"`
	} `json:"data"`
}

// ToProto creates a SignedBuilderBidElectra Proto from ExecHeaderResponseElectra.
func (ehr *ExecHeaderResponseElectra) ToProto(slot types.Slot) (*eth.SignedBuilderBidElectra, error) {
	bb, err := ehr.Data.Message.ToProto(slot)
	if err != nil {
		return nil, err
	}
	return &eth.SignedBuilderBidElectra{
		Message:   bb,
		Signature: bytesutil.SafeCopyBytes(ehr.Data.Signature),
	}, nil
}

// ToProto creates a BuilderBidElectra Proto from BuilderBidElectra.
func (bb *BuilderBidElectra) ToProto(slot types.Slot) (*eth.BuilderBidElectra, error) {
	header, err := bb.Header.ToConsensus()
	if err != nil {
		return nil, err
	}
	maxBlobsPerBlock := params.BeaconConfig().MaxBlobsPerBlock(slot)
	if len(bb.BlobKzgCommitments) > maxBlobsPerBlock {
		return nil, fmt.Errorf("blob commitment count %d exceeds the maximum %d", len(bb.BlobKzgCommitments), maxBlobsPerBlock)
	}
	kzgCommitments := make([][]byte, len(bb.BlobKzgCommitments))
	for i, commit := range bb.BlobKzgCommitments {
		if len(commit) != fieldparams.BLSPubkeyLength {
			return nil, fmt.Errorf("commitment length %d is not %d", len(commit), fieldparams.BLSPubkeyLength)
		}
		kzgCommitments[i] = bytesutil.SafeCopyBytes(commit)
	}
	// post electra execution requests should not be nil, if no requests exist use an empty request
	if bb.ExecutionRequests == nil {
		return nil, errors.New("bid contains nil execution requests")
	}
	executionRequests, err := bb.ExecutionRequests.ToConsensus()
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert ExecutionRequests")
	}
	return &eth.BuilderBidElectra{
		Header:             header,
		BlobKzgCommitments: kzgCommitments,
		ExecutionRequests:  executionRequests,
		// Note that SSZBytes() reverses byte order for the little-endian representation.
		// Uint256.Bytes() is big-endian, SSZBytes takes this value and reverses it.
		Value:  bytesutil.SafeCopyBytes(bb.Value.SSZBytes()),
		Pubkey: bytesutil.SafeCopyBytes(bb.Pubkey),
	}, nil
}

// BuilderBidElectra is a field of ExecHeaderResponseElectra.
type BuilderBidElectra struct {
	Header             *structs.ExecutionPayloadHeaderDeneb `json:"header"`
	BlobKzgCommitments []hexutil.Bytes                      `json:"blob_kzg_commitments"`
	ExecutionRequests  *structs.ExecutionRequests           `json:"execution_requests"`
	Value              Uint256                              `json:"value"`
	Pubkey             hexutil.Bytes                        `json:"pubkey"`
}

// ErrorMessage is a JSON representation of the builder API's returned error message.
type ErrorMessage struct {
	Code        int      `json:"code"`
	Message     string   `json:"message"`
	Stacktraces []string `json:"stacktraces,omitempty"`
}
