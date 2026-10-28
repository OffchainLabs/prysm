package execution_test

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

func FuzzForkChoiceResponse(f *testing.F) {
	f.Add([]byte(`{"payloadStatus":{"status":"VALID","latestValidHash":"0x0707070707070707070707070707070707070707070707070707070707070707","validationError":null},"payloadId":"0x0102030405060708"}`))
	f.Add([]byte(`{"payloadStatus":{"status":"INVALID","latestValidHash":"0x0808080808080808080808080808080808080808080808080808080808080808","validationError":"bad block"},"payloadId":null}`))
	f.Add([]byte(`{"payloadStatus":{"status":"SYNCING","latestValidHash":null,"validationError":null},"payloadId":null}`))
	// Prysm-only top-level field with the wrong type: only Prysm rejects it.
	f.Add([]byte(`{"validationError":0}`))
	f.Fuzz(func(t *testing.T, jsonBlob []byte) {
		gethResp := &engine.ForkChoiceResponse{}
		prysmResp := &execution.ForkchoiceUpdatedResponse{}
		gethErr := json.Unmarshal(jsonBlob, gethResp)
		prysmErr := json.Unmarshal(jsonBlob, prysmResp)
		if (gethErr == nil) != (prysmErr == nil) {
			// Only a field one side lacks may decide acceptance: Prysm's top-level validationError
			// string, or geth's payloadStatus.witness.
			oneSided := hasJSONKey(jsonBlob, "", "validationError") || hasJSONKey(jsonBlob, "payloadStatus", "witness")
			require.Equal(t, true, oneSided, "geth and prysm unmarshaller return inconsistent errors. %v and %v", gethErr, prysmErr)
			return
		}
		// Nothing to marshal if we have an error.
		if gethErr != nil {
			return
		}
		gethBlob, err := json.Marshal(gethResp)
		require.NoError(t, err)
		prysmBlob, err := json.Marshal(prysmResp)
		require.NoError(t, err)
		newGethResp := &engine.ForkChoiceResponse{}
		require.NoError(t, json.Unmarshal(prysmBlob, newGethResp))
		newGethResp2 := &engine.ForkChoiceResponse{}
		require.NoError(t, json.Unmarshal(gethBlob, newGethResp2))

		// Prysm maps a status it does not know to UNKNOWN, and has no status without a payloadStatus
		// object. Those are the only allowed status differences.
		if _, known := pb.PayloadStatus_Status_value[newGethResp2.PayloadStatus.Status]; !known {
			want := "UNKNOWN"
			if prysmResp.Status == nil {
				want = ""
			}
			require.Equal(t, want, newGethResp.PayloadStatus.Status)
			newGethResp.PayloadStatus.Status = newGethResp2.PayloadStatus.Status
		}
		require.Equal(t, newGethResp2.PayloadStatus.Status, newGethResp.PayloadStatus.Status)
		require.Equal(t, fmt.Sprintf("%v", newGethResp2.PayloadID), fmt.Sprintf("%v", newGethResp.PayloadID))
		require.Equal(t, fmt.Sprintf("%v", newGethResp2.PayloadStatus.LatestValidHash), fmt.Sprintf("%v", newGethResp.PayloadStatus.LatestValidHash))
		// Prysm stores a missing validation error as "".
		validationError := func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		}
		require.Equal(t, validationError(newGethResp2.PayloadStatus.ValidationError), validationError(newGethResp.PayloadStatus.ValidationError))
	})
}

