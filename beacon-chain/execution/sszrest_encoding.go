package execution

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
)

// EIP-8161 PayloadStatus SSZ status byte values.
const (
	sszPayloadStatusValid            uint8 = 0
	sszPayloadStatusInvalid          uint8 = 1
	sszPayloadStatusSyncing          uint8 = 2
	sszPayloadStatusAccepted         uint8 = 3
	sszPayloadStatusInvalidBlockHash uint8 = 4
)

// sszMarshaler is satisfied by proto types that can marshal to SSZ.
type sszMarshaler interface {
	MarshalSSZ() ([]byte, error)
}

// --- New Payload Request ---

// marshalNewPayloadRequest creates the SSZ-encoded body for a new_payload request.
// Uses sszgen-generated methods for the inner types and fastssz helpers for
// composing the container.
func marshalNewPayloadRequest(
	payload interfaces.ExecutionData,
	versionedHashes []common.Hash,
	parentBlockRoot *common.Hash,
	executionRequests *pb.ExecutionRequests,
) ([]byte, error) {
	payloadProto := payload.Proto()
	marshaler, ok := payloadProto.(sszMarshaler)
	if !ok {
		return nil, errors.New("execution payload does not support SSZ marshaling")
	}

	payloadSSZ, err := marshaler.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "marshal execution payload SSZ")
	}

	switch payloadProto.(type) {
	case *pb.ExecutionPayload:
		return payloadSSZ, nil
	case *pb.ExecutionPayloadCapella:
		return payloadSSZ, nil
	case *pb.ExecutionPayloadDeneb:
		if executionRequests == nil {
			// V3: {payload (var), versioned_hashes (var), parent_beacon_block_root (fixed 32)}
			return marshalNewPayloadV3Container(payloadSSZ, versionedHashes, parentBlockRoot), nil
		}
		// V4: {payload (var), versioned_hashes (var), parent_beacon_block_root (fixed 32), execution_requests (var)}
		requestsSSZ, err := executionRequests.MarshalSSZ()
		if err != nil {
			return nil, errors.Wrap(err, "marshal execution requests SSZ")
		}
		return marshalNewPayloadV4Container(payloadSSZ, versionedHashes, parentBlockRoot, requestsSSZ), nil
	case *pb.ExecutionPayloadGloas:
		if executionRequests == nil {
			executionRequests = &pb.ExecutionRequests{}
		}
		requestsSSZ, err := executionRequests.MarshalSSZ()
		if err != nil {
			return nil, errors.Wrap(err, "marshal execution requests SSZ")
		}
		return marshalNewPayloadV4Container(payloadSSZ, versionedHashes, parentBlockRoot, requestsSSZ), nil
	default:
		return nil, errors.New("unsupported execution payload type for SSZ-REST")
	}
}

func marshalNewPayloadV3Container(payloadSSZ []byte, versionedHashes []common.Hash, parentBlockRoot *common.Hash) []byte {
	// Fixed: offset(4) + offset(4) + root(32) = 40 bytes
	const fixedSize = 40
	hashesSize := len(versionedHashes) * 32
	buf := make([]byte, 0, fixedSize+len(payloadSSZ)+hashesSize)

	offset := fixedSize
	buf = ssz.WriteOffset(buf, offset)
	offset += len(payloadSSZ)
	buf = ssz.WriteOffset(buf, offset)
	buf = append(buf, parentBlockRoot[:]...)

	buf = append(buf, payloadSSZ...)
	for _, h := range versionedHashes {
		buf = append(buf, h[:]...)
	}
	return buf
}

func marshalNewPayloadV4Container(payloadSSZ []byte, versionedHashes []common.Hash, parentBlockRoot *common.Hash, requestsSSZ []byte) []byte {
	// Fixed: offset(4) + offset(4) + root(32) + offset(4) = 44 bytes
	const fixedSize = 44
	hashesSize := len(versionedHashes) * 32
	buf := make([]byte, 0, fixedSize+len(payloadSSZ)+hashesSize+len(requestsSSZ))

	offset := fixedSize
	buf = ssz.WriteOffset(buf, offset)
	offset += len(payloadSSZ)
	buf = ssz.WriteOffset(buf, offset)
	offset += hashesSize
	buf = append(buf, parentBlockRoot[:]...)
	buf = ssz.WriteOffset(buf, offset)

	buf = append(buf, payloadSSZ...)
	for _, h := range versionedHashes {
		buf = append(buf, h[:]...)
	}
	buf = append(buf, requestsSSZ...)
	return buf
}

