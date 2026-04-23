package execution

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/network"
	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common"
)

func TestHandleSSZRestError(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		message  string
		expected error
	}{
		{name: "ErrParse", code: -32700, expected: ErrParse},
		{name: "ErrInvalidRequest", code: -32600, expected: ErrInvalidRequest},
		{name: "ErrMethodNotFound", code: -32601, expected: ErrMethodNotFound},
		{name: "ErrInvalidParams", code: -32602, expected: ErrInvalidParams},
		{name: "ErrInternal", code: -32603, expected: ErrInternal},
		{name: "ErrUnknownPayload", code: -38001, expected: ErrUnknownPayload},
		{name: "ErrInvalidForkchoiceState", code: -38002, expected: ErrInvalidForkchoiceState},
		{name: "ErrInvalidPayloadAttributes", code: -38003, expected: ErrInvalidPayloadAttributes},
		{name: "ErrRequestTooLarge", code: -38004, expected: ErrRequestTooLarge},
		{name: "ErrServer", code: -32000, message: "some server error", expected: ErrServer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restErr := &sszRestError{Code: tt.code, Message: tt.message}
			got := handleSSZRestError(restErr)
			require.ErrorContains(t, tt.expected.Error(), got)
		})
	}
	t.Run("unknown error code", func(t *testing.T) {
		restErr := &sszRestError{Code: -99999, Message: "unknown"}
		got := handleSSZRestError(restErr)
		require.ErrorContains(t, "SSZ-REST error", got)
	})
}

func TestHandlePayloadStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   pb.PayloadStatus_Status
		expected error
	}{
		{name: "VALID", status: pb.PayloadStatus_VALID, expected: nil},
		{name: "INVALID", status: pb.PayloadStatus_INVALID, expected: ErrInvalidPayloadStatus},
		{name: "SYNCING", status: pb.PayloadStatus_SYNCING, expected: ErrAcceptedSyncingPayloadStatus},
		{name: "ACCEPTED", status: pb.PayloadStatus_ACCEPTED, expected: ErrAcceptedSyncingPayloadStatus},
		{name: "INVALID_BLOCK_HASH", status: pb.PayloadStatus_INVALID_BLOCK_HASH, expected: ErrInvalidBlockHashPayloadStatus},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := &pb.PayloadStatus{Status: tt.status}
			got := handlePayloadStatus(ps)
			if tt.expected == nil {
				require.NoError(t, got)
			} else {
				require.ErrorContains(t, tt.expected.Error(), got)
			}
		})
	}
	t.Run("UNKNOWN status", func(t *testing.T) {
		ps := &pb.PayloadStatus{Status: pb.PayloadStatus_UNKNOWN}
		got := handlePayloadStatus(ps)
		require.ErrorContains(t, ErrUnknownPayloadStatus.Error(), got)
	})
}

func TestMarshalForkchoiceStateSSZ(t *testing.T) {
	head := make([]byte, 32)
	safe := make([]byte, 32)
	finalized := make([]byte, 32)
	for i := range 32 {
		head[i] = byte(i)
		safe[i] = byte(i + 32)
		finalized[i] = byte(i + 64)
	}

	state := &pb.ForkchoiceState{
		HeadBlockHash:      head,
		SafeBlockHash:      safe,
		FinalizedBlockHash: finalized,
	}

	result, err := state.MarshalSSZ()
	require.NoError(t, err)
	require.Equal(t, 96, len(result))
	require.DeepEqual(t, head, result[0:32])
	require.DeepEqual(t, safe, result[32:64])
	require.DeepEqual(t, finalized, result[64:96])
}

func TestAppendHash(t *testing.T) {
	t.Run("full 32 bytes", func(t *testing.T) {
		hash := make([]byte, 32)
		for i := range hash {
			hash[i] = byte(i)
		}
		result := appendHash(nil, hash)
		require.Equal(t, 32, len(result))
		require.DeepEqual(t, hash, result)
	})
	t.Run("short hash zero-padded", func(t *testing.T) {
		hash := []byte{1, 2, 3}
		result := appendHash(nil, hash)
		require.Equal(t, 32, len(result))
		assert.Equal(t, byte(1), result[0])
		assert.Equal(t, byte(2), result[1])
		assert.Equal(t, byte(3), result[2])
		for i := 3; i < 32; i++ {
			assert.Equal(t, byte(0), result[i])
		}
	})
	t.Run("longer than 32 bytes truncated", func(t *testing.T) {
		hash := make([]byte, 40)
		for i := range hash {
			hash[i] = byte(i)
		}
		result := appendHash(nil, hash)
		require.Equal(t, 32, len(result))
		require.DeepEqual(t, hash[:32], result)
	})
}