func FuzzExecutionPayload(f *testing.F) {
	// A Capella engine_getPayloadV2 response that both decoders accept.
	seed := `{"executionPayload":{` +
		`"parentHash":"0x0101010101010101010101010101010101010101010101010101010101010101",` +
		`"feeRecipient":"0x0202020202020202020202020202020202020202",` +
		`"stateRoot":"0x0303030303030303030303030303030303030303030303030303030303030303",` +
		`"receiptsRoot":"0x0404040404040404040404040404040404040404040404040404040404040404",` +
		`"logsBloom":"0x` + strings.Repeat("00", 256) + `",` +
		`"prevRandao":"0x0505050505050505050505050505050505050505050505050505050505050505",` +
		`"blockNumber":"0x1","gasLimit":"0x1c9c380","gasUsed":"0x5208","timestamp":"0x64",` +
		`"extraData":"0x","baseFeePerGas":"0x7",` +
		`"blockHash":"0x0606060606060606060606060606060606060606060606060606060606060606",` +
		`"transactions":["0x02"],` +
		`"withdrawals":[{"index":"0x1","validatorIndex":"0x2","address":"0x0909090909090909090909090909090909090909","amount":"0x3"}]},` +
		`"blockValue":"0x1bc16d674ec80000"}`
	f.Add([]byte(seed))
	// A later-fork field with the wrong type: only geth rejects it.
	f.Add([]byte(strings.Replace(seed, `"blockValue"`, `"shouldOverrideBuilder":"invalid","blockValue"`, 1)))
	f.Fuzz(func(t *testing.T, jsonBlob []byte) {
		gethResp := &engine.ExecutionPayloadEnvelope{}
		prysmResp := &pb.ExecutionPayloadCapellaWithValue{}
		gethErr := json.Unmarshal(jsonBlob, gethResp)
		prysmErr := json.Unmarshal(jsonBlob, prysmResp)
		// Prysm may reject what geth accepts (it requires every withdrawal field).
		if prysmErr != nil {
			return
		}
		if gethErr != nil {
			// geth also parses fields of later forks, which a Capella response ignores. Only those may
			// make geth reject what Prysm accepts.
			laterFork := hasJSONKey(jsonBlob, "", "blobsBundle", "executionRequests", "shouldOverrideBuilder", "witness") ||
				hasJSONKey(jsonBlob, "executionPayload", "blobGasUsed", "excessBlobGas", "slotNumber", "blockAccessList")
			require.Equal(t, true, laterFork, "prysm accepted an input that geth rejects: %v", gethErr)
			return
		}

		// The response type has no MarshalJSON, so encode Prysm's decoded payload and value in the
		// response layout and decode that with geth.
		value := new(big.Int).SetBytes(bytesutil.ReverseByteOrder(prysmResp.Value))
		prysmBlob, err := json.Marshal(struct {
			ExecutionPayload *pb.ExecutionPayloadCapella `json:"executionPayload"`
			BlockValue       *hexutil.Big                `json:"blockValue"`
		}{prysmResp.Payload, (*hexutil.Big)(value)})
		require.NoError(t, err)
		fromPrysm := &engine.ExecutionPayloadEnvelope{}
		require.NoError(t, json.Unmarshal(prysmBlob, fromPrysm))

		// A Capella payload drops fields of later forks and turns missing withdrawals into an empty list.
		p := gethResp.ExecutionPayload
		p.BlobGasUsed, p.ExcessBlobGas, p.SlotNumber, p.BlockAccessList = nil, nil, nil, nil
		if p.Withdrawals == nil {
			p.Withdrawals = []*types.Withdrawal{}
		}
		gethResp.BlobsBundle, gethResp.Requests, gethResp.Override, gethResp.Witness = nil, nil, false, nil
		gethJSON, err := json.Marshal(gethResp)
		require.NoError(t, err)
		prysmJSON, err := json.Marshal(fromPrysm)
		require.NoError(t, err)
		require.Equal(t, string(gethJSON), string(prysmJSON))
	})
}

// hasJSONKey reports whether the JSON object in blob has a key that encoding/json would match to one
// of names, which it does case-insensitively. With parent set, it looks in the objects under the
// top-level keys that match parent.
func hasJSONKey(blob []byte, parent string, names ...string) bool {
	var obj map[string]json.RawMessage
	if json.Unmarshal(blob, &obj) != nil {
		return false
	}
	for k, v := range obj {
		if parent != "" {
			if strings.EqualFold(k, parent) && hasJSONKey(v, "", names...) {
				return true
			}
			continue
		}
		for _, name := range names {
			if strings.EqualFold(k, name) {
				return true
			}
		}
	}
	return false
}
