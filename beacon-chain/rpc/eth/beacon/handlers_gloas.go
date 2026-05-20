package beacon

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetExecutionPayloadEnvelope retrieves a full execution payload envelope by beacon block root.
// The blinded envelope is fetched from the DB and the full execution payload is reconstructed
// from the EL via eth_getBlockByHash.
func (s *Server) GetExecutionPayloadEnvelope(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetExecutionPayloadEnvelope")
	defer span.End()

	blockID := r.PathValue("block_id")
	if blockID == "" {
		httputil.HandleError(w, "block_id is required in URL params", http.StatusBadRequest)
		return
	}

	root, err := s.Blocker.BlockRoot(ctx, []byte(blockID))
	if !shared.WriteBlockRootFetchError(w, err) {
		return
	}

	blinded, err := s.BeaconDB.ExecutionPayloadEnvelope(ctx, root)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			httputil.HandleError(w, "execution payload envelope not found", http.StatusNotFound)
			return
		}
		httputil.HandleError(w, "could not retrieve execution payload envelope: "+err.Error(), http.StatusInternalServerError)
		return
	}
	full, err := s.ExecutionReconstructor.ReconstructExecutionPayloadEnvelope(ctx, blinded)
	if err != nil {
		httputil.HandleError(w, "could not reconstruct execution payload envelope: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(api.VersionHeader, version.String(version.Gloas))

	if httputil.RespondWithSsz(r) {
		sszBytes, err := full.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, "could not marshal envelope to SSZ: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, sszBytes)
		return
	}

	isOptimistic, err := s.OptimisticModeFetcher.IsOptimisticForRoot(ctx, root)
	if err != nil {
		httputil.HandleError(w, "could not check optimistic status: "+err.Error(), http.StatusInternalServerError)
		return
	}
	finalized := s.FinalizationFetcher.IsFinalized(ctx, root)

	jsonEnvelope, err := structs.SignedExecutionPayloadEnvelopeFromConsensus(full)
	if err != nil {
		httputil.HandleError(w, "could not convert envelope to JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httputil.WriteJson(w, &structs.GetExecutionPayloadEnvelopeResponse{
		Version:             version.String(version.Gloas),
		ExecutionOptimistic: isOptimistic,
		Finalized:           finalized,
		Data:                jsonEnvelope,
	})
}

// PublishExecutionPayloadEnvelope broadcasts a signed envelope. Body may be
// either SignedExecutionPayloadEnvelope (stateful) or
// SignedExecutionPayloadEnvelopeContents (stateless, with blobs+proofs).
// Endpoint: POST /eth/v1/beacon/execution_payload_envelope
func (s *Server) PublishExecutionPayloadEnvelope(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.PublishExecutionPayloadEnvelope")
	defer span.End()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		httputil.HandleError(w, "could not read request body: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var consensus *eth.SignedExecutionPayloadEnvelope
	if httputil.IsRequestSsz(r) {
		// Dispatch by SSZ lead offset: 12 = Contents, 100 = bare envelope.
		contentsLeadOffset := []byte{12, 0, 0, 0}
		if bytes.HasPrefix(body, contentsLeadOffset) {
			contents := &eth.SignedExecutionPayloadEnvelopeContents{}
			if err := contents.UnmarshalSSZ(body); err != nil {
				httputil.HandleError(w, "could not decode SSZ envelope contents: "+err.Error(), http.StatusBadRequest)
				return
			}
			s.publishExecutionPayloadEnvelopeContentsSSZ(ctx, w, r, contents)
			return
		}
		consensus = &eth.SignedExecutionPayloadEnvelope{}
		if err := consensus.UnmarshalSSZ(body); err != nil {
			httputil.HandleError(w, "could not decode SSZ envelope: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		// Contents wraps the signed envelope alongside blobs/kzg_proofs; the
		// wrapper key distinguishes it from a bare envelope body.
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(body, &probe); err != nil {
			httputil.HandleError(w, "could not decode request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if _, isContents := probe["signed_execution_payload_envelope"]; isContents {
			s.publishExecutionPayloadEnvelopeContents(ctx, w, r, body)
			return
		}

		var jsonEnvelope structs.SignedExecutionPayloadEnvelope
		if err := json.Unmarshal(body, &jsonEnvelope); err != nil {
			httputil.HandleError(w, "could not decode request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		consensus, err = jsonEnvelope.ToConsensus()
		if err != nil {
			httputil.HandleError(w, "invalid signed execution payload envelope: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	if err := s.validateEnvelopeBroadcast(ctx, r, consensus); err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if _, err := s.V1Alpha1ValidatorServer.PublishExecutionPayloadEnvelope(ctx, consensus); err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.InvalidArgument:
				httputil.HandleError(w, st.Message(), http.StatusBadRequest)
			default:
				httputil.HandleError(w, st.Message(), http.StatusInternalServerError)
			}
			return
		}
		httputil.HandleError(w, "could not publish execution payload envelope: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// publishExecutionPayloadEnvelopeContents handles the JSON stateless variant.
func (s *Server) publishExecutionPayloadEnvelopeContents(ctx context.Context, w http.ResponseWriter, r *http.Request, body []byte) {
	var contents structs.SignedExecutionPayloadEnvelopeContents
	if err := json.Unmarshal(body, &contents); err != nil {
		httputil.HandleError(w, "could not decode envelope contents: "+err.Error(), http.StatusBadRequest)
		return
	}
	signed, kzgProofs, blobs, err := contents.ToConsensus()
	if err != nil {
		httputil.HandleError(w, "invalid signed execution payload envelope contents: "+err.Error(), http.StatusBadRequest)
		return
	}
	s.processEnvelopeContents(ctx, w, r, signed, kzgProofs, blobs)
}

// publishExecutionPayloadEnvelopeContentsSSZ handles the SSZ stateless variant.
func (s *Server) publishExecutionPayloadEnvelopeContentsSSZ(ctx context.Context, w http.ResponseWriter, r *http.Request, contents *eth.SignedExecutionPayloadEnvelopeContents) {
	if contents == nil || contents.SignedExecutionPayloadEnvelope == nil {
		httputil.HandleError(w, "nil signed execution payload envelope contents", http.StatusBadRequest)
		return
	}
	s.processEnvelopeContents(ctx, w, r, contents.SignedExecutionPayloadEnvelope, contents.KzgProofs, contents.Blobs)
}

// processEnvelopeContents verifies caller-supplied blobs/proofs, broadcasts
// derived sidecars, then delegates the envelope to the bare publish path.
func (s *Server) processEnvelopeContents(ctx context.Context, w http.ResponseWriter, r *http.Request, signed *eth.SignedExecutionPayloadEnvelope, kzgProofs, blobs [][]byte) {
	if err := s.validateEnvelopeBroadcast(ctx, r, signed); err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(blobs) > 0 {
		blockRoot := bytesutil.ToBytes32(signed.Message.BeaconBlockRoot)
		cellsPerBlob, proofsPerBlob, err := peerdas.ComputeCellsAndProofsFromFlat(blobs, kzgProofs)
		if err != nil {
			httputil.HandleError(w, "could not compute cells and proofs: "+err.Error(), http.StatusBadRequest)
			return
		}
		// External trust boundary — verify before broadcasting/storing.
		if err := verifyCellProofs(blobs, kzgProofs); err != nil {
			httputil.HandleError(w, "kzg verification failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		roSidecars, err := peerdas.DataColumnSidecarsGloas(cellsPerBlob, proofsPerBlob, primitives.Slot(signed.Message.Payload.SlotNumber), blockRoot)
		if err != nil {
			httputil.HandleError(w, "could not build data column sidecars: "+err.Error(), http.StatusInternalServerError)
			return
		}
		verifiedSidecars := make([]consensusblocks.VerifiedRODataColumn, 0, len(roSidecars))
		for _, sc := range roSidecars {
			verifiedSidecars = append(verifiedSidecars, consensusblocks.NewVerifiedRODataColumn(sc))
		}
		if err := s.Broadcaster.BroadcastDataColumnSidecars(ctx, verifiedSidecars); err != nil {
			httputil.HandleError(w, "could not broadcast data column sidecars: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := s.DataColumnReceiver.ReceiveDataColumns(verifiedSidecars); err != nil {
			httputil.HandleError(w, "could not receive data column sidecars: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if _, err := s.V1Alpha1ValidatorServer.PublishExecutionPayloadEnvelope(ctx, signed); err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.InvalidArgument:
				httputil.HandleError(w, st.Message(), http.StatusBadRequest)
			default:
				httputil.HandleError(w, st.Message(), http.StatusInternalServerError)
			}
			return
		}
		httputil.HandleError(w, "could not publish execution payload envelope contents: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// validateEnvelopeBroadcast applies broadcast_validation semantics to an
// envelope publish before it is broadcast to gossip. Spec: beacon-APIs #580.
//   - gossip (default): no extra checks.
//   - consensus: run full envelope consensus checks (signature + payload
//     consistency against the pre-state at envelope.beacon_block_root).
//   - consensus_and_equivocation: consensus + reject if a different beacon
//     block at the envelope's slot has already been received.
func (s *Server) validateEnvelopeBroadcast(ctx context.Context, r *http.Request, signed *eth.SignedExecutionPayloadEnvelope) error {
	level := r.URL.Query().Get(broadcastValidationQueryParam)
	switch level {
	case "", broadcastValidationGossip:
		return nil
	case broadcastValidationConsensus, broadcastValidationConsensusAndEquivocation:
	default:
		return errors.Errorf("invalid %s value: %q", broadcastValidationQueryParam, level)
	}

	envSlot := primitives.Slot(signed.Message.Payload.SlotNumber)
	envRoot := bytesutil.ToBytes32(signed.Message.BeaconBlockRoot)

	if level == broadcastValidationConsensusAndEquivocation {
		if s.ForkchoiceFetcher.HighestReceivedBlockSlot() == envSlot &&
			s.ForkchoiceFetcher.HighestReceivedBlockRoot() != envRoot {
			return errors.Wrapf(errEquivocatedBlock, "another block for slot %d already exists in fork choice", envSlot)
		}
	}

	st, err := s.StateGenService.StateByRoot(ctx, envRoot)
	if err != nil {
		return errors.Wrap(err, "could not get state for envelope beacon block root")
	}
	if st == nil || st.IsNil() {
		return errors.Errorf("could not get state for envelope beacon block root %#x", envRoot)
	}
	roSigned, err := consensusblocks.WrappedROSignedExecutionPayloadEnvelope(signed)
	if err != nil {
		return errors.Wrap(err, "could not wrap signed envelope")
	}
	if err := gloas.VerifyExecutionPayloadEnvelope(ctx, st, roSigned); err != nil {
		return errors.Wrap(err, "consensus validation failed")
	}
	return nil
}

// verifyCellProofs batch-verifies cell proofs against commitments derived
// from the supplied blobs. Does not tie data to a specific block — that needs
// the block's BlobKzgCommitments which a stateless receiver may not have.
func verifyCellProofs(blobs [][]byte, flatProofs [][]byte) error {
	commitments := make([][]byte, len(blobs))
	for i, blob := range blobs {
		if len(blob) != len(kzg.Blob{}) {
			return errors.Errorf("blob %d has wrong size %d", i, len(blob))
		}
		var b kzg.Blob
		copy(b[:], blob)
		c, err := kzg.BlobToKZGCommitment(&b)
		if err != nil {
			return errors.Wrapf(err, "compute kzg commitment for blob %d", i)
		}
		commitments[i] = c[:]
	}
	return kzg.VerifyCellKZGProofBatchFromBlobData(blobs, commitments, flatProofs, fieldparams.NumberOfColumns)
}

// PublishSignedExecutionPayloadBid broadcasts a signed execution payload bid to the P2P network.
func (s *Server) PublishSignedExecutionPayloadBid(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.PublishSignedExecutionPayloadBid")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	versionHeader := r.Header.Get(api.VersionHeader)
	if versionHeader == "" {
		httputil.HandleError(w, api.VersionHeader+" header is required", http.StatusBadRequest)
		return
	}

	var signedBid *eth.SignedExecutionPayloadBid
	if httputil.IsRequestSsz(r) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			httputil.HandleError(w, "Could not read request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		signedBid = &eth.SignedExecutionPayloadBid{}
		if err := signedBid.UnmarshalSSZ(body); err != nil {
			httputil.HandleError(w, "Could not unmarshal SSZ: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		var jsonBid structs.SignedExecutionPayloadBid
		if err := json.NewDecoder(r.Body).Decode(&jsonBid); err != nil {
			if errors.Is(err, io.EOF) {
				httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
				return
			}
			httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		var err error
		signedBid, err = jsonBid.ToConsensus()
		if err != nil {
			httputil.HandleError(w, "Could not convert bid to consensus type: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	if err := s.Broadcaster.Broadcast(ctx, signedBid); err != nil {
		httputil.HandleError(w, "Could not broadcast execution payload bid: "+err.Error(), http.StatusInternalServerError)
		return
	}
}
