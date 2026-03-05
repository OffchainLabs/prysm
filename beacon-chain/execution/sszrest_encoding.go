package execution

import (
	"encoding/binary"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
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

// marshalForkchoiceStateSSZ manually encodes ForkchoiceState to SSZ.
// ForkchoiceState is a fixed-size container: 3 x Hash32 = 96 bytes.
func marshalForkchoiceStateSSZ(state *pb.ForkchoiceState) []byte {
	buf := make([]byte, 0, 96)
	buf = appendHash(buf, state.HeadBlockHash)
	buf = appendHash(buf, state.SafeBlockHash)
	buf = appendHash(buf, state.FinalizedBlockHash)
	return buf
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

// marshalNewPayloadRequest creates the SSZ-encoded body for a new_payload request.
// For v1/v2 payloads, the body is just the SSZ-encoded execution payload.
// For v3, it includes payload + versioned hashes + parent beacon block root.
// For v4, it adds execution requests.
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
			// V3: SSZ container = {payload (var), versioned_hashes (var), parent_beacon_block_root (fixed 32)}
			fixedSize := 4 + 4 + 32
			hashesSize := len(versionedHashes) * 32

			buf := make([]byte, 0, fixedSize+len(payloadSSZ)+hashesSize)

			// Offset for execution_payload (variable)
			offset := uint32(fixedSize)
			buf = binary.LittleEndian.AppendUint32(buf, offset)
			offset += uint32(len(payloadSSZ))

			// Offset for expected_blob_versioned_hashes (variable)
			buf = binary.LittleEndian.AppendUint32(buf, offset)

			// parent_beacon_block_root (fixed 32 bytes)
			buf = appendHash(buf, parentBlockRoot[:])

			// Variable data: payload
			buf = append(buf, payloadSSZ...)

			// Variable data: versioned hashes
			for _, h := range versionedHashes {
				buf = append(buf, h[:]...)
			}

			return buf, nil
		}

		// V4: SSZ container = {payload (var), versioned_hashes (var), parent_beacon_block_root (fixed 32), execution_requests (var)}
		requestsSSZ, err := executionRequests.MarshalSSZ()
		if err != nil {
			return nil, errors.Wrap(err, "marshal execution requests SSZ")
		}

		fixedSize := 4 + 4 + 32 + 4 // offsets + parent root
		hashesSize := len(versionedHashes) * 32

		buf := make([]byte, 0, fixedSize+len(payloadSSZ)+hashesSize+len(requestsSSZ))

		// Offset for execution_payload
		offset := uint32(fixedSize)
		buf = binary.LittleEndian.AppendUint32(buf, offset)
		offset += uint32(len(payloadSSZ))

		// Offset for expected_blob_versioned_hashes
		buf = binary.LittleEndian.AppendUint32(buf, offset)
		offset += uint32(hashesSize)

		// parent_beacon_block_root (fixed 32 bytes)
		buf = appendHash(buf, parentBlockRoot[:])

		// Offset for execution_requests
		buf = binary.LittleEndian.AppendUint32(buf, offset)

		// Variable data
		buf = append(buf, payloadSSZ...)
		for _, h := range versionedHashes {
			buf = append(buf, h[:]...)
		}
		buf = append(buf, requestsSSZ...)

		return buf, nil

	default:
		return nil, errors.New("unsupported execution payload type for SSZ-REST")
	}
}

