package beacon

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/testutil"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz/query"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	sszquerypb "github.com/OffchainLabs/prysm/v7/proto/ssz_query"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ssz "github.com/prysmaticlabs/fastssz"
)

func TestQueryBeaconState(t *testing.T) {
	ctx := context.Background()

	st, _ := util.DeterministicGenesisState(t, 16)
	require.NoError(t, st.SetSlot(primitives.Slot(42)))
	require.NoError(t, st.UpdateBalancesAtIndex(0, 42000000000))
	require.NoError(t, st.UpdateBalancesAtIndex(14, 43000000000))
	require.NoError(t, st.UpdateBalancesAtIndex(15, 44000000000))

	stateRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)

	tests := []struct {
		path          string
		expectedValue func() []byte
	}{
		{
			path: ".slot",
			expectedValue: func() []byte {
				slot := st.Slot()
				b, _ := slot.MarshalSSZ()
				return b
			},
		},
		{
			path: ".latest_block_header",
			expectedValue: func() []byte {
				b, _ := st.LatestBlockHeader().MarshalSSZ()
				return b
			},
		},
		{
			path: ".randao_mixes[0]",
			expectedValue: func() []byte {
				return st.RandaoMixes()[0]
			},
		},
		{
			path: ".latest_block_header.proposer_index",
			expectedValue: func() []byte {
				b := make([]byte, 8)
				binary.LittleEndian.PutUint64(b, uint64(st.LatestBlockHeader().ProposerIndex))
				return b
			},
		},
		{
			path: ".validators",
			expectedValue: func() []byte {
				var b []byte
				for _, v := range st.Validators() {
					vBytes, _ := v.MarshalSSZ()
					b = append(b, vBytes...)
				}
				return b
			},
		},
		{
			path: ".validators[0]",
			expectedValue: func() []byte {
				v, _ := st.ValidatorAtIndex(0)
				b, _ := v.MarshalSSZ()
				return b
			},
		},
		{
			path: ".validators[0].withdrawal_credentials",
			expectedValue: func() []byte {
				v, _ := st.ValidatorAtIndex(0)
				return v.WithdrawalCredentials
			},
		},
		{
			path: ".validators[0].effective_balance",
			expectedValue: func() []byte {
				v, _ := st.ValidatorAtIndex(0)
				b := make([]byte, 8)
				binary.LittleEndian.PutUint64(b, uint64(v.EffectiveBalance))
				return b
			},
		},
		{
			path: ".balances[0]",
			expectedValue: func() []byte {
				bal, _ := st.BalanceAtIndex(0)
				b := make([]byte, 8)
				binary.LittleEndian.PutUint64(b, bal)
				return b
			},
		},
		{
			path: ".balances[14]",
			expectedValue: func() []byte {
				bal, _ := st.BalanceAtIndex(14)
				b := make([]byte, 8)
				binary.LittleEndian.PutUint64(b, bal)
				return b
			},
		},
		{
			path: ".balances[15]",
			expectedValue: func() []byte {
				bal, _ := st.BalanceAtIndex(15)
				b := make([]byte, 8)
				binary.LittleEndian.PutUint64(b, bal)
				return b
			},
		},
		{
			path: ".validators[15]",
			expectedValue: func() []byte {
				v, _ := st.ValidatorAtIndex(15)
				b, _ := v.MarshalSSZ()
				return b
			},
		},
		{
			path: "len(validators)",
			expectedValue: func() []byte {
				b := make([]byte, 8)
				binary.LittleEndian.PutUint64(b, uint64(len(st.Validators())))
				return b
			},
		},
	}

	chainService := &chainMock.ChainService{Optimistic: false, FinalizedRoots: make(map[[32]byte]bool)}
	s := &Server{
		OptimisticModeFetcher: chainService,
		FinalizationFetcher:   chainService,
		Stater: &testutil.MockStater{
			BeaconStateRoot: stateRoot[:],
			BeaconState:     st,
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			expectedValue := tt.expectedValue()

			t.Run("without_proof", func(t *testing.T) {
				requestBody := &structs.SSZQueryRequest{
					Query:        tt.path,
					IncludeProof: false,
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
					Result: expectedValue,
				}
				sszExpectedResponse, err := expectedResponse.MarshalSSZ()
				require.NoError(t, err)
				require.DeepEqual(t, sszExpectedResponse, writer.Body.Bytes())
			})

			t.Run("with_proof", func(t *testing.T) {
				requestBody := &structs.SSZQueryRequest{
					Query:        tt.path,
					IncludeProof: true,
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

				responseData := writer.Body.Bytes()
				var response sszquerypb.SSZQueryResponseWithProof
				require.NoError(t, response.UnmarshalSSZ(responseData))

				// Verify the proof is included
				require.NotNil(t, response.Proof)
				require.Equal(t, true, len(response.Proof.Proofs) > 0, "merkle proof should not be empty")

				// Verify the result
				require.DeepEqual(t, expectedValue, response.Result)
				require.DeepEqual(t, stateRoot[:], response.Root)

				// Verify the merkle proof
				merkleProof := &ssz.Proof{
					Index:  int(response.Proof.Gindex),
					Leaf:   response.Proof.Leaf,
					Hashes: response.Proof.Proofs,
				}
				isValid, err := ssz.VerifyProof(response.Root, merkleProof)
				require.NoError(t, err)
				require.Equal(t, true, isValid, "merkle proof verification failed")
			})
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
			name:        "len() on a non-collection field",
			stateId:     "head",
			path:        "len(slot)",
			errorString: "only supported for List and Bitlist",
		},
		{
			name:        "len() on an non-collection field with index",
			stateId:     "head",
			path:        "len(validators[0])",
			errorString: "only supported for List and Bitlist",
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
		{
			name:        "validator index out of range",
			stateId:     "head",
			path:        ".validators[100]",
			code:        http.StatusInternalServerError,
			errorString: "out of bounds",
		},
		{
			name:        "balance index out of range",
			stateId:     "head",
			path:        ".balances[100]",
			code:        http.StatusInternalServerError,
			errorString: "out of bounds",
		},
		{
			name:        "index on non-list field",
			stateId:     "head",
			path:        ".slot[0]",
			code:        http.StatusInternalServerError,
			errorString: "Could not calculate offset and length",
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

			requestBody := &structs.SSZQueryRequest{
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
			}
			require.Equal(t, tt.code, writer.Code)
			if tt.errorString != "" {
				errorString := writer.Body.String()
				require.Equal(t, true, strings.Contains(errorString, tt.errorString))
			}
		})
	}
}

