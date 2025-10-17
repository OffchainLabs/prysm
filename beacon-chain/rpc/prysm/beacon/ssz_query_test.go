package beacon

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	chainMock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/testutil"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	sszquerypb "github.com/OffchainLabs/prysm/v6/proto/ssz_query"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestQueryBeaconState(t *testing.T) {
	ctx := context.Background()

	st, _ := util.DeterministicGenesisState(t, 16)
	require.NoError(t, st.SetSlot(primitives.Slot(42)))
	stateRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.NoError(t, st.UpdateBalancesAtIndex(0, 42000000000))

	tests := []struct {
		path          string
		expectedValue []byte
	}{
		{
			path: ".slot",
			expectedValue: func() []byte {
				slot := st.Slot()
				result, _ := slot.MarshalSSZ()
				return result
			}(),
		},
		{
			path: ".latest_block_header",
			expectedValue: func() []byte {
				header := st.LatestBlockHeader()
				result, _ := header.MarshalSSZ()
				return result
			}(),
		},
		{
			path: ".validators",
			expectedValue: func() []byte {
				b := make([]byte, 0)
				validators := st.Validators()
				for _, v := range validators {
					vBytes, _ := v.MarshalSSZ()
					b = append(b, vBytes...)
				}
				return b

			}(),
		},
		{
			path: ".validators[0]",
			expectedValue: func() []byte {
				v, _ := st.ValidatorAtIndex(0)
				result, _ := v.MarshalSSZ()
				return result
			}(),
		},
		{
			path: ".validators[0].withdrawal_credentials",
			expectedValue: func() []byte {
				v, _ := st.ValidatorAtIndex(0)
				return v.WithdrawalCredentials
			}(),
		},
		{
			path: ".validators[0].effective_balance",
			expectedValue: func() []byte {
				v, _ := st.ValidatorAtIndex(0)
				b := make([]byte, 8)
				binary.LittleEndian.PutUint64(b, uint64(v.EffectiveBalance))
				return b
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			chainService := &chainMock.ChainService{Optimistic: false, FinalizedRoots: make(map[[32]byte]bool)}
			s := &Server{
				OptimisticModeFetcher: chainService,
				FinalizationFetcher:   chainService,
				Stater: &testutil.MockStater{
					BeaconStateRoot: stateRoot[:],
					BeaconState:     st,
				},
			}

			requestBody := &structs.QuerySSZRequest{
				Query: tt.path,
			}
			var buf bytes.Buffer
			require.NoError(t, json.NewEncoder(&buf).Encode(requestBody))

			request := httptest.NewRequest(http.MethodPost, "http://example.com/prysm/v1/beacon/states/{state_id}/query", &buf)
			request.SetPathValue("state_id", "head")
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.QueryBeaconState(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)
			assert.Equal(t, version.String(version.Phase0), writer.Header().Get(api.VersionHeader))

			expectedResponse := &sszquerypb.SSZQueryResponse{
				Root:   stateRoot[:],
				Result: tt.expectedValue,
			}
			sszExpectedResponse, err := expectedResponse.MarshalSSZ()
			require.NoError(t, err)
			assert.DeepEqual(t, sszExpectedResponse, writer.Body.Bytes())
		})
	}
}

func TestQueryBeaconStateInvalidRequest(t *testing.T) {
	ctx := context.Background()

	st, _ := util.DeterministicGenesisState(t, 16)
	require.NoError(t, st.SetSlot(primitives.Slot(42)))
	stateRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)

	tests := []struct {
		name        string
		stateId     string
		path        string
		code        int
		errorString string
	}{
		{
			name:        "empty query submitted",
			stateId:     "head",
			path:        "",
			errorString: "Empty query submitted",
		},
		{
			name:        "invalid path",
			stateId:     "head",
			path:        ".invalid[]]",
			errorString: "Could not parse path",
		},
		{
			name:        "non-existent field",
			stateId:     "head",
			path:        ".non_existent_field",
			code:        http.StatusInternalServerError,
			errorString: "Could not calculate offset and length for path",
		},
		{
			name:    "empty state ID",
			stateId: "",
			path:    "",
		},
		{
			name:    "far future slot",
			stateId: "1000000000000",
			path:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			chainService := &chainMock.ChainService{Optimistic: false, FinalizedRoots: make(map[[32]byte]bool)}
			s := &Server{
				OptimisticModeFetcher: chainService,
				FinalizationFetcher:   chainService,
				Stater: &testutil.MockStater{
					BeaconStateRoot: stateRoot[:],
					BeaconState:     st,
				},
			}

			requestBody := &structs.QuerySSZRequest{
				Query: tt.path,
			}
			var buf bytes.Buffer
			require.NoError(t, json.NewEncoder(&buf).Encode(requestBody))

			request := httptest.NewRequest(http.MethodPost, "http://example.com/prysm/v1/beacon/states/{state_id}/query", &buf)
			request.SetPathValue("state_id", tt.stateId)
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.QueryBeaconState(writer, request)

			if tt.code == 0 {
				tt.code = http.StatusBadRequest
			} else {
				tt.code = tt.code
			}
			require.Equal(t, tt.code, writer.Code)
			if tt.errorString != "" {
				errorString := writer.Body.String()
				require.Equal(t, true, strings.Contains(errorString, tt.errorString))
			}
		})
	}
}

func TestServer_QueryBeaconBlock(t *testing.T) {}