// unmarshalPayloadStatusSSZ decodes an SSZ-encoded PayloadStatusSSZ response.
// Layout per EIP-8161:
//
//	status: uint8 (1 byte fixed)
//	latest_valid_hash: Union[None, Hash32] (variable, offset at bytes 1..5)
//	validation_error: List[uint8, 1024] (variable, offset at bytes 5..9)
func unmarshalPayloadStatusSSZ(data []byte) (*pb.PayloadStatus, error) {
	const fixedSize = 1 + 4 + 4 // status + 2 offsets
	if len(data) < fixedSize {
		return nil, fmt.Errorf("SSZ payload status too short: %d bytes", len(data))
	}

	status := &pb.PayloadStatus{}

	switch data[0] {
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
		return nil, fmt.Errorf("unknown SSZ payload status: %d", data[0])
	}

	hashOffset := binary.LittleEndian.Uint32(data[1:5])
	errorOffset := binary.LittleEndian.Uint32(data[5:9])

	if uint32(len(data)) < errorOffset {
		return nil, fmt.Errorf("SSZ payload status truncated")
	}

	// latest_valid_hash: Union[None, Hash32]
	if hashOffset < errorOffset {
		hashData := data[hashOffset:errorOffset]
		if len(hashData) > 0 && hashData[0] == 1 && len(hashData) >= 33 {
			status.LatestValidHash = make([]byte, 32)
			copy(status.LatestValidHash, hashData[1:33])
		}
	}

	// validation_error: List[uint8, 1024]
	if errorOffset < uint32(len(data)) {
		errorData := data[errorOffset:]
		if len(errorData) > 0 {
			status.ValidationError = string(errorData)
		}
	}

	return status, nil
}

// forkchoiceUpdatedResponseSSZ holds parsed forkchoice updated response data.
type forkchoiceUpdatedResponseSSZ struct {
	Status    *pb.PayloadStatus
	PayloadId *pb.PayloadIDBytes
}

// marshalForkchoiceUpdatedRequest creates the SSZ-encoded body for a forkchoice_updated request.
// Layout (SSZ container):
//
//	forkchoice_state: ForkchoiceState (96 bytes, fixed)
//	payload_attributes: Union[None, PayloadAttributes] (variable, offset at bytes 96..100)
func marshalForkchoiceUpdatedRequest(
	state *pb.ForkchoiceState,
	attrs payloadattribute.Attributer,
) ([]byte, error) {
	stateSSZ := marshalForkchoiceStateSSZ(state)

	hasAttrs := attrs != nil && !attrs.IsEmpty()

	// Fixed part = 96 (state) + 4 (offset for union) = 100 bytes
	const fixedSize = 100

	if !hasAttrs {
		buf := make([]byte, 0, fixedSize)
		buf = append(buf, stateSSZ...)
		// Offset points to end of data (no union data)
		buf = binary.LittleEndian.AppendUint32(buf, fixedSize)
		return buf, nil
	}

	// Serialize payload attributes manually since the proto types lack SSZ methods.
	attrsSSZ, err := marshalPayloadAttributesSSZ(attrs)
	if err != nil {
		return nil, err
	}

	// Union encoding: selector byte (1 = present) + data
	unionData := make([]byte, 0, 1+len(attrsSSZ))
	unionData = append(unionData, 1)
	unionData = append(unionData, attrsSSZ...)

	buf := make([]byte, 0, fixedSize+len(unionData))
	buf = append(buf, stateSSZ...)
	buf = binary.LittleEndian.AppendUint32(buf, fixedSize)
	buf = append(buf, unionData...)

	return buf, nil
}