// buildPayloadStatusSSZ creates a valid SSZ-encoded PayloadStatusSSZ for testing.
func buildPayloadStatusSSZ(statusByte uint8, latestValidHash []byte, validationError string) []byte {
	const fixedSize = 9 // 1 byte status + 4 byte hashOffset + 4 byte errorOffset
	hashOffset := uint32(fixedSize)

	// Build latest_valid_hash as List[Hash32, 1].
	var hashData []byte
	if latestValidHash != nil {
		hashData = append(hashData, latestValidHash...)
	}

	errorOffset := hashOffset + uint32(len(hashData))
	errorData := []byte(validationError)

	buf := make([]byte, 0, fixedSize+len(hashData)+len(errorData))
	buf = append(buf, statusByte)
	buf = binary.LittleEndian.AppendUint32(buf, hashOffset)
	buf = binary.LittleEndian.AppendUint32(buf, errorOffset)
	buf = append(buf, hashData...)
	buf = append(buf, errorData...)
	return buf
}

func TestUnmarshalPayloadStatusSSZ(t *testing.T) {
	t.Run("VALID status with hash", func(t *testing.T) {
		hash := make([]byte, 32)
		for i := range hash {
			hash[i] = byte(i + 1)
		}
		data := buildPayloadStatusSSZ(sszPayloadStatusValid, hash, "")
		status, err := unmarshalPayloadStatusSSZ(data)
		require.NoError(t, err)
		assert.Equal(t, pb.PayloadStatus_VALID, status.Status)
		require.DeepEqual(t, hash, status.LatestValidHash)
		assert.Equal(t, "", status.ValidationError)
	})
	t.Run("INVALID status with validation error", func(t *testing.T) {
		data := buildPayloadStatusSSZ(sszPayloadStatusInvalid, nil, "block is invalid")
		status, err := unmarshalPayloadStatusSSZ(data)
		require.NoError(t, err)
		assert.Equal(t, pb.PayloadStatus_INVALID, status.Status)
		assert.Equal(t, 0, len(status.LatestValidHash))
		assert.Equal(t, "block is invalid", status.ValidationError)
	})
	t.Run("SYNCING status no hash no error", func(t *testing.T) {
		data := buildPayloadStatusSSZ(sszPayloadStatusSyncing, nil, "")
		status, err := unmarshalPayloadStatusSSZ(data)
		require.NoError(t, err)
		assert.Equal(t, pb.PayloadStatus_SYNCING, status.Status)
	})
	t.Run("ACCEPTED status", func(t *testing.T) {
		data := buildPayloadStatusSSZ(sszPayloadStatusAccepted, nil, "")
		status, err := unmarshalPayloadStatusSSZ(data)
		require.NoError(t, err)
		assert.Equal(t, pb.PayloadStatus_ACCEPTED, status.Status)
	})
	t.Run("INVALID_BLOCK_HASH status", func(t *testing.T) {
		data := buildPayloadStatusSSZ(sszPayloadStatusInvalidBlockHash, nil, "")
		status, err := unmarshalPayloadStatusSSZ(data)
		require.NoError(t, err)
		assert.Equal(t, pb.PayloadStatus_INVALID_BLOCK_HASH, status.Status)
	})
	t.Run("unknown status byte", func(t *testing.T) {
		data := buildPayloadStatusSSZ(99, nil, "")
		_, err := unmarshalPayloadStatusSSZ(data)
		require.ErrorContains(t, "unknown SSZ payload status", err)
	})
	t.Run("too short data", func(t *testing.T) {
		_, err := unmarshalPayloadStatusSSZ([]byte{0, 1, 2})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// buildForkchoiceUpdatedResponseSSZ builds a valid SSZ-encoded ForkchoiceUpdatedResponse.
func buildForkchoiceUpdatedResponseSSZ(payloadStatus []byte, payloadId *[8]byte) []byte {
	const fixedSize = 8 // 2 x uint32 offsets
	statusOffset := uint32(fixedSize)
	payloadIdOffset := statusOffset + uint32(len(payloadStatus))

	var pidData []byte
	if payloadId != nil {
		pidData = append(pidData, payloadId[:]...)
	}

	buf := make([]byte, 0, fixedSize+len(payloadStatus)+len(pidData))
	buf = binary.LittleEndian.AppendUint32(buf, statusOffset)
	buf = binary.LittleEndian.AppendUint32(buf, payloadIdOffset)
	buf = append(buf, payloadStatus...)
	buf = append(buf, pidData...)
	return buf
}

func TestUnmarshalForkchoiceUpdatedResponseSSZ(t *testing.T) {
	t.Run("VALID with payload ID", func(t *testing.T) {
		hash := make([]byte, 32)
		for i := range hash {
			hash[i] = byte(i)
		}
		statusSSZ := buildPayloadStatusSSZ(sszPayloadStatusValid, hash, "")
		pid := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
		data := buildForkchoiceUpdatedResponseSSZ(statusSSZ, &pid)

		resp, err := unmarshalForkchoiceUpdatedResponseSSZ(data)
		require.NoError(t, err)
		assert.Equal(t, pb.PayloadStatus_VALID, resp.Status.Status)
		require.DeepEqual(t, hash, resp.Status.LatestValidHash)
		require.NotNil(t, resp.PayloadId)
		require.DeepEqual(t, pb.PayloadIDBytes(pid), *resp.PayloadId)
	})
	t.Run("SYNCING without payload ID", func(t *testing.T) {
		statusSSZ := buildPayloadStatusSSZ(sszPayloadStatusSyncing, nil, "")
		data := buildForkchoiceUpdatedResponseSSZ(statusSSZ, nil)

		resp, err := unmarshalForkchoiceUpdatedResponseSSZ(data)
		require.NoError(t, err)
		assert.Equal(t, pb.PayloadStatus_SYNCING, resp.Status.Status)
		assert.Equal(t, true, resp.PayloadId == nil)
	})
	t.Run("too short data", func(t *testing.T) {
		_, err := unmarshalForkchoiceUpdatedResponseSSZ([]byte{0, 1, 2})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestMarshalForkchoiceUpdatedRequest(t *testing.T) {
	head := make([]byte, 32)
	safe := make([]byte, 32)
	finalized := make([]byte, 32)
	for i := range 32 {
		head[i] = 0x11
		safe[i] = 0x22
		finalized[i] = 0x33
	}
	state := &pb.ForkchoiceState{
		HeadBlockHash:      head,
		SafeBlockHash:      safe,
		FinalizedBlockHash: finalized,
	}

	t.Run("without payload attributes", func(t *testing.T) {
		result, err := marshalForkchoiceUpdatedRequest(state, nil)
		require.NoError(t, err)
		// Fixed size: 96 (forkchoice state) + 4 (offset) = 100
		require.Equal(t, 100, len(result))
		// Verify forkchoice state
		require.DeepEqual(t, head, result[0:32])
		require.DeepEqual(t, safe, result[32:64])
		require.DeepEqual(t, finalized, result[64:96])
		// Offset should point to end of data (no union)
		offset := binary.LittleEndian.Uint32(result[96:100])
		assert.Equal(t, uint32(100), offset)
	})
}

func TestDoRequest(t *testing.T) {
	t.Run("successful request", func(t *testing.T) {
		expectedResp := []byte{1, 2, 3, 4, 5}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, sszContentType, r.Header.Get("Content-Type"))
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.DeepEqual(t, []byte{0xAA, 0xBB}, body)
			w.WriteHeader(http.StatusOK)
			_, err = w.Write(expectedResp)
			require.NoError(t, err)
		}))
		defer srv.Close()

		client := newSSZRestClient(srv.URL, srv.Client())
		resp, err := client.doRequest(context.Background(), "/test/path", []byte{0xAA, 0xBB})
		require.NoError(t, err)
		require.DeepEqual(t, expectedResp, resp)
	})
	t.Run("error response with JSON body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			errResp := sszRestError{Code: -32602, Message: "invalid params"}
			err := json.NewEncoder(w).Encode(errResp)
			require.NoError(t, err)
		}))
		defer srv.Close()

		client := newSSZRestClient(srv.URL, srv.Client())
		_, err := client.doRequest(context.Background(), "/test/path", nil)
		require.ErrorContains(t, ErrInvalidParams.Error(), err)
	})
	t.Run("error response with non-JSON body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, err := w.Write([]byte("internal error"))
			require.NoError(t, err)
		}))
		defer srv.Close()

		client := newSSZRestClient(srv.URL, srv.Client())
		_, err := client.doRequest(context.Background(), "/test/path", nil)
		require.ErrorContains(t, "returned status 500", err)
	})
	t.Run("empty body request", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.Equal(t, 0, len(body))
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		client := newSSZRestClient(srv.URL, srv.Client())
		resp, err := client.doRequest(context.Background(), "/test", nil)
		require.NoError(t, err)
		assert.Equal(t, 0, len(resp))
	})
}

