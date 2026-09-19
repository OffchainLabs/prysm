package beacon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
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

// PublishExecutionPayloadEnvelope broadcasts a signed envelope. Eth-Blob-Data-Included selects the
// body: true=contents with blobs+proofs, false=bare signed envelope (BN attaches cached blob data).
func (s *Server) PublishExecutionPayloadEnvelope(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.PublishExecutionPayloadEnvelope")
	defer span.End()
	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}
	versionHeader := r.Header.Get(api.VersionHeader)
	if versionHeader == "" {
		httputil.HandleError(w, api.VersionHeader+" header is required", http.StatusBadRequest)
		return
	}
	v, err := version.FromString(versionHeader)
	if err != nil || v < version.Gloas {
		httputil.HandleError(w, api.VersionHeader+" header must be gloas or later", http.StatusBadRequest)
		return
	}
	blobDataHeader := r.Header.Get(api.BlobDataIncludedHeader)
	if blobDataHeader == "" {
		httputil.HandleError(w, api.BlobDataIncludedHeader+" header is required", http.StatusBadRequest)
		return
	}
	blobDataIncluded, err := strconv.ParseBool(blobDataHeader)
	if err != nil {
		httputil.HandleError(w, "invalid "+api.BlobDataIncludedHeader+" value: "+err.Error(), http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		httputil.HandleError(w, "could not read request body: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if blobDataIncluded {
		s.publishEnvelopeContents(ctx, w, r, body)
		return
	}
	s.publishBareEnvelope(ctx, w, r, body)
}

// publishBareEnvelope handles the stateful flow (header=false): the body carries only the signed
// envelope and the BN attaches the blobs and KZG proofs cached at block production.
func (s *Server) publishBareEnvelope(ctx context.Context, w http.ResponseWriter, r *http.Request, body []byte) {
	signed := &eth.SignedExecutionPayloadEnvelope{}
	if httputil.IsRequestSsz(r) {
		if err := signed.UnmarshalSSZ(body); err != nil {
			httputil.HandleError(w, "could not decode SSZ envelope: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		var jsonEnvelope structs.SignedExecutionPayloadEnvelope
		if err := json.Unmarshal(body, &jsonEnvelope); err != nil {
			httputil.HandleError(w, "could not decode JSON envelope: "+err.Error(), http.StatusBadRequest)
			return
		}
		consensus, err := jsonEnvelope.ToConsensus()
		if err != nil {
			httputil.HandleError(w, "invalid signed envelope: "+err.Error(), http.StatusBadRequest)
			return
		}
		signed = consensus
	}
	if signed.Message == nil || signed.Message.Payload == nil {
		httputil.HandleError(w, "envelope message or payload is nil", http.StatusBadRequest)
		return
	}

	if !s.validateEnvelopeBroadcast(ctx, w, r, signed) {
		return
	}

	// The cached-envelope match (and the cache-miss 400) happens in the v1alpha1 server, shared
	// with the gRPC path.
	generic := &eth.GenericSignedExecutionPayloadEnvelope{
		Envelope: &eth.GenericSignedExecutionPayloadEnvelope_SignedEnvelope{SignedEnvelope: signed},
	}
	if _, err := s.V1Alpha1ValidatorServer.PublishExecutionPayloadEnvelope(ctx, generic); err != nil {
		writeEnvelopePublishError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// writeEnvelopePublishError maps the v1alpha1 publish outcome to the spec status codes,
// InvalidArgument/FailedPrecondition -> 400.
func writeEnvelopePublishError(w http.ResponseWriter, err error) {
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.InvalidArgument, codes.FailedPrecondition:
			httputil.HandleError(w, st.Message(), http.StatusBadRequest)
		default:
			httputil.HandleError(w, st.Message(), http.StatusInternalServerError)
		}
		return
	}
	httputil.HandleError(w, "could not publish execution payload envelope: "+err.Error(), http.StatusInternalServerError)
}

// publishEnvelopeContents handles the stateless flow (header=true).
func (s *Server) publishEnvelopeContents(ctx context.Context, w http.ResponseWriter, r *http.Request, body []byte) {
	if httputil.IsRequestSsz(r) {
		contents := &eth.SignedExecutionPayloadEnvelopeContents{}
		if err := contents.UnmarshalSSZ(body); err != nil {
			httputil.HandleError(w, "could not decode SSZ envelope contents: "+err.Error(), http.StatusBadRequest)
			return
		}
		s.publishExecutionPayloadEnvelopeContentsSSZ(ctx, w, r, contents)
		return
	}
	s.publishExecutionPayloadEnvelopeContents(ctx, w, r, body)
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

// processEnvelopeContents delegates the signed envelope + blobs/proofs to the v1alpha1 publish path,
// which verifies the blobs, broadcasts the data column sidecars, and publishes the envelope.
func (s *Server) processEnvelopeContents(ctx context.Context, w http.ResponseWriter, r *http.Request, signed *eth.SignedExecutionPayloadEnvelope, kzgProofs, blobs [][]byte) {
	if !s.validateEnvelopeBroadcast(ctx, w, r, signed) {
		return
	}

	generic := &eth.GenericSignedExecutionPayloadEnvelope{
		Envelope: &eth.GenericSignedExecutionPayloadEnvelope_Contents{
			Contents: &eth.SignedExecutionPayloadEnvelopeContents{
				SignedExecutionPayloadEnvelope: signed,
				KzgProofs:                      kzgProofs,
				Blobs:                          blobs,
			},
		},
	}
	if _, err := s.V1Alpha1ValidatorServer.PublishExecutionPayloadEnvelope(ctx, generic); err != nil {
		writeEnvelopePublishError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Gossip level broadcasts unconditionally, builders may reveal before this node has seen the block and peers queue such envelopes.
func (s *Server) validateEnvelopeBroadcast(ctx context.Context, w http.ResponseWriter, r *http.Request, signed *eth.SignedExecutionPayloadEnvelope) bool {
	level := r.URL.Query().Get(broadcastValidationQueryParam)
	switch level {
	case "", broadcastValidationGossip:
		return true
	case broadcastValidationConsensus, broadcastValidationConsensusAndEquivocation:
	default:
		httputil.HandleError(w, fmt.Sprintf("invalid %s value: %q", broadcastValidationQueryParam, level), http.StatusBadRequest)
		return false
	}

	envSlot := signed.Message.Payload.SlotNumber
	envRoot := bytesutil.ToBytes32(signed.Message.BeaconBlockRoot)

	if level == broadcastValidationConsensusAndEquivocation {
		// CanonicalNodeAtSlot's bool means "payload full", not "node found" — at the
		// wall clock slot it is always false. A non-zero root is the found signal.
		canonRoot, _ := s.ForkchoiceFetcher.CanonicalNodeAtSlot(envSlot)
		if canonRoot != ([32]byte{}) && canonRoot != envRoot {
			err := errors.Wrapf(errEquivocatedBlock, "another block for slot %d already exists in fork choice", envSlot)
			httputil.HandleError(w, err.Error(), http.StatusBadRequest)
			return false
		}
	}

	// Submission path: envelope must be for the current head.
	headRoot, err := s.HeadFetcher.HeadRoot(ctx)
	if err != nil {
		httputil.HandleError(w, "could not get head root: "+err.Error(), http.StatusInternalServerError)
		return false
	}
	if !bytes.Equal(headRoot, envRoot[:]) {
		httputil.HandleError(w, fmt.Sprintf("envelope beacon block root %#x is not canonical head", envRoot), http.StatusBadRequest)
		return false
	}
	st, err := s.HeadFetcher.HeadState(ctx)
	if err != nil {
		httputil.HandleError(w, "could not get head state: "+err.Error(), http.StatusInternalServerError)
		return false
	}
	roSigned, err := consensusblocks.WrappedROSignedExecutionPayloadEnvelope(signed)
	if err != nil {
		httputil.HandleError(w, "could not wrap signed envelope: "+err.Error(), http.StatusInternalServerError)
		return false
	}
	if err := gloas.VerifyExecutionPayloadEnvelope(ctx, st, roSigned); err != nil {
		httputil.HandleError(w, "consensus validation failed: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
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

	// Delegate to the v1alpha1 server, which verifies the bid with the gossip rules, records it in
	// the local highest-bid cache, broadcasts it, and emits the operation feed event.
	if _, err := s.V1Alpha1ValidatorServer.SubmitSignedExecutionPayloadBid(ctx, signedBid); err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.InvalidArgument:
				httputil.HandleError(w, st.Message(), http.StatusBadRequest)
			case codes.FailedPrecondition:
				httputil.HandleError(w, st.Message(), http.StatusBadRequest)
			case codes.Unavailable:
				httputil.HandleError(w, st.Message(), http.StatusServiceUnavailable)
			default:
				httputil.HandleError(w, st.Message(), http.StatusInternalServerError)
			}
			return
		}
		httputil.HandleError(w, "Could not submit execution payload bid: "+err.Error(), http.StatusInternalServerError)
	}
}