// marshalPayloadAttributesSSZ manually encodes PayloadAttributes (V1/V2/V3) to SSZ.
// V1: timestamp (8) + prev_randao (32) + suggested_fee_recipient (20) = 60 bytes fixed
// V2: V1 + withdrawals (variable)
// V3: V2 + parent_beacon_block_root (32)
func marshalPayloadAttributesSSZ(attrs payloadattribute.Attributer) ([]byte, error) {
	switch attrs.Version() {
	case 1: // Bellatrix - all fixed-size fields
		a, err := attrs.PbV1()
		if err != nil {
			return nil, err
		}
		buf := make([]byte, 0, 60)
		buf = binary.LittleEndian.AppendUint64(buf, a.Timestamp)
		buf = appendHash(buf, a.PrevRandao)
		// fee recipient is 20 bytes
		if len(a.SuggestedFeeRecipient) >= 20 {
			buf = append(buf, a.SuggestedFeeRecipient[:20]...)
		} else {
			padded := make([]byte, 20)
			copy(padded, a.SuggestedFeeRecipient)
			buf = append(buf, padded...)
		}
		return buf, nil

	case 2: // Capella - adds withdrawals (variable)
		a, err := attrs.PbV2()
		if err != nil {
			return nil, err
		}
		// Fixed: timestamp(8) + prev_randao(32) + fee_recipient(20) + withdrawals_offset(4) = 64
		const fixedSize = 64
		buf := make([]byte, 0, fixedSize)
		buf = binary.LittleEndian.AppendUint64(buf, a.Timestamp)
		buf = appendHash(buf, a.PrevRandao)
		buf = appendFeeRecipient(buf, a.SuggestedFeeRecipient)
		// Offset for withdrawals
		buf = binary.LittleEndian.AppendUint32(buf, fixedSize)
		// Withdrawals: each is 44 bytes (index:8 + validator_index:8 + address:20 + amount:8)
		for _, w := range a.Withdrawals {
			buf = marshalWithdrawalSSZ(buf, w)
		}
		return buf, nil

	default: // Deneb/Electra/Fulu (V3) - adds parent_beacon_block_root
		a, err := attrs.PbV3()
		if err != nil {
			return nil, err
		}
		// Fixed: timestamp(8) + prev_randao(32) + fee_recipient(20) + withdrawals_offset(4) + parent_root(32) = 96
		const fixedSize = 96
		buf := make([]byte, 0, fixedSize)
		buf = binary.LittleEndian.AppendUint64(buf, a.Timestamp)
		buf = appendHash(buf, a.PrevRandao)
		buf = appendFeeRecipient(buf, a.SuggestedFeeRecipient)
		// Offset for withdrawals
		buf = binary.LittleEndian.AppendUint32(buf, fixedSize)
		// parent_beacon_block_root (fixed)
		buf = appendHash(buf, a.ParentBeaconBlockRoot)
		// Withdrawals data
		for _, w := range a.Withdrawals {
			buf = marshalWithdrawalSSZ(buf, w)
		}
		return buf, nil
	}
}

// appendFeeRecipient appends a 20-byte fee recipient to buf.
func appendFeeRecipient(buf, feeRecipient []byte) []byte {
	if len(feeRecipient) >= 20 {
		return append(buf, feeRecipient[:20]...)
	}
	padded := make([]byte, 20)
	copy(padded, feeRecipient)
	return append(buf, padded...)
}

// marshalWithdrawalSSZ appends a Withdrawal's SSZ encoding to buf.
// Withdrawal is fixed-size: index(8) + validator_index(8) + address(20) + amount(8) = 44 bytes
func marshalWithdrawalSSZ(buf []byte, w *pb.Withdrawal) []byte {
	buf = binary.LittleEndian.AppendUint64(buf, w.Index)
	buf = binary.LittleEndian.AppendUint64(buf, uint64(w.ValidatorIndex))
	buf = appendFeeRecipient(buf, w.Address) // 20 bytes
	buf = binary.LittleEndian.AppendUint64(buf, w.Amount)
	return buf
}