func TestSSZRestError_Error(t *testing.T) {
	e := &sszRestError{Code: -32602, Message: "invalid params"}
	assert.Equal(t, "SSZ-REST error (code -32602): invalid params", e.Error())
}

func TestIsSSZRestAvailable(t *testing.T) {
	t.Run("available when client set", func(t *testing.T) {
		s := &Service{sszRestClient: &sszRestClient{}}
		assert.Equal(t, true, s.isSSZRestAvailable())
	})
	t.Run("not available when client nil", func(t *testing.T) {
		s := &Service{}
		assert.Equal(t, false, s.isSSZRestAvailable())
	})
}

func TestSetupSSZRestClientDisableFlag(t *testing.T) {
	t.Run("enabled by default", func(t *testing.T) {
		s := &Service{
			cfg: &config{
				currHttpEndpoint: network.HttpEndpoint("http://localhost:8551"),
			},
		}
		s.setupSSZRestClient()
		assert.Equal(t, true, s.isSSZRestAvailable())
	})

	t.Run("disabled clears client", func(t *testing.T) {
		s := &Service{
			cfg: &config{
				currHttpEndpoint:  network.HttpEndpoint("http://localhost:8551"),
				disableSSZRouting: true,
			},
			sszRestClient: &sszRestClient{},
		}
		s.setupSSZRestClient()
		assert.Equal(t, false, s.isSSZRestAvailable())
	})
}

