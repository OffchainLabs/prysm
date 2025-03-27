package beacon

import (
	"context"
	"fmt"
	"net/http"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
)

// GetExecutionPayloadV1 retrieves execution payload envelope details for given block ID.
func (s *Server) GetExecutionPayloadV1(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetExecutionPayloadV1")
	defer span.End()

	blockId := r.PathValue("block_id")
	if blockId == "" {
		httputil.HandleError(w, "block_id is required in URL params", http.StatusBadRequest)
		return
	}
	signed, err := s.Blocker.Payload(ctx, []byte(blockId))
	if !writePayloadFetchError(w, signed, err) {
		return
	}

	if httputil.RespondWithSsz(r) {
		s.getExecutionPayloadV1Ssz(w, signed)
	} else {
		s.getExecutionPayloadV1Json(ctx, w, signed)
	}
}

// getExecutionPayloadV1Ssz returns the SSZ-serialized version of the execution payload for given block ID.
func (s *Server) getExecutionPayloadV1Ssz(w http.ResponseWriter, signed interfaces.ROSignedExecutionPayloadEnvelope) {
	result, err := s.getExecutionPayloadResponseBodySsz(signed)
	if err != nil {
		httputil.HandleError(w, "Could not get signed execution payload envelope: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if result == nil {
		httputil.HandleError(w, fmt.Sprintf("Unknown execution payload envelope type %T", signed), http.StatusInternalServerError)
		return
	}
	w.Header().Set(api.VersionHeader, version.String(version.EPBS))
	httputil.WriteSsz(w, result)
}

func (*Server) getExecutionPayloadResponseBodySsz(signed interfaces.ROSignedExecutionPayloadEnvelope) ([]byte, error) {
	if signed.IsNil() {
		return nil, errNilBlock
	}
	pb := signed.Proto()
	marshaler, ok := pb.(ssz.Marshaler)
	if !ok {
		return nil, errMarshalSSZ
	}
	sszData, err := marshaler.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrapf(err, "could not marshal payload envelope into SSZ")
	}
	return sszData, nil
}

// getExecutionPayloadV1Json returns the JSON-serialized version of the execution payload envelope for given block ID.
func (s *Server) getExecutionPayloadV1Json(ctx context.Context, w http.ResponseWriter, signed interfaces.ROSignedExecutionPayloadEnvelope) {
	result, err := s.getExecutionPayloadResponseBodyJson(ctx, signed)
	if err != nil {
		httputil.HandleError(w, "Error processing request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if result == nil {
		httputil.HandleError(w, fmt.Sprintf("Unknown block type %T", signed), http.StatusInternalServerError)
		return
	}
	w.Header().Set(api.VersionHeader, result.Version)
	httputil.WriteJson(w, result)
}

func (s *Server) getExecutionPayloadResponseBodyJson(ctx context.Context, signed interfaces.ROSignedExecutionPayloadEnvelope) (*structs.GetExecutionPayloadV1Response, error) {
	if signed.IsNil() {
		return nil, errNilPayload
	}
	pb, ok := signed.Proto().(*enginev1.SignedExecutionPayloadEnvelope)
	if !ok {
		return nil, errors.New("could not cast to signed execution payload envelope")
	}
	payload, err := structs.SignedExecutionPayloadEnvelopeFromConsensus(pb)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert to signed execution payload envelope")
	}
	envelope, err := signed.Envelope()
	if err != nil {
		return nil, errors.Wrap(err, "could not get execution payload envelope")
	}
	blkRoot := envelope.BeaconBlockRoot()
	finalized := s.FinalizationFetcher.IsFinalized(ctx, blkRoot)
	isOptimistic, err := s.OptimisticModeFetcher.IsOptimisticForRoot(ctx, blkRoot)
	if err != nil {
		return nil, errors.Wrap(err, "could not check if block is optimistic")
	}

	return &structs.GetExecutionPayloadV1Response{
		Version:             version.String(version.EPBS),
		ExecutionOptimistic: isOptimistic,
		Finalized:           finalized,
		Data:                payload,
	}, nil
}

// writePayloadFetchError writes an appropriate error based on the supplied argument.
// The argument error should be a result of fetching an execution payload
func writePayloadFetchError(w http.ResponseWriter, signed interfaces.ROSignedExecutionPayloadEnvelope, err error) bool {
	var invalidBlockIdErr *lookup.BlockIdParseError
	if errors.As(err, &invalidBlockIdErr) {
		httputil.HandleError(w, "Invalid block ID: "+invalidBlockIdErr.Error(), http.StatusBadRequest)
		return false
	}
	if err != nil {
		httputil.HandleError(w, "Could not get payload from block ID: "+err.Error(), http.StatusInternalServerError)
		return false
	}
	if signed.IsNil() {
		httputil.HandleError(w, "Could not find requested execution payload: ", http.StatusNotFound)
		return false
	}
	return true
}