// unmarshalForkchoiceUpdatedResponseSSZ decodes an SSZ-encoded ForkchoiceUpdatedResponse.
// Layout (SSZ container):
//
//	payload_status: PayloadStatusSSZ (variable, offset at bytes 0..4)
//	payload_id: Union[None, uint64] (variable, offset at bytes 4..8)
func unmarshalForkchoiceUpdatedResponseSSZ(data []byte) (*forkchoiceUpdatedResponseSSZ, error) {
	log.WithField("len", len(data)).WithField("first20", fmt.Sprintf("%x", data[:min(20, len(data))])).Info("SSZ-REST: unmarshal FCU response raw bytes")
	if len(data) < 8 {
		return nil, fmt.Errorf("SSZ forkchoice updated response too short: %d bytes", len(data))
	}

	statusOffset := binary.LittleEndian.Uint32(data[0:4])
	payloadIdOffset := binary.LittleEndian.Uint32(data[4:8])
	log.WithField("statusOffset", statusOffset).WithField("payloadIdOffset", payloadIdOffset).WithField("dataLen", len(data)).Info("SSZ-REST: FCU response offsets")

	if uint32(len(data)) < statusOffset || uint32(len(data)) < payloadIdOffset {
		return nil, fmt.Errorf("SSZ forkchoice updated response truncated")
	}

	statusData := data[statusOffset:payloadIdOffset]
	status, err := unmarshalPayloadStatusSSZ(statusData)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal payload status from forkchoice response")
	}

	resp := &forkchoiceUpdatedResponseSSZ{Status: status}

	// payload_id: Union[None, uint64]
	if payloadIdOffset < uint32(len(data)) {
		pidData := data[payloadIdOffset:]
		log.WithField("pidDataLen", len(pidData)).WithField("pidFirst5", fmt.Sprintf("%x", pidData[:min(5, len(pidData))])).Info("SSZ-REST: FCU payload ID union data")
		if len(pidData) > 0 && pidData[0] == 1 && len(pidData) >= 9 {
			var pid pb.PayloadIDBytes
			copy(pid[:], pidData[1:9])
			resp.PayloadId = &pid
			log.WithField("payloadId", fmt.Sprintf("%x", pid[:])).Info("SSZ-REST: decoded payload ID")
		} else {
			log.WithField("selector", pidData[0]).Info("SSZ-REST: payload ID is None or invalid")
		}
	} else {
		log.Info("SSZ-REST: no payload ID data (offset at end)")
	}

	return resp, nil
}

// sszUnmarshaler is satisfied by proto types that can unmarshal from SSZ.
type sszUnmarshaler interface {
	UnmarshalSSZ(buf []byte) error
}

// getPayloadResponseSSZ holds parsed get_payload response data.
type getPayloadResponseSSZ struct {
	ExecutionPayloadSSZ []byte
	BlockValue          []byte // 32 bytes, uint256 LE
	BlobsBundleSSZ      []byte
	OverrideBuilder     bool
	ExecutionRequests   *pb.ExecutionRequests
}

// unmarshalGetPayloadResponseSSZ decodes an SSZ-encoded GetPayloadResponse.
// Layout (SSZ container):
//
//	execution_payload: ExecutionPayload (variable, offset at bytes 0..4)
//	block_value: uint256 (fixed 32 bytes, at bytes 4..36)
//	blobs_bundle: BlobsBundle (variable, offset at bytes 36..40)
//	should_override_builder: boolean (fixed 1 byte, at byte 40)
//	execution_requests: ExecutionRequests (variable, offset at bytes 41..45)
func unmarshalGetPayloadResponseSSZ(data []byte) (*getPayloadResponseSSZ, error) {
	const fixedSize = 4 + 32 + 4 + 1 + 4 // = 45
	if len(data) < fixedSize {
		return nil, fmt.Errorf("SSZ get_payload response too short: %d bytes, need at least %d", len(data), fixedSize)
	}

	payloadOffset := binary.LittleEndian.Uint32(data[0:4])
	blockValue := data[4:36]
	blobsOffset := binary.LittleEndian.Uint32(data[36:40])
	overrideBuilder := data[40] != 0
	requestsOffset := binary.LittleEndian.Uint32(data[41:45])

	dataLen := uint32(len(data))
	if payloadOffset > dataLen || blobsOffset > dataLen || requestsOffset > dataLen {
		return nil, fmt.Errorf("SSZ get_payload response truncated")
	}
	if payloadOffset > blobsOffset || blobsOffset > requestsOffset {
		return nil, fmt.Errorf("SSZ get_payload response offsets out of order")
	}

	resp := &getPayloadResponseSSZ{
		ExecutionPayloadSSZ: data[payloadOffset:blobsOffset],
		BlockValue:          make([]byte, 32),
		BlobsBundleSSZ:      data[blobsOffset:requestsOffset],
		OverrideBuilder:     overrideBuilder,
	}
	copy(resp.BlockValue, blockValue)

	// Unmarshal execution requests.
	if requestsOffset < dataLen {
		reqData := data[requestsOffset:]
		requests := &pb.ExecutionRequests{}
		if err := requests.UnmarshalSSZ(reqData); err != nil {
			return nil, errors.Wrap(err, "unmarshal execution requests SSZ")
		}
		resp.ExecutionRequests = requests
	}

	return resp, nil
}