func TestNewSSZRestClient(t *testing.T) {
	httpClient := &http.Client{}
	client := newSSZRestClient("http://localhost:6367", httpClient)
	assert.Equal(t, "http://localhost:6367", client.baseURL)
	assert.Equal(t, true, client.httpClient == httpClient)
}

func TestDoRequestURLConstruction(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := newSSZRestClient(srv.URL, srv.Client())
	_, err := client.doRequest(context.Background(), "/engine/v4/new_payload", nil)
	require.NoError(t, err)
	assert.Equal(t, "/engine/v4/new_payload", capturedPath)
}

func TestPayloadStatusRoundTrip(t *testing.T) {
	// Test all status types round-trip correctly through marshal/unmarshal.
	statuses := []struct {
		name   string
		sszVal uint8
		pbVal  pb.PayloadStatus_Status
	}{
		{"VALID", sszPayloadStatusValid, pb.PayloadStatus_VALID},
		{"INVALID", sszPayloadStatusInvalid, pb.PayloadStatus_INVALID},
		{"SYNCING", sszPayloadStatusSyncing, pb.PayloadStatus_SYNCING},
		{"ACCEPTED", sszPayloadStatusAccepted, pb.PayloadStatus_ACCEPTED},
		{"INVALID_BLOCK_HASH", sszPayloadStatusInvalidBlockHash, pb.PayloadStatus_INVALID_BLOCK_HASH},
	}
	for _, tt := range statuses {
		t.Run(tt.name, func(t *testing.T) {
			hash := make([]byte, 32)
			for i := range hash {
				hash[i] = byte(i)
			}
			data := buildPayloadStatusSSZ(tt.sszVal, hash, "")
			status, err := unmarshalPayloadStatusSSZ(data)
			require.NoError(t, err)
			assert.Equal(t, tt.pbVal, status.Status)
			require.DeepEqual(t, hash, status.LatestValidHash)
		})
	}
}

