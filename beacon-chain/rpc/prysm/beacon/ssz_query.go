package beacon

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	sszquerypb "github.com/OffchainLabs/prysm/v6/proto/ssz_query"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
)

// QueryBeaconState handles SSZ Query request for BeaconState.
// Returns as bytes serialized SSZQueryResponse.
func (s *Server) QueryBeaconState(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.QueryBeaconState")
	defer span.End()

	stateID := r.PathValue("state_id")
	if stateID == "" {
		httputil.HandleError(w, "state_id is required in URL params", http.StatusBadRequest)
		return
	}

	// Validate path before lookup: it might be expensive.
	var req structs.SSZQueryRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	switch {
	case errors.Is(err, io.EOF):
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if len(req.Query) == 0 {
		httputil.HandleError(w, "Empty query submitted", http.StatusBadRequest)
		return
	}

	path, err := query.ParsePath(req.Query)
	if err != nil {
		httputil.HandleError(w, "Could not parse path '"+req.Query+"': "+err.Error(), http.StatusBadRequest)
		return
	}

	stateRoot, err := s.Stater.StateRoot(ctx, []byte(stateID))
	if err != nil {
		var rootNotFoundErr *lookup.StateRootNotFoundError
		if errors.As(err, &rootNotFoundErr) {
			httputil.HandleError(w, "State root not found: "+rootNotFoundErr.Error(), http.StatusNotFound)
			return
		}
		httputil.HandleError(w, "Could not get state root: "+err.Error(), http.StatusInternalServerError)
		return
	}

	st, err := s.Stater.State(ctx, []byte(stateID))
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}

	// NOTE: Using unsafe conversion to proto is acceptable here,
	// as we play with a copy of the state returned by Stater.
	sszObject, ok := st.ToProtoUnsafe().(query.SSZObject)
	if !ok {
		httputil.HandleError(w, "Unsupported state version for querying: "+version.String(st.Version()), http.StatusBadRequest)
		return
	}

	info, err := query.AnalyzeObject(sszObject)
	if err != nil {
		httputil.HandleError(w, "Could not analyze state object: "+err.Error(), http.StatusInternalServerError)
		return
	}

	_, offset, length, err := query.CalculateOffsetAndLength(info, path)
	if err != nil {
		httputil.HandleError(w, "Could not calculate offset and length for path '"+req.Query+"': "+err.Error(), http.StatusInternalServerError)
		return
	}

	encodedState, err := st.MarshalSSZ()
	if err != nil {
		httputil.HandleError(w, "Could not marshal state to SSZ: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := &sszquerypb.SSZQueryResponse{
		Root:   stateRoot,
		Result: encodedState[offset : offset+length],
	}

	responseSsz, err := response.MarshalSSZ()
	if err != nil {
		httputil.HandleError(w, "Could not marshal response to SSZ: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(api.VersionHeader, version.String(st.Version()))
	httputil.WriteSsz(w, responseSsz)
}

// QueryBeaconState handles SSZ Query request for BeaconState.
// Returns as bytes serialized SSZQueryResponse.
func (s *Server) QueryBeaconBlock(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.QueryBeaconBlock")
	defer span.End()

	blockId := r.PathValue("block_id")
	if blockId == "" {
		httputil.HandleError(w, "block_id is required in URL params", http.StatusBadRequest)
		return
	}

	// Validate path before lookup: it might be expensive.
	var req structs.SSZQueryRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	switch {
	case errors.Is(err, io.EOF):
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if len(req.Query) == 0 {
		httputil.HandleError(w, "Empty query submitted", http.StatusBadRequest)
		return
	}

	path, err := query.ParsePath(req.Query)
	if err != nil {
		httputil.HandleError(w, "Could not parse path '"+req.Query+"': "+err.Error(), http.StatusBadRequest)
		return
	}

	signedBlock, err := s.Blocker.Block(ctx, []byte(blockId))
	if !shared.WriteBlockFetchError(w, signedBlock, err) {
		return
	}

	protoBlock, err := signedBlock.Block().Proto()
	if err != nil {
		httputil.HandleError(w, "Could not convert block to proto: "+err.Error(), http.StatusInternalServerError)
		return
	}

	block, ok := protoBlock.(query.SSZObject)
	if !ok {
		httputil.HandleError(w, "Unsupported block version for querying: "+version.String(signedBlock.Version()), http.StatusBadRequest)
		return
	}

	info, err := query.AnalyzeObject(block)
	if err != nil {
		httputil.HandleError(w, "Could not analyze block object: "+err.Error(), http.StatusInternalServerError)
		return
	}

	_, offset, length, err := query.CalculateOffsetAndLength(info, path)
	if err != nil {
		httputil.HandleError(w, "Could not calculate offset and length for path '"+req.Query+"': "+err.Error(), http.StatusInternalServerError)
		return
	}

	encodedBlock, err := signedBlock.Block().MarshalSSZ()
	if err != nil {
		httputil.HandleError(w, "Could not marshal block to SSZ: "+err.Error(), http.StatusInternalServerError)
		return
	}

	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "Could not compute block root: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := &sszquerypb.SSZQueryResponse{
		Root:   blockRoot[:],
		Result: encodedBlock[offset : offset+length],
	}

	responseSsz, err := response.MarshalSSZ()
	if err != nil {
		httputil.HandleError(w, "Could not marshal response to SSZ: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(api.VersionHeader, version.String(signedBlock.Version()))
	httputil.WriteSsz(w, responseSsz)
}