// marshalGetBlobsRequest creates the SSZ-encoded body for a get_blobs request.
// Layout (SSZ container):
//
//	versioned_hashes: List[Hash32, MAX_BLOB_COMMITMENTS_PER_BLOCK]
//
// Single variable field: offset(4) + concatenated hashes.
func marshalGetBlobsRequest(versionedHashes []common.Hash) []byte {
	const fixedSize = 4 // offset for the list
	hashesSize := len(versionedHashes) * 32
	buf := make([]byte, 0, fixedSize+hashesSize)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(fixedSize))
	for _, h := range versionedHashes {
		buf = append(buf, h[:]...)
	}
	return buf
}

// blobAndProofSize is the fixed SSZ size of a BlobAndProof.
// blob (131072 bytes) + kzg_proof (48 bytes) = 131120 bytes.
const blobAndProofSize = fieldparams.BlobLength + 48

// unmarshalGetBlobsResponseSSZ decodes an SSZ-encoded GetBlobsResponse.
// Layout (SSZ container):
//
//	blobs: List[BlobAndProof, MAX_BLOB_COMMITMENTS_PER_BLOCK]
//
// BlobAndProof is fixed-size (131120 bytes): blob(131072) + kzg_proof(48).
func unmarshalGetBlobsResponseSSZ(data []byte) ([]*pb.BlobAndProof, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("SSZ get_blobs response too short: %d bytes", len(data))
	}

	listOffset := binary.LittleEndian.Uint32(data[0:4])
	if listOffset > uint32(len(data)) {
		return nil, fmt.Errorf("SSZ get_blobs response truncated")
	}

	listData := data[listOffset:]
	if len(listData) == 0 {
		return []*pb.BlobAndProof{}, nil
	}
	if len(listData)%blobAndProofSize != 0 {
		return nil, fmt.Errorf("SSZ get_blobs response data size %d not divisible by BlobAndProof size %d", len(listData), blobAndProofSize)
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

// marshalExchangeCapabilitiesRequest creates the SSZ-encoded body for an exchange_capabilities request.
// Layout: SSZ Container { capabilities: List[List[uint8, 64], 128] }
// Wire format: container_offset(4) -> list_data = N * item_offset(4) + concatenated string bytes.
func marshalExchangeCapabilitiesRequest(capabilities []string) []byte {
	const containerFixedSize = 4 // offset for the capabilities list
	n := len(capabilities)

	// Calculate list data size: N offsets + concatenated strings
	offsetsSize := n * 4
	totalStrSize := 0
	for _, c := range capabilities {
		totalStrSize += len(c)
	}

	dst := make([]byte, 0, containerFixedSize+offsetsSize+totalStrSize)
	// Container offset pointing to start of list data
	dst = ssz.WriteOffset(dst, containerFixedSize)

	// List item offsets (relative to start of list data)
	itemOffset := offsetsSize
	for _, c := range capabilities {
		dst = ssz.WriteOffset(dst, itemOffset)
		itemOffset += len(c)
	}
	// Concatenated string data
	for _, c := range capabilities {
		dst = append(dst, []byte(c)...)
	}

	return dst
}

// unmarshalExchangeCapabilitiesResponse decodes an SSZ ExchangeCapabilitiesRequest/Response.
// Same format as the request: Container { capabilities: List[List[uint8, 64], 128] }
func unmarshalExchangeCapabilitiesResponse(data []byte) ([]string, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("SSZ exchange_capabilities response too short: %d bytes", len(data))
	}

	listOffset := uint32(ssz.ReadOffset(data[0:4]))
	if listOffset > uint32(len(data)) {
		return nil, fmt.Errorf("SSZ exchange_capabilities response truncated")
	}

	listData := data[listOffset:]
	if len(listData) == 0 {
		return []string{}, nil
	}

	return unmarshalSSZStringList(listData)
}