func TestForkchoiceUpdatedResponseRoundTrip(t *testing.T) {
	// Test VALID status with payload ID.
	hash := make([]byte, 32)
	for i := range hash {
		hash[i] = byte(i)
	}
	statusSSZ := buildPayloadStatusSSZ(sszPayloadStatusValid, hash, "")
	pid := [8]byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80}
	data := buildForkchoiceUpdatedResponseSSZ(statusSSZ, &pid)

	resp, err := unmarshalForkchoiceUpdatedResponseSSZ(data)
	require.NoError(t, err)
	assert.Equal(t, pb.PayloadStatus_VALID, resp.Status.Status)
	require.DeepEqual(t, hash, resp.Status.LatestValidHash)
	require.NotNil(t, resp.PayloadId)
	for i := range 8 {
		assert.Equal(t, pid[i], resp.PayloadId[i])
	}

	// Test INVALID without payload ID.
	statusSSZ2 := buildPayloadStatusSSZ(sszPayloadStatusInvalid, nil, "bad block")
	data2 := buildForkchoiceUpdatedResponseSSZ(statusSSZ2, nil)

	resp2, err := unmarshalForkchoiceUpdatedResponseSSZ(data2)
	require.NoError(t, err)
	assert.Equal(t, pb.PayloadStatus_INVALID, resp2.Status.Status)
	assert.Equal(t, true, resp2.PayloadId == nil)
	assert.Equal(t, "bad block", resp2.Status.ValidationError)
}

func TestDoRequestContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response - should be cancelled by context.
		<-r.Context().Done()
	}))
	defer srv.Close()

	client := newSSZRestClient(srv.URL, srv.Client())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.doRequest(ctx, "/test", nil)
	require.NotNil(t, err)
}

func TestHandleSSZRestErrorAllCodes(t *testing.T) {
	// Verify all error codes map correctly by checking they don't return the raw sszRestError.
	knownCodes := []int{-32700, -32600, -32601, -32602, -32603, -38001, -38002, -38003, -38004, -32000}
	for _, code := range knownCodes {
		t.Run(fmt.Sprintf("code_%d", code), func(t *testing.T) {
			restErr := &sszRestError{Code: code, Message: "test"}
			got := handleSSZRestError(restErr)
			// Known codes should NOT return the raw sszRestError type.
			_, isRaw := got.(*sszRestError)
			assert.Equal(t, false, isRaw, "code %d returned raw sszRestError", code)
		})
	}
}

// Tests for get_payload SSZ encoding/decoding.

func TestUnmarshalGetPayloadResponseSSZ(t *testing.T) {
	t.Run("v4 generated response with all fields", func(t *testing.T) {
		var blockValue [32]byte
		blockValue[0] = 0x42
		wire := &pb.GetPayloadV4ResponseSSZ{
			Payload:               testExecutionPayloadDeneb(),
			BlockValue:            blockValue,
			BlobsBundle:           &pb.BlobsBundle{},
			ShouldOverrideBuilder: true,
			ExecutionRequests:     &pb.ExecutionRequests{},
		}

		data, err := wire.MarshalSSZ()
		require.NoError(t, err)
		resp, err := unmarshalGetPayloadResponseSSZ(data, 4)
		require.NoError(t, err)
		require.NotNil(t, resp.ExecutionData)
		require.NotNil(t, resp.BlobsBundler)
		require.NotNil(t, resp.ExecutionRequests)
		assert.Equal(t, true, resp.OverrideBuilder)
		require.DeepEqual(t, blockValueToWei(blockValue[:]), resp.Bid)
	})

	t.Run("v3 override builder false", func(t *testing.T) {
		var blockValue [32]byte
		wire := &pb.GetPayloadV3ResponseSSZ{
			Payload:     testExecutionPayloadDeneb(),
			BlockValue:  blockValue,
			BlobsBundle: &pb.BlobsBundle{},
		}
		data, err := wire.MarshalSSZ()
		require.NoError(t, err)
		resp, err := unmarshalGetPayloadResponseSSZ(data, 3)
		require.NoError(t, err)
		assert.Equal(t, false, resp.OverrideBuilder)
	})

	t.Run("too short data", func(t *testing.T) {
		_, err := unmarshalGetPayloadResponseSSZ([]byte{0, 1, 2, 3}, 4)
		require.NotNil(t, err)
	})
}

