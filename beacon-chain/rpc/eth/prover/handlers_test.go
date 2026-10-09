package prover

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestSubmitExecutionProofs(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{EnableExecutionProofs: true})
	defer resetCfg()

	blockRoot := bytesutil.ToBytes32([]byte("block"))
	proofType := ethpb.ProofTypeEthrexSP1

	envelope := func(proofType ethpb.ProofType, proofData []byte) *ethpb.SignedExecutionProofEnvelope {
		return &ethpb.SignedExecutionProofEnvelope{
			Message: &ethpb.ExecutionProofEnvelope{
				ProofData:       proofData,
				ProofType:       []byte{byte(proofType)},
				BeaconBlockRoot: blockRoot[:],
			},
			Signature: make([]byte, 96),
		}
	}

	jsonBody := func(t *testing.T, envelopes ...*ethpb.SignedExecutionProofEnvelope) []byte {
		items := make([]*structs.SignedExecutionProofEnvelope, 0, len(envelopes))
		for _, e := range envelopes {
			items = append(items, &structs.SignedExecutionProofEnvelope{
				Message: &structs.ExecutionProofEnvelope{
					ProofData:       hexutil.Encode(e.Message.ProofData),
					ProofType:       strconv.Itoa(int(e.Message.ProofType[0])),
					BeaconBlockRoot: hexutil.Encode(e.Message.BeaconBlockRoot),
				},
				ValidatorIndex: strconv.FormatUint(uint64(e.ValidatorIndex), 10),
				Signature:      hexutil.Encode(e.Signature),
			})
		}
		body, err := json.Marshal(items)
		require.NoError(t, err)
		return body
	}

	sszBody := func(t *testing.T, envelopes ...*ethpb.SignedExecutionProofEnvelope) []byte {
		elements := make([][]byte, 0, len(envelopes))
		for _, e := range envelopes {
			encoded, err := e.MarshalSSZ()
			require.NoError(t, err)
			elements = append(elements, encoded)
		}
		return ssz.MarshalVariableList(elements...)
	}

	submit := func(t *testing.T, chain *mockChain.ChainService, body []byte, ssz bool) (*httptest.ResponseRecorder, *mockp2p.MockBroadcaster) {
		broadcaster := &mockp2p.MockBroadcaster{}
		server := &Server{Broadcaster: broadcaster, ExecutionProofReceiver: chain}

		req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_proofs", bytes.NewReader(body))
		if ssz {
			req.Header.Set("Content-Type", api.OctetStreamMediaType)
		}
		w := httptest.NewRecorder()
		server.SubmitExecutionProofs(w, req)
		return w, broadcaster
	}

	failures := func(t *testing.T, w *httptest.ResponseRecorder) []*server.IndexedError {
		var container server.IndexedErrorContainer
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &container))
		return container.Failures
	}

	t.Run("json proof is imported and broadcast", func(t *testing.T) {
		chain := &mockChain.ChainService{}
		w, broadcaster := submit(t, chain, jsonBody(t, envelope(proofType, []byte{0x01})), false)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, true, chain.ExecutionProofTypes[blockRoot][proofType])
		require.Equal(t, true, broadcaster.BroadcastCalled.Load())
	})

	t.Run("ssz proofs are imported and broadcast", func(t *testing.T) {
		chain := &mockChain.ChainService{}
		body := sszBody(t, envelope(proofType, []byte{0x01}), envelope(ethpb.ProofTypeEthrexZisk, []byte{0x02, 0x03}))
		w, broadcaster := submit(t, chain, body, true)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, true, chain.ExecutionProofTypes[blockRoot][proofType])
		require.Equal(t, true, chain.ExecutionProofTypes[blockRoot][ethpb.ProofTypeEthrexZisk])
		require.Equal(t, 2, len(broadcaster.BroadcastMessages))
	})

	t.Run("known proof type is not broadcast again", func(t *testing.T) {
		chain := &mockChain.ChainService{ExecutionProofTypes: map[[32]byte]map[ethpb.ProofType]bool{blockRoot: {proofType: true}}}
		w, broadcaster := submit(t, chain, jsonBody(t, envelope(proofType, []byte{0x01})), false)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, false, broadcaster.BroadcastCalled.Load())
	})

	t.Run("invalid proof does not prevent the others", func(t *testing.T) {
		chain := &mockChain.ChainService{}
		body := jsonBody(t, envelope(proofType, nil), envelope(ethpb.ProofTypeEthrexZisk, []byte{0x01}))
		w, broadcaster := submit(t, chain, body, false)
		require.Equal(t, http.StatusBadRequest, w.Code)
		f := failures(t, w)
		require.Equal(t, 1, len(f))
		require.Equal(t, 0, f[0].Index)
		require.Equal(t, false, chain.ExecutionProofTypes[blockRoot][proofType])
		require.Equal(t, true, chain.ExecutionProofTypes[blockRoot][ethpb.ProofTypeEthrexZisk])
		require.Equal(t, 1, len(broadcaster.BroadcastMessages))
	})

	t.Run("unsupported proof type", func(t *testing.T) {
		w, _ := submit(t, &mockChain.ChainService{}, jsonBody(t, envelope(ethpb.ProofType(0), []byte{0x01})), false)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	cases := []struct {
		name      string
		checkErr  error
		verifyErr error
		status    int
	}{
		{name: "unknown block", checkErr: blockchain.ErrExecutionProofBlockUnknown, status: http.StatusBadRequest},
		{name: "payload not available", checkErr: blockchain.ErrExecutionProofPayloadUnavailable, status: http.StatusBadRequest},
		{name: "invalid envelope", checkErr: blockchain.ErrInvalidExecutionProofEnvelope, status: http.StatusBadRequest},
		{name: "envelope check failure", checkErr: errors.New("state unavailable"), status: http.StatusInternalServerError},
		{name: "invalid proof", verifyErr: blockchain.ErrInvalidExecutionProof, status: http.StatusBadRequest},
		{name: "verification failure", verifyErr: errors.New("engine failure"), status: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name+" is neither imported nor broadcast", func(t *testing.T) {
			chain := &mockChain.ChainService{CheckExecutionProofErr: tc.checkErr, VerifyExecutionProofErr: tc.verifyErr}
			w, broadcaster := submit(t, chain, jsonBody(t, envelope(proofType, []byte{0x01})), false)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, 1, len(failures(t, w)))
			require.Equal(t, false, chain.ExecutionProofTypes[blockRoot][proofType])
			require.Equal(t, false, broadcaster.BroadcastCalled.Load())
		})
	}

	t.Run("malformed json", func(t *testing.T) {
		w, _ := submit(t, &mockChain.ChainService{}, []byte("{"), false)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("malformed ssz", func(t *testing.T) {
		w, _ := submit(t, &mockChain.ChainService{}, []byte{0x01, 0x02, 0x03, 0x04}, true)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("too many proofs", func(t *testing.T) {
		e := envelope(proofType, []byte{0x01})
		w, broadcaster := submit(t, &mockChain.ChainService{}, jsonBody(t, e, e, e, e, e), false)
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Equal(t, false, broadcaster.BroadcastCalled.Load())
	})

	t.Run("execution proofs disabled", func(t *testing.T) {
		resetCfg := features.InitWithReset(&features.Flags{})
		defer resetCfg()

		w, _ := submit(t, &mockChain.ChainService{}, jsonBody(t, envelope(proofType, []byte{0x01})), false)
		require.Equal(t, http.StatusNotImplemented, w.Code)
	})
}