// unmarshalSSZStringList decodes an SSZ-encoded List[List[uint8, N], M] from raw list data.
// The list data starts with N 4-byte offsets, followed by concatenated string bytes.
func unmarshalSSZStringList(listData []byte) ([]string, error) {
	if len(listData) < 4 {
		return []string{}, nil
	}

	firstOffset := uint32(ssz.ReadOffset(listData[0:4]))
	if firstOffset%4 != 0 {
		return nil, fmt.Errorf("SSZ string list first offset %d not aligned to 4", firstOffset)
	}
	count := int(firstOffset / 4)
	if len(listData) < count*4 {
		return nil, fmt.Errorf("SSZ string list truncated: need %d bytes for offsets, have %d", count*4, len(listData))
	}

	offsets := make([]uint32, count)
	for i := range count {
		offsets[i] = uint32(ssz.ReadOffset(listData[i*4 : i*4+4]))
	}

	result := make([]string, count)
	for i := range count {
		start := offsets[i]
		var end uint32
		if i+1 < count {
			end = offsets[i+1]
		} else {
			end = uint32(len(listData))
		}
		if start > uint32(len(listData)) || end > uint32(len(listData)) || start > end {
			return nil, fmt.Errorf("SSZ string list offset out of bounds")
		}
		result[i] = string(listData[start:end])
	}

	return result, nil
}

// marshalClientVersionRequest creates the SSZ-encoded body for a get_client_version request.
// Layout (SSZ container):
//
//	code: List[uint8, 8]     (variable, offset at 0..4)
//	name: List[uint8, 64]    (variable, offset at 4..8)
//	version: List[uint8, 64] (variable, offset at 8..12)
//	commit: Bytes4           (fixed 4 bytes, at 12..16)
func marshalClientVersionRequest(code, name, ver, commit string) []byte {
	const fixedSize = 4 + 4 + 4 + 4 // 3 offsets + commit = 16 bytes
	codeBytes := []byte(code)
	nameBytes := []byte(name)
	verBytes := []byte(ver)
	commitBytes := parseCommitToBytes4(commit)

	buf := make([]byte, 0, fixedSize+len(codeBytes)+len(nameBytes)+len(verBytes))

	// Offsets for variable fields
	offset := uint32(fixedSize)
	buf = binary.LittleEndian.AppendUint32(buf, offset) // code offset
	offset += uint32(len(codeBytes))
	buf = binary.LittleEndian.AppendUint32(buf, offset) // name offset
	offset += uint32(len(nameBytes))
	buf = binary.LittleEndian.AppendUint32(buf, offset) // version offset

	// Fixed field: commit (4 bytes)
	buf = append(buf, commitBytes[:]...)

	// Variable data
	buf = append(buf, codeBytes...)
	buf = append(buf, nameBytes...)
	buf = append(buf, verBytes...)

	return buf
}

// parseCommitToBytes4 converts a hex commit string to a 4-byte array.
func parseCommitToBytes4(commit string) [4]byte {
	var result [4]byte
	b := common.FromHex(commit)
	if len(b) >= 4 {
		copy(result[:], b[:4])
	} else {
		copy(result[:], b)
	}
	return result
}