// --- Payload Status (response) ---

// unmarshalPayloadStatusSSZ decodes an SSZ-encoded PayloadStatusV1SSZ response
// using sszgen-generated UnmarshalSSZ.
func unmarshalPayloadStatusSSZ(data []byte) (*pb.PayloadStatus, error) {
	wire := &pb.PayloadStatusV1SSZ{}
	if err := wire.UnmarshalSSZ(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal SSZ PayloadStatus")
	}
	return payloadStatusFromSSZ(wire)
}

func payloadStatusFromSSZ(wire *pb.PayloadStatusV1SSZ) (*pb.PayloadStatus, error) {
	status := &pb.PayloadStatus{}
	if len(wire.Status) != 1 {
		return nil, fmt.Errorf("invalid SSZ payload status length: %d", len(wire.Status))
	}
	switch wire.Status[0] {
	case sszPayloadStatusValid:
		status.Status = pb.PayloadStatus_VALID
	case sszPayloadStatusInvalid:
		status.Status = pb.PayloadStatus_INVALID
	case sszPayloadStatusSyncing:
		status.Status = pb.PayloadStatus_SYNCING
	case sszPayloadStatusAccepted:
		status.Status = pb.PayloadStatus_ACCEPTED
	case sszPayloadStatusInvalidBlockHash:
		status.Status = pb.PayloadStatus_INVALID_BLOCK_HASH
	default:
		return nil, fmt.Errorf("unknown SSZ payload status: %d", wire.Status[0])
	}
	if len(wire.LatestValidHash) == 1 {
		status.LatestValidHash = make([]byte, 32)
		copy(status.LatestValidHash, wire.LatestValidHash[0])
	}
	if len(wire.ValidationError) > 0 {
		status.ValidationError = string(wire.ValidationError)
	}
	return status, nil
}

// --- ForkchoiceUpdated Request ---

// marshalForkchoiceUpdatedRequest creates the SSZ-encoded body for a forkchoice_updated
// request using sszgen-generated MarshalSSZ.
func marshalForkchoiceUpdatedRequest(
	state *pb.ForkchoiceState,
	attrs payloadattribute.Attributer,
) ([]byte, error) {
	req := &pb.ForkchoiceUpdatedV3RequestSSZ{
		ForkchoiceState: state,
	}

	hasAttrs := attrs != nil && !attrs.IsEmpty()
	if hasAttrs {
		wireAttrs, err := payloadAttributesToSSZ(attrs)
		if err != nil {
			return nil, err
		}
		req.PayloadAttributes = []*pb.PayloadAttributesV3SSZ{wireAttrs}
	}

	return req.MarshalSSZ()
}

func payloadAttributesToSSZ(attrs payloadattribute.Attributer) (*pb.PayloadAttributesV3SSZ, error) {
	wire := &pb.PayloadAttributesV3SSZ{}

	switch attrs.Version() {
	case 1:
		a, err := attrs.PbV1()
		if err != nil {
			return nil, err
		}
		wire.Timestamp = a.Timestamp
		wire.PrevRandao = cloneBytes(a.PrevRandao)
		wire.SuggestedFeeRecipient = cloneBytes(a.SuggestedFeeRecipient)
	case 2:
		a, err := attrs.PbV2()
		if err != nil {
			return nil, err
		}
		wire.Timestamp = a.Timestamp
		wire.PrevRandao = cloneBytes(a.PrevRandao)
		wire.SuggestedFeeRecipient = cloneBytes(a.SuggestedFeeRecipient)
		wire.Withdrawals = withdrawalsToSSZ(a.Withdrawals)
	default: // V3 (Deneb/Electra/Fulu)
		a, err := attrs.PbV3()
		if err != nil {
			return nil, err
		}
		wire.Timestamp = a.Timestamp
		wire.PrevRandao = cloneBytes(a.PrevRandao)
		wire.SuggestedFeeRecipient = cloneBytes(a.SuggestedFeeRecipient)
		wire.Withdrawals = withdrawalsToSSZ(a.Withdrawals)
		wire.ParentBeaconBlockRoot = cloneBytes(a.ParentBeaconBlockRoot)
	}
	return wire, nil
}

