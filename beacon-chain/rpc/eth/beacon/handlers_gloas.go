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
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
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

// PublishExecutionPayloadEnvelope broadcasts a signed envelope. Eth-Execution-Payload-Blinded
// selects the body: true=blinded (stateful, BN reconstructs from cache), false=contents (stateless).
// Endpoint: POST /eth/v1/beacon/execution_payload_envelopes
func (s *Server) PublishExecutionPayloadEnvelope(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.PublishExecutionPayloadEnvelope")
	defer span.End()
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
	blindedHeader := r.Header.Get(api.ExecutionPayloadBlindedHeader)
	if blindedHeader == "" {
		httputil.HandleError(w, api.ExecutionPayloadBlindedHeader+" header is required", http.StatusBadRequest)
		return
	}
	isBlinded, err := strconv.ParseBool(blindedHeader)
	if err != nil {
		httputil.HandleError(w, "invalid "+api.ExecutionPayloadBlindedHeader+" value: "+err.Error(), http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		httputil.HandleError(w, "could not read request body: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if isBlinded {
		s.publishBlindedEnvelope(ctx, w, r, body)
		return
	}
	s.publishEnvelopeContents(ctx, w, r, body)
}

// publishBlindedEnvelope reconstructs the full envelope from cache by beacon_block_root.
// HTR(blinded) == HTR(full), so the validator signature stays valid.
func (s *Server) publishBlindedEnvelope(ctx context.Context, w http.ResponseWriter, r *http.Request, body []byte) {
	signedBlinded := &eth.SignedWireBlindedExecutionPayloadEnvelope{}
	if httputil.IsRequestSsz(r) {
		if err := signedBlinded.UnmarshalSSZ(body); err != nil {
			httputil.HandleError(w, "could not decode SSZ blinded envelope: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		var jsonBlinded structs.SignedBlindedExecutionPayloadEnvelope
		if err := json.Unmarshal(body, &jsonBlinded); err != nil {
			httputil.HandleError(w, "could not decode JSON blinded envelope: "+err.Error(), http.StatusBadRequest)
			return
		}
		consensus, err := jsonBlinded.ToConsensus()
		if err != nil {
			httputil.HandleError(w, "invalid signed blinded envelope: "+err.Error(), http.StatusBadRequest)
			return
		}
		signedBlinded = consensus
	}
	if signedBlinded.Message == nil {
		httputil.HandleError(w, "blinded envelope message is nil", http.StatusBadRequest)
		return
	}

	cached, ok := s.ExecutionPayloadEnvelopeCache.Contents()
	if !ok || cached.Envelope == nil {
		httputil.HandleError(w, "no cached execution payload envelope to reconstruct from", http.StatusBadRequest)
		return
	}
	if !bytes.Equal(cached.Envelope.BeaconBlockRoot, signedBlinded.Message.BeaconBlockRoot) {
		httputil.HandleError(w, "cached envelope beacon_block_root does not match blinded envelope", http.StatusBadRequest)
		return
	}
	blindedRoot, err := signedBlinded.Message.HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "could not hash blinded envelope: "+err.Error(), http.StatusInternalServerError)
		return
	}
	cachedRoot, err := cached.Envelope.HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "could not hash cached envelope: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if blindedRoot != cachedRoot {
		httputil.HandleError(w, "cached envelope hash tree root does not match blinded envelope", http.StatusBadRequest)
		return
	}

	full := &eth.SignedExecutionPayloadEnvelope{
		Message:   cached.Envelope,
		Signature: signedBlinded.Signature,
	}

	if !s.validateEnvelopeBroadcast(ctx, w, r, full) {
		return
	}

	if _, err := s.V1Alpha1ValidatorServer.PublishExecutionPayloadEnvelope(ctx, full); err != nil {
		writeEnvelopePublishError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// writeEnvelopePublishError maps the v1alpha1 publish outcome to the spec status
// codes: InvalidArgument -> 400, Aborted -> 202 (broadcast ok, import failed).
func writeEnvelopePublishError(w http.ResponseWriter, err error) {
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.InvalidArgument:
			httputil.HandleError(w, st.Message(), http.StatusBadRequest)
		case codes.Aborted:
			httputil.HandleError(w, st.Message(), http.StatusAccepted)
		default:
			httputil.HandleError(w, st.Message(), http.StatusInternalServerError)
		}
		return
	}
	httputil.HandleError(w, "could not publish execution payload envelope: "+err.Error(), http.StatusInternalServerError)
}

// publishEnvelopeContents handles the stateless flow (header=false).
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

// processEnvelopeContents verifies caller-supplied blobs/proofs, broadcasts
// derived sidecars, then delegates the envelope to the bare publish path.
func (s *Server) processEnvelopeContents(ctx context.Context, w http.ResponseWriter, r *http.Request, signed *eth.SignedExecutionPayloadEnvelope, kzgProofs, blobs [][]byte) {
	if !s.validateEnvelopeBroadcast(ctx, w, r, signed) {
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
		if err := s.Broadcaster.BroadcastDataColumnSidecars(ctx, verifiedSidecars, nil); err != nil {
			httputil.HandleError(w, "could not broadcast data column sidecars: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := s.DataColumnReceiver.ReceiveDataColumns(verifiedSidecars); err != nil {
			httputil.HandleError(w, "could not receive data column sidecars: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if _, err := s.V1Alpha1ValidatorServer.PublishExecutionPayloadEnvelope(ctx, signed); err != nil {
		writeEnvelopePublishError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// validateEnvelopeBroadcast applies broadcast_validation semantics to an
// envelope publish before it is broadcast to gossip. Spec: beacon-APIs #580.
// Writes the HTTP error and returns false on failure: 400 for validation
// failures, 500 for internal errors.
//   - gossip (default): the REJECT-class gossip checks (slot, bid consistency,
//     builder signature) against the envelope's beacon block.
//   - consensus: full envelope consensus checks against the head state. Submission
//     path requires envRoot to equal head.
//   - consensus_and_equivocation: consensus + reject if a different beacon
//     block at the envelope's slot has already been received.
func (s *Server) validateEnvelopeBroadcast(ctx context.Context, w http.ResponseWriter, r *http.Request, signed *eth.SignedExecutionPayloadEnvelope) bool {
	level := r.URL.Query().Get(broadcastValidationQueryParam)
	switch level {
	case "", broadcastValidationGossip:
		return s.validateEnvelopeGossip(ctx, w, signed)
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

// validateEnvelopeGossip runs the REJECT-class gossip checks; p2p-only IGNORE checks are skipped.
// TODO: share orchestration with sync.validateExecutionPayloadEnvelope so check inputs can't drift.
func (s *Server) validateEnvelopeGossip(ctx context.Context, w http.ResponseWriter, signed *eth.SignedExecutionPayloadEnvelope) bool {
	roSigned, err := consensusblocks.WrappedROSignedExecutionPayloadEnvelope(signed)
	if err != nil {
		httputil.HandleError(w, "invalid signed envelope: "+err.Error(), http.StatusBadRequest)
		return false
	}
	v := s.PayloadEnvelopeVerifier(roSigned, verification.GossipExecutionPayloadEnvelopeRequirements)

	blk, err := s.Blocker.Block(ctx, signed.Message.BeaconBlockRoot)
	if err != nil || blk == nil {
		httputil.HandleError(w, "gossip validation failed: envelope beacon block root is unknown", http.StatusBadRequest)
		return false
	}
	// VerifyBlockRootValid is skipped: the bad-block cache is sync-only and a bad root can't be canonical.
	if err := v.VerifySlotMatchesBlock(blk.Block().Slot()); err != nil {
		httputil.HandleError(w, "gossip validation failed: "+err.Error(), http.StatusBadRequest)
		return false
	}

	signedBid, err := blk.Block().Body().SignedExecutionPayloadBid()
	if err != nil {
		httputil.HandleError(w, "gossip validation failed: block has no execution payload bid: "+err.Error(), http.StatusBadRequest)
		return false
	}
	wrappedBid, err := consensusblocks.WrappedROSignedExecutionPayloadBid(signedBid)
	if err != nil {
		httputil.HandleError(w, "could not wrap signed bid: "+err.Error(), http.StatusInternalServerError)
		return false
	}
	bid, err := wrappedBid.Bid()
	if err != nil {
		httputil.HandleError(w, "could not get bid: "+err.Error(), http.StatusInternalServerError)
		return false
	}
	if err := v.VerifyBuilderValid(bid); err != nil {
		httputil.HandleError(w, "gossip validation failed: "+err.Error(), http.StatusBadRequest)
		return false
	}
	if err := v.VerifyPayloadHash(bid); err != nil {
		httputil.HandleError(w, "gossip validation failed: "+err.Error(), http.StatusBadRequest)
		return false
	}
	if err := v.VerifyExecutionRequestsRoot(bid); err != nil {
		httputil.HandleError(w, "gossip validation failed: "+err.Error(), http.StatusBadRequest)
		return false
	}

	// VerifySignature needs the state at the envelope's block. We only have head state on
	// hand, so require the envelope to be for the canonical head rather than replaying state.
	headRoot, err := s.HeadFetcher.HeadRoot(ctx)
	if err != nil {
		httputil.HandleError(w, "could not get head root: "+err.Error(), http.StatusInternalServerError)
		return false
	}
	if !bytes.Equal(headRoot, signed.Message.BeaconBlockRoot) {
		httputil.HandleError(w, "gossip validation failed: envelope beacon block root is not canonical head", http.StatusBadRequest)
		return false
	}
	// Read-only head state is the cheapest option (no copy). It only goes wrong on a long fork
	// where head diverges from the envelope's validator index position — an edge case we don't
	// support. Replaying the block's state would be correct but expensive; this endpoint is
	// trusted (attackers can't reach it, worst case is a self-inflicted DoS), so the cheap path
	// is fine for now. Worth revisiting — replay could also return a read-only state to skip the copy.
	st, err := s.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		httputil.HandleError(w, "could not get head state: "+err.Error(), http.StatusInternalServerError)
		return false
	}
	if err := v.VerifySignature(ctx, st); err != nil {
		httputil.HandleError(w, "gossip validation failed: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
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