// unmarshalClientVersionResponse decodes an SSZ-encoded ClientVersionResponse.
// Layout (SSZ container):
//
//	versions: List[ClientVersionRequest, 16]
//
// Container: offset(4) -> list data
// List of variable-length ClientVersionRequest items: N offsets + concatenated items.
func unmarshalClientVersionResponse(data []byte) ([]*structs.ClientVersionV1, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("SSZ client_version response too short: %d bytes", len(data))
	}

	listOffset := binary.LittleEndian.Uint32(data[0:4])
	if listOffset > uint32(len(data)) {
		return nil, fmt.Errorf("SSZ client_version response truncated")
	}

	listData := data[listOffset:]
	if len(listData) == 0 {
		return []*structs.ClientVersionV1{}, nil
	}

	// Each ClientVersionRequest is variable-length, so the list has offsets.
	if len(listData) < 4 {
		return nil, fmt.Errorf("SSZ client_version list too short")
	}

	firstOffset := binary.LittleEndian.Uint32(listData[0:4])
	if firstOffset%4 != 0 {
		return nil, fmt.Errorf("SSZ client_version list first offset %d not aligned", firstOffset)
	}
	count := int(firstOffset / 4)
	if len(listData) < count*4 {
		return nil, fmt.Errorf("SSZ client_version list truncated")
	}

	offsets := make([]uint32, count)
	for i := range count {
		offsets[i] = binary.LittleEndian.Uint32(listData[i*4 : i*4+4])
	}

	result := make([]*structs.ClientVersionV1, count)
	for i := range count {
		start := offsets[i]
		var end uint32
		if i+1 < count {
			end = offsets[i+1]
		} else {
			end = uint32(len(listData))
		}
		if start > uint32(len(listData)) || end > uint32(len(listData)) || start > end {
			return nil, fmt.Errorf("SSZ client_version item offset out of bounds")
		}
		ver, err := unmarshalClientVersionItem(listData[start:end])
		if err != nil {
			return nil, errors.Wrapf(err, "unmarshal client version item %d", i)
		}
		result[i] = ver
	}

	return result, nil
}

// unmarshalClientVersionItem decodes a single SSZ-encoded ClientVersionRequest.
// Layout:
//
//	code: List[uint8, 8]     (variable, offset at 0..4)
//	name: List[uint8, 64]    (variable, offset at 4..8)
//	version: List[uint8, 64] (variable, offset at 8..12)
//	commit: Bytes4           (fixed 4 bytes, at 12..16)
func unmarshalClientVersionItem(data []byte) (*structs.ClientVersionV1, error) {
	const fixedSize = 16
	if len(data) < fixedSize {
		return nil, fmt.Errorf("SSZ client version item too short: %d bytes", len(data))
	}

	codeOffset := binary.LittleEndian.Uint32(data[0:4])
	nameOffset := binary.LittleEndian.Uint32(data[4:8])
	versionOffset := binary.LittleEndian.Uint32(data[8:12])
	commit := data[12:16]

	dataLen := uint32(len(data))
	if codeOffset > dataLen || nameOffset > dataLen || versionOffset > dataLen {
		return nil, fmt.Errorf("SSZ client version item truncated")
	}

	code := string(data[codeOffset:nameOffset])
	name := string(data[nameOffset:versionOffset])
	ver := string(data[versionOffset:])
	commitHex := fmt.Sprintf("%x", commit)

	return &structs.ClientVersionV1{
		Code:    code,
		Name:    name,
		Version: ver,
		Commit:  commitHex,
	}, nil
}

// blockValueToWei converts a 32-byte little-endian uint256 to primitives.Wei.
func blockValueToWei(blockValue []byte) primitives.Wei {
	return primitives.LittleEndianBytesToWei(blockValue)
}