func withdrawalsToSSZ(ws []*pb.Withdrawal) []*pb.WithdrawalSSZ {
	result := make([]*pb.WithdrawalSSZ, len(ws))
	for i, w := range ws {
		r := &pb.WithdrawalSSZ{
			Index:          w.Index,
			ValidatorIndex: uint64(w.ValidatorIndex),
			Amount:         w.Amount,
			Address:        cloneBytes(w.Address),
		}
		result[i] = r
	}
	return result
}

func cloneBytes(b []byte) []byte {
	return append([]byte(nil), b...)
}

// --- ForkchoiceUpdated Response ---

// forkchoiceUpdatedResponseSSZ holds parsed forkchoice updated response data.
type forkchoiceUpdatedResponseSSZ struct {
	Status    *pb.PayloadStatus
	PayloadId *pb.PayloadIDBytes
}

// unmarshalForkchoiceUpdatedResponseSSZ decodes an SSZ-encoded ForkchoiceUpdatedResponse
// using sszgen-generated UnmarshalSSZ.
func unmarshalForkchoiceUpdatedResponseSSZ(data []byte) (*forkchoiceUpdatedResponseSSZ, error) {
	wire := &pb.ForkchoiceUpdatedResponseSSZ{}
	if err := wire.UnmarshalSSZ(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal SSZ ForkchoiceUpdatedResponse")
	}

	status, err := payloadStatusFromSSZ(wire.PayloadStatus)
	if err != nil {
		return nil, err
	}

	resp := &forkchoiceUpdatedResponseSSZ{Status: status}
	if len(wire.PayloadId) == 1 {
		var pid pb.PayloadIDBytes
		copy(pid[:], wire.PayloadId[0])
		resp.PayloadId = &pid
	}
	return resp, nil
}

// --- GetPayload Response ---

func unmarshalGetPayloadResponseSSZ(data []byte, version int) (*blocks.GetPayloadResponse, error) {
	switch version {
	case 1:
		payload := &pb.ExecutionPayload{}
		if err := payload.UnmarshalSSZ(data); err != nil {
			return nil, errors.Wrap(err, "unmarshal execution payload SSZ")
		}
		ed, err := blocks.WrappedExecutionPayload(payload)
		if err != nil {
			return nil, err
		}
		return &blocks.GetPayloadResponse{
			ExecutionData: ed,
			Bid:           primitives.ZeroWei(),
		}, nil
	case 2:
		return unmarshalGetPayloadV2ResponseSSZ(data)
	case 3:
		return unmarshalGetPayloadV3ResponseSSZ(data)
	case 4:
		return unmarshalGetPayloadV4ResponseSSZ(data)
	case 5:
		return unmarshalGetPayloadV5ResponseSSZ(data)
	case 6:
		return unmarshalGetPayloadV6ResponseSSZ(data)
	default:
		return nil, fmt.Errorf("unsupported SSZ get_payload version: %d", version)
	}
}

func unmarshalGetPayloadV2ResponseSSZ(data []byte) (*blocks.GetPayloadResponse, error) {
	wire := &pb.GetPayloadV2ResponseSSZ{}
	if err := wire.UnmarshalSSZ(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal SSZ get_payload v2 response")
	}
	ed, err := blocks.WrappedExecutionPayloadCapella(wire.Payload)
	if err != nil {
		return nil, err
	}
	return &blocks.GetPayloadResponse{
		ExecutionData: ed,
		Bid:           blockValueToWei(wire.BlockValue),
	}, nil
}