func TestQueryBeaconBlock(t *testing.T) {
	randaoReveal, err := hexutil.Decode("0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505")
	require.NoError(t, err)
	root, err := hexutil.Decode("0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2")
	require.NoError(t, err)
	signature, err := hexutil.Decode("0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505")
	require.NoError(t, err)
	att := &eth.Attestation{
		AggregationBits: bitfield.Bitlist{0x01},
		Data: &eth.AttestationData{
			Slot:            1,
			CommitteeIndex:  1,
			BeaconBlockRoot: root,
			Source: &eth.Checkpoint{
				Epoch: 1,
				Root:  root,
			},
			Target: &eth.Checkpoint{
				Epoch: 1,
				Root:  root,
			},
		},
		Signature: signature,
	}

	tests := []struct {
		name          string
		path          string
		includeProof  bool
		block         interfaces.ReadOnlySignedBeaconBlock
		expectedValue []byte
	}{
		{
			name:         "slot",
			path:         ".slot",
			includeProof: false,
			block: func() interfaces.ReadOnlySignedBeaconBlock {
				b := util.NewBeaconBlock()
				b.Block.Slot = 123
				sb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return sb
			}(),
			expectedValue: func() []byte {
				b := make([]byte, 8)
				binary.LittleEndian.PutUint64(b, 123)
				return b
			}(),
		},
		{
			name:         "randao_reveal",
			path:         ".body.randao_reveal",
			includeProof: false,
			block: func() interfaces.ReadOnlySignedBeaconBlock {
				b := util.NewBeaconBlock()
				b.Block.Body.RandaoReveal = randaoReveal
				sb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return sb
			}(),
			expectedValue: randaoReveal,
		},
		{
			name:         "attestations",
			path:         ".body.attestations",
			includeProof: false,
			block: func() interfaces.ReadOnlySignedBeaconBlock {
				b := util.NewBeaconBlock()
				b.Block.Body.Attestations = []*eth.Attestation{
					att,
				}
				sb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return sb
			}(),
			expectedValue: func() []byte {
				b, err := att.MarshalSSZ()
				require.NoError(t, err)
				return b
			}(),
		},
		{
			name:         "slot",
			path:         ".slot",
			includeProof: true,
			block: func() interfaces.ReadOnlySignedBeaconBlock {
				b := util.NewBeaconBlock()
				b.Block.Slot = 123
				sb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return sb
			}(),
			expectedValue: func() []byte {
				b := make([]byte, 8)
				binary.LittleEndian.PutUint64(b, 123)
				return b
			}(),
		},
		{
			name:         "randao_reveal",
			path:         ".body.randao_reveal",
			includeProof: true,
			block: func() interfaces.ReadOnlySignedBeaconBlock {
				b := util.NewBeaconBlock()
				b.Block.Body.RandaoReveal = randaoReveal
				sb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return sb
			}(),
			expectedValue: randaoReveal,
		},
		{
			name:         "attestations",
			path:         ".body.attestations",
			includeProof: true,
			block: func() interfaces.ReadOnlySignedBeaconBlock {
				b := util.NewBeaconBlock()
				b.Block.Body.Attestations = []*eth.Attestation{
					att,
				}
				sb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return sb
			}(),
			expectedValue: func() []byte {
				b, err := att.MarshalSSZ()
				require.NoError(t, err)
				return b
			}(),
		},
		{
			name:         "len(body.attestations)",
			path:         "len(body.attestations)",
			includeProof: false,
			block: func() interfaces.ReadOnlySignedBeaconBlock {
				b := util.NewBeaconBlock()
				b.Block.Body.Attestations = []*eth.Attestation{att, att}
				sb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return sb
			}(),
			expectedValue: func() []byte {
				b := make([]byte, 8)
				binary.LittleEndian.PutUint64(b, 2)
				return b
			}(),
		},
		{
			name:         "len(body.attestations)",
			path:         "len(body.attestations)",
			includeProof: true,
			block: func() interfaces.ReadOnlySignedBeaconBlock {
				b := util.NewBeaconBlock()
				b.Block.Body.Attestations = []*eth.Attestation{att, att}
				sb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				return sb
			}(),
			expectedValue: func() []byte {
				b := make([]byte, 8)
				binary.LittleEndian.PutUint64(b, 2)
				return b
			}(),
		},
	}

	for _, tt := range tests {
		proofSuffix := "without proof"
		if tt.includeProof {
			proofSuffix = "with proof"
		}
		t.Run(fmt.Sprintf("%s %s", tt.path, proofSuffix), func(t *testing.T) {
			mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: tt.block}
			mockChainService := &chainMock.ChainService{
				FinalizedRoots: map[[32]byte]bool{},
			}
			s := &Server{
				FinalizationFetcher: mockChainService,
				Blocker:             mockBlockFetcher,
			}
			requestBody := &structs.SSZQueryRequest{
				Query:        tt.path,
				IncludeProof: tt.includeProof,
			}
			var buf bytes.Buffer
			require.NoError(t, json.NewEncoder(&buf).Encode(requestBody))

			request := httptest.NewRequest(http.MethodPost, "http://example.com/prysm/v1/beacon/blocks/{block_id}/query", &buf)
			request.SetPathValue("block_id", "head")
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.QueryBeaconBlock(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)
			assert.Equal(t, version.String(version.Phase0), writer.Header().Get(api.VersionHeader))

			blockRoot, err := tt.block.Block().HashTreeRoot()
			require.NoError(t, err)

			if !tt.includeProof {
				expectedResponse := &sszquerypb.SSZQueryResponse{
					Root:   blockRoot[:],
					Result: tt.expectedValue,
				}
				sszExpectedResponse, err := expectedResponse.MarshalSSZ()
				require.NoError(t, err)
				assert.DeepEqual(t, sszExpectedResponse, writer.Body.Bytes())
				return
			}

			// Decode the response to verify the proof
			responseData := writer.Body.Bytes()
			var response sszquerypb.SSZQueryResponseWithProof
			require.NoError(t, response.UnmarshalSSZ(responseData))

			// Verify the proof is included
			require.NotNil(t, response.Proof)
			require.Equal(t, true, len(response.Proof.Proofs) > 0, "merkle proof should not be empty")

			// Verify the result matches expected value
			require.DeepEqual(t, tt.expectedValue, response.Result)

			// Verify root matches block root
			require.DeepEqual(t, blockRoot[:], response.Root)

			// Verify the merkle proof using VerifyProof
			merkleProof := &ssz.Proof{
				Index:  int(response.Proof.Gindex),
				Leaf:   response.Proof.Leaf,
				Hashes: response.Proof.Proofs,
			}
			isValid, err := ssz.VerifyProof(response.Root, merkleProof)
			require.NoError(t, err)
			require.Equal(t, true, isValid, "merkle proof verification failed")
		})
	}
}