func testExecutionPayloadDeneb() *pb.ExecutionPayloadDeneb {
	return &pb.ExecutionPayloadDeneb{
		ParentHash:    make([]byte, 32),
		FeeRecipient:  make([]byte, 20),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
		PrevRandao:    make([]byte, 32),
		ExtraData:     []byte{},
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
		Transactions:  [][]byte{},
		Withdrawals:   []*pb.Withdrawal{},
	}
}

// Tests for get_blobs SSZ encoding/decoding.

func TestMarshalGetBlobsRequest(t *testing.T) {
	hashes := []common.Hash{
		common.HexToHash("0x0102030405060708091011121314151617181920212223242526272829303132"),
		common.HexToHash("0xaabbccddee000000000000000000000000000000000000000000000000000000"),
	}
	data := marshalGetBlobsRequest(hashes)
	require.Equal(t, 4+64, len(data)) // 4 byte offset + 2*32 bytes
	offset := binary.LittleEndian.Uint32(data[0:4])
	assert.Equal(t, uint32(4), offset)
	require.DeepEqual(t, hashes[0][:], data[4:36])
	require.DeepEqual(t, hashes[1][:], data[36:68])
}

func TestMarshalGetBlobsRequestEmpty(t *testing.T) {
	data := marshalGetBlobsRequest(nil)
	require.Equal(t, 4, len(data))
	offset := binary.LittleEndian.Uint32(data[0:4])
	assert.Equal(t, uint32(4), offset)
}

func buildGetBlobsResponseSSZ(blobs []*pb.BlobAndProof) []byte {
	const fixedSize = 4
	listData := make([]byte, 0, len(blobs)*blobAndProofSize)
	for _, b := range blobs {
		padBlob := make([]byte, fieldparams.BlobLength)
		if b.Blob != nil {
			copy(padBlob, b.Blob)
		}
		listData = append(listData, padBlob...)
		padProof := make([]byte, 48)
		if b.KzgProof != nil {
			copy(padProof, b.KzgProof)
		}
		listData = append(listData, padProof...)
	}
	buf := make([]byte, 0, fixedSize+len(listData))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(fixedSize))
	buf = append(buf, listData...)
	return buf
}

func TestUnmarshalGetBlobsResponseSSZ(t *testing.T) {
	t.Run("single blob", func(t *testing.T) {
		blob := make([]byte, fieldparams.BlobLength)
		blob[0] = 0xAA
		blob[fieldparams.BlobLength-1] = 0xBB
		proof := make([]byte, 48)
		proof[0] = 0xCC
		blobs := []*pb.BlobAndProof{{Blob: blob, KzgProof: proof}}

		data := buildGetBlobsResponseSSZ(blobs)
		result, err := unmarshalGetBlobsResponseSSZ(data)
		require.NoError(t, err)
		require.Equal(t, 1, len(result))
		assert.Equal(t, byte(0xAA), result[0].Blob[0])
		assert.Equal(t, byte(0xBB), result[0].Blob[fieldparams.BlobLength-1])
		assert.Equal(t, byte(0xCC), result[0].KzgProof[0])
	})

	t.Run("empty blobs", func(t *testing.T) {
		data := buildGetBlobsResponseSSZ(nil)
		result, err := unmarshalGetBlobsResponseSSZ(data)
		require.NoError(t, err)
		require.Equal(t, 0, len(result))
	})

	t.Run("too short data", func(t *testing.T) {
		_, err := unmarshalGetBlobsResponseSSZ([]byte{0, 1})
		require.ErrorContains(t, "too short", err)
	})

	t.Run("invalid size", func(t *testing.T) {
		// Data not divisible by blobAndProofSize
		buf := make([]byte, 4+100)
		binary.LittleEndian.PutUint32(buf[0:4], 4)
		_, err := unmarshalGetBlobsResponseSSZ(buf)
		require.ErrorContains(t, "not divisible", err)
	})
}

// Tests for exchange_capabilities SSZ encoding/decoding.