func unmarshalGetPayloadV3ResponseSSZ(data []byte) (*blocks.GetPayloadResponse, error) {
	wire := &pb.GetPayloadV3ResponseSSZ{}
	if err := wire.UnmarshalSSZ(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal SSZ get_payload v3 response")
	}
	ed, err := blocks.WrappedExecutionPayloadDeneb(wire.Payload)
	if err != nil {
		return nil, err
	}
	return &blocks.GetPayloadResponse{
		ExecutionData:   ed,
		BlobsBundler:    wire.BlobsBundle,
		OverrideBuilder: wire.ShouldOverrideBuilder,
		Bid:             blockValueToWei(wire.BlockValue),
	}, nil
}

func unmarshalGetPayloadV4ResponseSSZ(data []byte) (*blocks.GetPayloadResponse, error) {
	wire := &pb.GetPayloadV4ResponseSSZ{}
	if err := wire.UnmarshalSSZ(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal SSZ get_payload v4 response")
	}
	ed, err := blocks.WrappedExecutionPayloadDeneb(wire.Payload)
	if err != nil {
		return nil, err
	}
	return &blocks.GetPayloadResponse{
		ExecutionData:     ed,
		BlobsBundler:      wire.BlobsBundle,
		OverrideBuilder:   wire.ShouldOverrideBuilder,
		Bid:               blockValueToWei(wire.BlockValue),
		ExecutionRequests: wire.ExecutionRequests,
	}, nil
}

func unmarshalGetPayloadV5ResponseSSZ(data []byte) (*blocks.GetPayloadResponse, error) {
	wire := &pb.GetPayloadV5ResponseSSZ{}
	if err := wire.UnmarshalSSZ(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal SSZ get_payload v5 response")
	}
	ed, err := blocks.WrappedExecutionPayloadDeneb(wire.Payload)
	if err != nil {
		return nil, err
	}
	return &blocks.GetPayloadResponse{
		ExecutionData:     ed,
		BlobsBundler:      wire.BlobsBundle,
		OverrideBuilder:   wire.ShouldOverrideBuilder,
		Bid:               blockValueToWei(wire.BlockValue),
		ExecutionRequests: wire.ExecutionRequests,
	}, nil
}

func unmarshalGetPayloadV6ResponseSSZ(data []byte) (*blocks.GetPayloadResponse, error) {
	wire := &pb.GetPayloadV6ResponseSSZ{}
	if err := wire.UnmarshalSSZ(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal SSZ get_payload v6 response")
	}
	ed, err := blocks.WrappedExecutionPayloadGloas(wire.Payload)
	if err != nil {
		return nil, err
	}
	return &blocks.GetPayloadResponse{
		ExecutionData:     ed,
		BlobsBundler:      wire.BlobsBundle,
		OverrideBuilder:   wire.ShouldOverrideBuilder,
		Bid:               blockValueToWei(wire.BlockValue),
		ExecutionRequests: wire.ExecutionRequests,
	}, nil
}

// --- GetBlobs ---

// blobAndProofSize is the fixed SSZ size of a BlobAndProof.
const blobAndProofSize = fieldparams.BlobLength + 48

// marshalGetBlobsRequest creates the SSZ-encoded body for a get_blobs request
// using sszgen-generated MarshalSSZ.
func marshalGetBlobsRequest(versionedHashes []common.Hash) []byte {
	req := &pb.GetBlobsRequestSSZ{
		VersionedHashes: make([][]byte, len(versionedHashes)),
	}
	for i, h := range versionedHashes {
		hash := make([]byte, 32)
		copy(hash, h[:])
		req.VersionedHashes[i] = hash
	}
	data, _ := req.MarshalSSZ()
	return data
}