func TestGetBeaconStateProof(t *testing.T) {
	ctx := context.Background()

	st, _ := util.DeterministicGenesisState(t, 16)
	require.NoError(t, st.SetSlot(primitives.Slot(42)))
	require.NoError(t, st.UpdateBalancesAtIndex(0, 42000000000))
	require.NoError(t, st.UpdateBalancesAtIndex(14, 43000000000))
	require.NoError(t, st.UpdateBalancesAtIndex(15, 44000000000))

	stateRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)

	info, err := query.AnalyzeObject(st.ToProtoUnsafe().(query.SSZObject))
	require.NoError(t, err)

	tests := []struct {
		path        string
		expectError bool
	}{
		{
			// Basic SSZ type
			path: ".slot",
		},
		{
			// Container SSZ type
			path: ".latest_block_header",
		},
		{
			// List SSZ type (container elements)
			path: ".validators",
		},
		{
			// Accessing an element in a list
			path: ".validators[0]",
		},
		{
			// Accessing last element in a list (boundary)
			path: ".validators[15]",
		},
		{
			// Accessing an element in a list, then a field in the container
			path: ".validators[0].effective_balance",
		},
		{
			// Accessing a different nested field (bytes type)
			path: ".validators[0].withdrawal_credentials",
		},
		{
			// Accessing an element in a vector
			path: ".block_roots[0]",
		},
		{
			// Accessing different vector type
			path: ".randao_mixes[0]",
		},
		{
			// Accessing a field in a container
			path: ".latest_block_header.proposer_index",
		},
		{
			// BasicList (balances) - first element
			path: ".balances[0]",
		},
		{
			// BasicList (balances) - last element (boundary)
			path: ".balances[15]",
		},
		{
			// Nested container field
			path: ".finalized_checkpoint.root",
		},
		{
			path:        ".wrong_path",
			expectError: true,
		},
		{
			// Out of bounds index
			path:        ".validators[100]",
			expectError: true,
		},
		{
			path: ".slashings",
		},
		{
			path: ".slashings[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			path, err := query.ParsePath(tt.path)
			require.NoError(t, err)

			proof, err := getBeaconStateProof(ctx, st, info, path)
			if tt.expectError {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)

			isValid, err := ssz.VerifyProof(stateRoot[:], &ssz.Proof{
				Index:  int(proof.Gindex),
				Leaf:   proof.Leaf,
				Hashes: proof.Proofs,
			})
			require.NoError(t, err)
			require.Equal(t, true, isValid)
		})
	}
}
