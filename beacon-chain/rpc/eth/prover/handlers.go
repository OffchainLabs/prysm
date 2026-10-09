package prover

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api/server"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/dustin/go-humanize"
	"github.com/sirupsen/logrus"
)

// maxExecutionProofsPerPayload is MAX_EXECUTION_PROOFS_PER_PAYLOAD, the maximum number of envelopes in one request.
const maxExecutionProofsPerPayload = 4

// SubmitExecutionProofs verifies signed execution proof envelopes as gossip would, imports them, and broadcasts them
// on the `execution_proof` gossip topic. An envelope failing verification does not prevent the others from being
// published.
// https://github.com/ethereum/beacon-APIs/pull/569
func (s *Server) SubmitExecutionProofs(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "prover.SubmitExecutionProofs")
	defer span.End()

	if !features.Get().EnableExecutionProofs {
		httputil.HandleError(w, "Execution proofs are not enabled on this node", http.StatusNotImplemented)
		return
	}

	envelopes, failures, err := decodeExecutionProofs(r)
	if err != nil {
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if len(envelopes) > maxExecutionProofsPerPayload {
		httputil.HandleError(w, fmt.Sprintf("Too many execution proofs: %d > %d", len(envelopes), maxExecutionProofsPerPayload), http.StatusBadRequest)
		return
	}

	internalFailure := false
	for i, envelope := range envelopes {
		if envelope == nil {
			// Decoding already reported this failure.
			continue
		}

		err := s.submitExecutionProof(ctx, envelope)
		if err == nil {
			continue
		}

		failures = append(failures, &server.IndexedError{Index: i, Message: err.Error()})

		var failure *submitFailure
		if !errors.As(err, &failure) || failure.internal {
			internalFailure = true
		}
	}

	if len(failures) > 0 {
		code := http.StatusBadRequest
		if internalFailure {
			code = http.StatusInternalServerError
		}

		httputil.WriteError(w, &server.IndexedErrorContainer{
			Code:     code,
			Message:  server.ErrIndexedValidationFail,
			Failures: failures,
		})
		return
	}

	w.WriteHeader(http.StatusOK)
}

// submitFailure is the failure of one envelope of a request. An internal failure is the node's fault, the others are
// the envelope's.
type submitFailure struct {
	err      error
	internal bool
}

func (f *submitFailure) Error() string { return f.err.Error() }
func (f *submitFailure) Unwrap() error { return f.err }

func invalid(format string, args ...any) error {
	return &submitFailure{err: fmt.Errorf(format, args...)}
}

func internal(format string, args ...any) error {
	return &submitFailure{err: fmt.Errorf(format, args...), internal: true}
}

// submitExecutionProof verifies, imports and broadcasts one signed execution proof envelope.
func (s *Server) submitExecutionProof(ctx context.Context, signed *ethpb.SignedExecutionProofEnvelope) error {
	envelope, err := blocks.NewROSignedExecutionProofEnvelope(signed)
	if err != nil {
		return invalid("invalid execution proof envelope: %w", err)
	}

	blockRoot := envelope.BeaconBlockRoot()
	proofType := envelope.ProofType()

	if s.ExecutionProofReceiver.HasExecutionProofType(blockRoot, proofType) {
		// A proof of this type is already known, and was already broadcast.
		return nil
	}

	newPayloadRequestRoot, err := s.ExecutionProofReceiver.CheckExecutionProofEnvelope(ctx, envelope)
	switch {
	case errors.Is(err, blockchain.ErrExecutionProofBlockUnknown),
		errors.Is(err, blockchain.ErrExecutionProofPayloadUnavailable),
		errors.Is(err, blockchain.ErrInvalidExecutionProofEnvelope):
		return invalid("%w", err)
	case err != nil:
		return internal("could not check execution proof envelope: %w", err)
	}

	slot, err := s.ExecutionProofReceiver.RecentBlockSlot(blockRoot)
	if err != nil {
		return internal("could not get slot of block %#x: %w", blockRoot, err)
	}

	err = s.ExecutionProofReceiver.VerifyExecutionProof(ctx, newPayloadRequestRoot, envelope)
	if errors.Is(err, blockchain.ErrInvalidExecutionProof) {
		return invalid("%w", err)
	}
	if err != nil {
		return internal("could not verify execution proof: %w", err)
	}

	err = s.ExecutionProofReceiver.ReceiveExecutionProof(ctx, blockRoot, proofType)
	if errors.Is(err, blockchain.ErrExecutionProofTypeKnown) {
		// Another proof of this type won the race for this block.
		return nil
	}
	if err != nil {
		return internal("could not receive execution proof: %w", err)
	}

	if err := s.Broadcaster.Broadcast(ctx, signed); err != nil {
		return internal("could not broadcast execution proof: %w", err)
	}

	log.WithFields(logrus.Fields{
		"slot":      slot,
		"blockRoot": fmt.Sprintf("%#x", bytesutil.Trunc(blockRoot[:])),
		"proofType": proofType,
		"prover":    signed.ValidatorIndex,
		"proofSize": humanize.Bytes(uint64(len(signed.Message.ProofData))),
	}).Info("Broadcast execution proof")

	return nil
}

// decodeExecutionProofs decodes the request body, a JSON array or an SSZ `List[SignedExecutionProofEnvelope,
// MAX_EXECUTION_PROOFS_PER_PAYLOAD]`. An element that cannot be decoded is nil, and reported as a failure.
func decodeExecutionProofs(r *http.Request) ([]*ethpb.SignedExecutionProofEnvelope, []*server.IndexedError, error) {
	if httputil.IsRequestSsz(r) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, nil, fmt.Errorf("read request body: %w", err)
		}

		elements, err := ssz.SplitVariableList(body, maxExecutionProofsPerPayload)
		if err != nil {
			return nil, nil, err
		}

		envelopes, failures := server.ConvertList(elements, func(element []byte) (*ethpb.SignedExecutionProofEnvelope, error) {
			envelope := &ethpb.SignedExecutionProofEnvelope{}
			if err := envelope.UnmarshalSSZ(element); err != nil {
				return nil, fmt.Errorf("could not decode SSZ execution proof envelope: %w", err)
			}
			return envelope, nil
		})

		return envelopes, failures, nil
	}

	return server.DecodeJSONList(r.Body, (*structs.SignedExecutionProofEnvelope).ToConsensus)
}