// unmarshalGetBlobsResponseSSZ decodes an SSZ-encoded GetBlobsResponse.
// BlobAndProof is fixed-size (131120 bytes) so we decode manually for efficiency.
func unmarshalGetBlobsResponseSSZ(data []byte) ([]*pb.BlobAndProof, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("SSZ get_blobs response too short: %d bytes", len(data))
	}

	listOffset := ssz.ReadOffset(data[0:4])
	if listOffset > uint64(len(data)) {
		return nil, fmt.Errorf("SSZ get_blobs response truncated")
	}

	listData := data[listOffset:]
	if len(listData) == 0 {
		return []*pb.BlobAndProof{}, nil
	}
	if len(listData)%blobAndProofSize != 0 {
		return nil, fmt.Errorf("SSZ get_blobs data size %d not divisible by BlobAndProof size %d", len(listData), blobAndProofSize)
	}

	count := len(listData) / blobAndProofSize
	result := make([]*pb.BlobAndProof, count)
	for i := range count {
		offset := i * blobAndProofSize
		blob := make([]byte, fieldparams.BlobLength)
		copy(blob, listData[offset:offset+fieldparams.BlobLength])
		proof := make([]byte, 48)
		copy(proof, listData[offset+fieldparams.BlobLength:offset+blobAndProofSize])
		result[i] = &pb.BlobAndProof{
			Blob:     blob,
			KzgProof: proof,
		}
	}
	return result, nil
}

// --- ExchangeCapabilities ---

// marshalExchangeCapabilitiesRequest creates the SSZ-encoded body for an
// exchange_capabilities request using sszgen-generated MarshalSSZ.
func marshalExchangeCapabilitiesRequest(capabilities []string) []byte {
	req := &pb.ExchangeCapabilitiesSSZ{
		Capabilities: make([][]byte, len(capabilities)),
	}
	for i, c := range capabilities {
		req.Capabilities[i] = []byte(c)
	}
	data, _ := req.MarshalSSZ()
	return data
}

// unmarshalExchangeCapabilitiesResponse decodes an SSZ ExchangeCapabilitiesSSZ
// response using sszgen-generated UnmarshalSSZ.
func unmarshalExchangeCapabilitiesResponse(data []byte) ([]string, error) {
	wire := &pb.ExchangeCapabilitiesSSZ{}
	if err := wire.UnmarshalSSZ(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal SSZ ExchangeCapabilities")
	}
	result := make([]string, len(wire.Capabilities))
	for i, c := range wire.Capabilities {
		result[i] = string(c)
	}
	return result, nil
}

// --- ClientVersion ---

// marshalClientVersionRequest creates the SSZ-encoded body for a get_client_version
// request using sszgen-generated MarshalSSZ.
func marshalClientVersionRequest(code, name, ver, commit string) []byte {
	req := &pb.ClientVersionV1SSZ{
		Code:    []byte(code),
		Name:    []byte(name),
		Version: []byte(ver),
	}
	req.Commit = parseCommitToBytes4(commit)

	data, _ := req.MarshalSSZ()
	return data
}

// parseCommitToBytes4 converts a hex commit string to 4 bytes.
func parseCommitToBytes4(commit string) []byte {
	result := make([]byte, 4)
	b := common.FromHex(commit)
	if len(b) >= 4 {
		copy(result, b[:4])
	} else {
		copy(result, b)
	}
	return result
}

// unmarshalClientVersionResponse decodes an SSZ-encoded ClientVersionResponse
// using sszgen-generated UnmarshalSSZ.
func unmarshalClientVersionResponse(data []byte) ([]*structs.ClientVersionV1, error) {
	wire := &pb.ClientVersionResponseSSZ{}
	if err := wire.UnmarshalSSZ(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal SSZ ClientVersionResponse")
	}
	result := make([]*structs.ClientVersionV1, len(wire.Versions))
	for i, v := range wire.Versions {
		result[i] = &structs.ClientVersionV1{
			Code:    string(v.Code),
			Name:    string(v.Name),
			Version: string(v.Version),
			Commit:  fmt.Sprintf("%x", v.Commit),
		}
	}
	return result, nil
}

// blockValueToWei converts a 32-byte little-endian uint256 to primitives.Wei.
func blockValueToWei(blockValue []byte) primitives.Wei {
	return primitives.LittleEndianBytesToWei(blockValue)
}

// appendHash appends a 32-byte hash to buf, zero-padding if needed.
func appendHash(buf []byte, hash []byte) []byte {
	if len(hash) >= 32 {
		return append(buf, hash[:32]...)
	}
	padded := make([]byte, 32)
	copy(padded, hash)
	return append(buf, padded...)
}