func TestMarshalExchangeCapabilitiesRequest(t *testing.T) {
	caps := []string{"engine_newPayloadV4", "engine_forkchoiceUpdatedV3"}
	data := marshalExchangeCapabilitiesRequest(caps)
	// Container offset (4) + list data
	require.Equal(t, true, len(data) > 4)
}

func TestExchangeCapabilitiesRoundTrip(t *testing.T) {
	caps := []string{"engine_newPayloadV4", "engine_forkchoiceUpdatedV3", "engine_getPayloadV4"}
	data := marshalExchangeCapabilitiesRequest(caps)
	result, err := unmarshalExchangeCapabilitiesResponse(data)
	require.NoError(t, err)
	require.Equal(t, len(caps), len(result))
	for i, c := range caps {
		assert.Equal(t, c, result[i])
	}
}

func TestExchangeCapabilitiesRoundTripEmpty(t *testing.T) {
	data := marshalExchangeCapabilitiesRequest(nil)
	result, err := unmarshalExchangeCapabilitiesResponse(data)
	require.NoError(t, err)
	require.Equal(t, 0, len(result))
}

func TestExchangeCapabilitiesRoundTripSingle(t *testing.T) {
	caps := []string{"engine_newPayloadV4"}
	data := marshalExchangeCapabilitiesRequest(caps)
	result, err := unmarshalExchangeCapabilitiesResponse(data)
	require.NoError(t, err)
	require.Equal(t, 1, len(result))
	assert.Equal(t, "engine_newPayloadV4", result[0])
}

func TestUnmarshalExchangeCapabilitiesResponseTooShort(t *testing.T) {
	_, err := unmarshalExchangeCapabilitiesResponse([]byte{0, 1})
	require.ErrorContains(t, "incorrect size", err)
}

// Tests for client_version SSZ encoding/decoding.

func buildClientVersionResponseSSZ(versions []structs.ClientVersionV1) []byte {
	// Build each item's SSZ
	items := make([][]byte, len(versions))
	for i, v := range versions {
		items[i] = marshalClientVersionRequest(v.Code, v.Name, v.Version, v.Commit)
	}

	// Outer container: offset(4) -> list data
	// List data: N offsets(4 each) + concatenated items
	const containerFixed = 4
	listOffsetsSize := len(items) * 4
	totalItemSize := 0
	for _, item := range items {
		totalItemSize += len(item)
	}

	buf := make([]byte, 0, containerFixed+listOffsetsSize+totalItemSize)
	// Container offset
	buf = binary.LittleEndian.AppendUint32(buf, uint32(containerFixed))

	// List offsets
	offset := uint32(listOffsetsSize)
	for _, item := range items {
		buf = binary.LittleEndian.AppendUint32(buf, offset)
		offset += uint32(len(item))
	}
	// List items
	for _, item := range items {
		buf = append(buf, item...)
	}
	return buf
}

func TestMarshalClientVersionRequest(t *testing.T) {
	data := marshalClientVersionRequest("PM", "Prysm", "v5.0.0", "abcdef01")
	require.Equal(t, true, len(data) >= 16) // At minimum: 3 offsets + 4 bytes commit
}

func TestClientVersionRoundTrip(t *testing.T) {
	versions := []structs.ClientVersionV1{
		{Code: "PM", Name: "Prysm", Version: "v5.0.0", Commit: "abcdef01"},
		{Code: "EG", Name: "Erigon", Version: "v3.0.0", Commit: "12345678"},
	}
	data := buildClientVersionResponseSSZ(versions)
	result, err := unmarshalClientVersionResponse(data)
	require.NoError(t, err)
	require.Equal(t, 2, len(result))
	assert.Equal(t, "PM", result[0].Code)
	assert.Equal(t, "Prysm", result[0].Name)
	assert.Equal(t, "v5.0.0", result[0].Version)
	assert.Equal(t, "EG", result[1].Code)
	assert.Equal(t, "Erigon", result[1].Name)
}

func TestClientVersionRoundTripSingle(t *testing.T) {
	versions := []structs.ClientVersionV1{
		{Code: "GE", Name: "Geth", Version: "v1.14.0", Commit: "aabb0011"},
	}
	data := buildClientVersionResponseSSZ(versions)
	result, err := unmarshalClientVersionResponse(data)
	require.NoError(t, err)
	require.Equal(t, 1, len(result))
	assert.Equal(t, "GE", result[0].Code)
	assert.Equal(t, "Geth", result[0].Name)
	assert.Equal(t, "v1.14.0", result[0].Version)
}

