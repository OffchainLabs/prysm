package prover

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
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

	sszBody := func(t *testing.T, envelopes ...*ethpb.SignedExecutionProofEnvelope) []byte {
		elements := make([][]byte, 0, len(envelopes))
		for _, e := range envelopes {
			encoded, err := e.MarshalSSZ()
			require.NoError(t, err)
			elements = append(elements, encoded)
		}
		return ssz.MarshalVariableList(elements...)
	}

	// submit also returns the proofs of the execution proof events sent on the operation feed.
	submit := func(t *testing.T, chain *mockChain.ChainService, body []byte, contentType string) (*httptest.ResponseRecorder, *mockp2p.MockBroadcaster, []*ethpb.SignedExecutionProofEnvelope) {
		broadcaster := &mockp2p.MockBroadcaster{}
		notifier := &mockChain.MockOperationNotifier{}
		server := &Server{Broadcaster: broadcaster, ExecutionProofReceiver: chain, OperationNotifier: notifier}

		events := make(chan *feed.Event, 16)
		sub := notifier.OperationFeed().Subscribe(events)
		defer sub.Unsubscribe()

		req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/execution_proofs", bytes.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		server.SubmitExecutionProofs(w, req)

		proofs := make([]*ethpb.SignedExecutionProofEnvelope, 0, len(events))
		for len(events) > 0 {
			event := <-events
			require.Equal(t, feed.EventType(opfeed.ExecutionProofReceived), event.Type)
			data, ok := event.Data.(*opfeed.ExecutionProofReceivedData)
			require.Equal(t, true, ok)
			proofs = append(proofs, data.Proof)
		}
		return w, broadcaster, proofs
	}

	failures := func(t *testing.T, w *httptest.ResponseRecorder) []*server.IndexedError {
		var container server.IndexedErrorContainer
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &container))
		return container.Failures
	}

	t.Run("ssz proofs are imported, broadcast and notified", func(t *testing.T) {
		chain := &mockChain.ChainService{}
		first, second := envelope(proofType, []byte{0x01}), envelope(ethpb.ProofTypeEthrexZisk, []byte{0x02, 0x03})
		w, broadcaster, proofs := submit(t, chain, sszBody(t, first, second), api.OctetStreamMediaType)
		require.Equal(t, 2, len(proofs))
		require.DeepEqual(t, first, proofs[0])
		require.DeepEqual(t, second, proofs[1])
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, true, chain.ExecutionProofTypes[blockRoot][proofType])
		require.Equal(t, true, chain.ExecutionProofTypes[blockRoot][ethpb.ProofTypeEthrexZisk])
		require.Equal(t, 2, len(broadcaster.BroadcastMessages))
	})

	t.Run("known proof type is neither broadcast nor notified again", func(t *testing.T) {
		chain := &mockChain.ChainService{ExecutionProofTypes: map[[32]byte]map[ethpb.ProofType]bool{blockRoot: {proofType: true}}}
		w, broadcaster, proofs := submit(t, chain, sszBody(t, envelope(proofType, []byte{0x01})), api.OctetStreamMediaType)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, false, broadcaster.BroadcastCalled.Load())
		require.Equal(t, 0, len(proofs))
	})

	t.Run("invalid proof does not prevent the others", func(t *testing.T) {
		chain := &mockChain.ChainService{}
		body := sszBody(t, envelope(proofType, nil), envelope(ethpb.ProofTypeEthrexZisk, []byte{0x01}))
		w, broadcaster, _ := submit(t, chain, body, api.OctetStreamMediaType)
		require.Equal(t, http.StatusBadRequest, w.Code)
		f := failures(t, w)
		require.Equal(t, 1, len(f))
		require.Equal(t, 0, f[0].Index)
		require.Equal(t, false, chain.ExecutionProofTypes[blockRoot][proofType])
		require.Equal(t, true, chain.ExecutionProofTypes[blockRoot][ethpb.ProofTypeEthrexZisk])
		require.Equal(t, 1, len(broadcaster.BroadcastMessages))
	})

	t.Run("unsupported proof type", func(t *testing.T) {
		w, _, _ := submit(t, &mockChain.ChainService{}, sszBody(t, envelope(ethpb.ProofType(0), []byte{0x01})), api.OctetStreamMediaType)
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
		t.Run(tc.name+" is neither imported, broadcast nor notified", func(t *testing.T) {
			chain := &mockChain.ChainService{CheckExecutionProofErr: tc.checkErr, VerifyExecutionProofErr: tc.verifyErr}
			w, broadcaster, proofs := submit(t, chain, sszBody(t, envelope(proofType, []byte{0x01})), api.OctetStreamMediaType)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, 1, len(failures(t, w)))
			require.Equal(t, false, chain.ExecutionProofTypes[blockRoot][proofType])
			require.Equal(t, false, broadcaster.BroadcastCalled.Load())
			require.Equal(t, 0, len(proofs))
		})
	}

	t.Run("json body", func(t *testing.T) {
		w, broadcaster, _ := submit(t, &mockChain.ChainService{}, []byte("[]"), api.JsonMediaType)
		require.Equal(t, http.StatusUnsupportedMediaType, w.Code)
		require.Equal(t, false, broadcaster.BroadcastCalled.Load())
	})

	t.Run("malformed ssz", func(t *testing.T) {
		w, _, _ := submit(t, &mockChain.ChainService{}, []byte{0x01, 0x02, 0x03, 0x04}, api.OctetStreamMediaType)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("more than four proofs", func(t *testing.T) {
		chain := &mockChain.ChainService{}
		proofTypes := []ethpb.ProofType{ethpb.ProofTypeEthrexSP1, ethpb.ProofTypeEthrexZisk}
		envelopes := make([]*ethpb.SignedExecutionProofEnvelope, 0, 5)
		for i := range 5 {
			envelopes = append(envelopes, envelope(proofTypes[i%len(proofTypes)], []byte{byte(i + 1)}))
		}
		w, broadcaster, _ := submit(t, chain, sszBody(t, envelopes...), api.OctetStreamMediaType)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, 2, len(broadcaster.BroadcastMessages))
	})

	t.Run("execution proofs disabled", func(t *testing.T) {
		resetCfg := features.InitWithReset(&features.Flags{})
		defer resetCfg()

		w, _, _ := submit(t, &mockChain.ChainService{}, sszBody(t, envelope(proofType, []byte{0x01})), api.OctetStreamMediaType)
		require.Equal(t, http.StatusNotImplemented, w.Code)
	})
}