func TestUnmarshalClientVersionResponseTooShort(t *testing.T) {
	_, err := unmarshalClientVersionResponse([]byte{0, 1})
	require.ErrorContains(t, "incorrect size", err)
}

func TestParseCommitToBytes4(t *testing.T) {
	t.Run("full hex string", func(t *testing.T) {
		result := parseCommitToBytes4("abcdef01")
		assert.Equal(t, byte(0xab), result[0])
		assert.Equal(t, byte(0xcd), result[1])
		assert.Equal(t, byte(0xef), result[2])
		assert.Equal(t, byte(0x01), result[3])
	})
	t.Run("short hex string", func(t *testing.T) {
		result := parseCommitToBytes4("ab")
		assert.Equal(t, byte(0xab), result[0])
		assert.Equal(t, byte(0), result[1])
	})
	t.Run("with 0x prefix", func(t *testing.T) {
		result := parseCommitToBytes4("0xaabbccdd")
		assert.Equal(t, byte(0xaa), result[0])
		assert.Equal(t, byte(0xbb), result[1])
		assert.Equal(t, byte(0xcc), result[2])
		assert.Equal(t, byte(0xdd), result[3])
	})
}

// Tests for SSZ-REST endpoint integration via httptest.

func TestGetBlobsSSZRestEndpoint(t *testing.T) {
	blob := make([]byte, fieldparams.BlobLength)
	blob[0] = 0x42
	proof := make([]byte, 48)
	proof[0] = 0x99
	blobsData := []*pb.BlobAndProof{{Blob: blob, KzgProof: proof}}
	responseSSZ := buildGetBlobsResponseSSZ(blobsData)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/engine/v1/get_blobs", r.URL.Path)
		assert.Equal(t, sszContentType, r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(responseSSZ)
		require.NoError(t, err)
	}))
	defer srv.Close()

	s := &Service{sszRestClient: newSSZRestClient(srv.URL, srv.Client())}
	hashes := []common.Hash{common.HexToHash("0x01")}
	result, err := s.getBlobsSSZRest(context.Background(), hashes)
	require.NoError(t, err)
	require.Equal(t, 1, len(result))
	assert.Equal(t, byte(0x42), result[0].Blob[0])
	assert.Equal(t, byte(0x99), result[0].KzgProof[0])
}

func TestExchangeCapabilitiesSSZRestEndpoint(t *testing.T) {
	caps := []string{"engine_newPayloadV4", "engine_getPayloadV4"}
	responseSSZ := marshalExchangeCapabilitiesRequest(caps)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/engine/v1/exchange_capabilities", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(responseSSZ)
		require.NoError(t, err)
	}))
	defer srv.Close()

	s := &Service{sszRestClient: newSSZRestClient(srv.URL, srv.Client())}
	result, err := s.exchangeCapabilitiesSSZRest(context.Background(), []string{"engine_newPayloadV4"})
	require.NoError(t, err)
	require.Equal(t, 2, len(result))
	assert.Equal(t, "engine_newPayloadV4", result[0])
	assert.Equal(t, "engine_getPayloadV4", result[1])
}

func TestGetClientVersionSSZRestEndpoint(t *testing.T) {
	versions := []structs.ClientVersionV1{
		{Code: "GE", Name: "Geth", Version: "v1.14.0", Commit: "aabb0011"},
	}
	responseSSZ := buildClientVersionResponseSSZ(versions)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/engine/v1/get_client_version", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(responseSSZ)
		require.NoError(t, err)
	}))
	defer srv.Close()

	s := &Service{sszRestClient: newSSZRestClient(srv.URL, srv.Client())}
	result, err := s.getClientVersionSSZRest(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, len(result))
	assert.Equal(t, "GE", result[0].Code)
	assert.Equal(t, "Geth", result[0].Name)
}

func TestBlockValueToWei(t *testing.T) {
	blockValue := make([]byte, 32)
	blockValue[0] = 0x42 // Little-endian
	wei := blockValueToWei(blockValue)
	require.NotNil(t, wei)
}
