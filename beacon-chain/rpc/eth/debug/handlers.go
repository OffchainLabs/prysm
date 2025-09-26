package debug

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

const (
	errMsgStateFromConsensus = "Could not convert consensus state to response"
)

// GetBeaconStateV2 returns the full beacon state for a given state ID.
func (s *Server) GetBeaconStateV2(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "debug.GetBeaconStateV2")
	defer span.End()

	stateId := r.PathValue("state_id")
	if stateId == "" {
		httputil.HandleError(w, "state_id is required in URL params", http.StatusBadRequest)
		return
	}

	if httputil.RespondWithSsz(r) {
		s.getBeaconStateSSZV2(ctx, w, []byte(stateId))
	} else {
		s.getBeaconStateV2(ctx, w, []byte(stateId))
	}
}

// getBeaconStateV2 returns the JSON-serialized version of the full beacon state object for given state ID.
func (s *Server) getBeaconStateV2(ctx context.Context, w http.ResponseWriter, id []byte) {
	st, err := s.Stater.State(ctx, id)
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}

	isOptimistic, err := helpers.IsOptimistic(ctx, id, s.OptimisticModeFetcher, s.Stater, s.ChainInfoFetcher, s.BeaconDB)
	if err != nil {
		helpers.HandleIsOptimisticError(w, err)
		return
	}
	blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "Could not calculate root of latest block header: "+err.Error(), http.StatusInternalServerError)
		return
	}
	isFinalized := s.FinalizationFetcher.IsFinalized(ctx, blockRoot)
	var respSt interface{}

	switch st.Version() {
	case version.Phase0:
		respSt, err = structs.BeaconStateFromConsensus(st)
		if err != nil {
			httputil.HandleError(w, errMsgStateFromConsensus+": "+err.Error(), http.StatusInternalServerError)
			return
		}
	case version.Altair:
		respSt, err = structs.BeaconStateAltairFromConsensus(st)
		if err != nil {
			httputil.HandleError(w, errMsgStateFromConsensus+": "+err.Error(), http.StatusInternalServerError)
			return
		}
	case version.Bellatrix:
		respSt, err = structs.BeaconStateBellatrixFromConsensus(st)
		if err != nil {
			httputil.HandleError(w, errMsgStateFromConsensus+": "+err.Error(), http.StatusInternalServerError)
			return
		}
	case version.Capella:
		respSt, err = structs.BeaconStateCapellaFromConsensus(st)
		if err != nil {
			httputil.HandleError(w, errMsgStateFromConsensus+": "+err.Error(), http.StatusInternalServerError)
			return
		}
	case version.Deneb:
		respSt, err = structs.BeaconStateDenebFromConsensus(st)
		if err != nil {
			httputil.HandleError(w, errMsgStateFromConsensus+": "+err.Error(), http.StatusInternalServerError)
			return
		}
	case version.Electra:
		respSt, err = structs.BeaconStateElectraFromConsensus(st)
		if err != nil {
			httputil.HandleError(w, errMsgStateFromConsensus+": "+err.Error(), http.StatusInternalServerError)
			return
		}
	case version.Fulu:
		respSt, err = structs.BeaconStateFuluFromConsensus(st)
		if err != nil {
			httputil.HandleError(w, errMsgStateFromConsensus+": "+err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		httputil.HandleError(w, "Unsupported state version", http.StatusInternalServerError)
		return
	}

	jsonBytes, err := json.Marshal(respSt)
	if err != nil {
		httputil.HandleError(w, "Could not marshal state into JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}
	ver := version.String(st.Version())
	resp := &structs.GetBeaconStateV2Response{
		Version:             ver,
		ExecutionOptimistic: isOptimistic,
		Finalized:           isFinalized,
		Data:                jsonBytes,
	}
	w.Header().Set(api.VersionHeader, ver)
	httputil.WriteJson(w, resp)
}

// getBeaconStateSSZV2 returns the SSZ-serialized version of the full beacon state object for given state ID.
func (s *Server) getBeaconStateSSZV2(ctx context.Context, w http.ResponseWriter, id []byte) {
	st, err := s.Stater.State(ctx, id)
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}
	sszState, err := st.MarshalSSZ()
	if err != nil {
		httputil.HandleError(w, "Could not marshal state into SSZ: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(api.VersionHeader, version.String(st.Version()))
	httputil.WriteSsz(w, sszState)
}

// GetForkChoiceHeadsV2 retrieves the leaves of the current fork choice tree.
func (s *Server) GetForkChoiceHeadsV2(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "debug.GetForkChoiceHeadsV2")
	defer span.End()

	headRoots, headSlots := s.HeadFetcher.ChainHeads()
	resp := &structs.GetForkChoiceHeadsV2Response{
		Data: make([]*structs.ForkChoiceHead, len(headRoots)),
	}
	for i := range headRoots {
		isOptimistic, err := s.OptimisticModeFetcher.IsOptimisticForRoot(ctx, headRoots[i])
		if err != nil {
			httputil.HandleError(w, "Could not check if head is optimistic: "+err.Error(), http.StatusInternalServerError)
			return
		}
		resp.Data[i] = &structs.ForkChoiceHead{
			Root:                hexutil.Encode(headRoots[i][:]),
			Slot:                fmt.Sprintf("%d", headSlots[i]),
			ExecutionOptimistic: isOptimistic,
		}
	}

	httputil.WriteJson(w, resp)
}

// GetForkChoice returns a dump fork choice store.
func (s *Server) GetForkChoice(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "debug.GetForkChoice")
	defer span.End()

	dump, err := s.ForkchoiceFetcher.ForkChoiceDump(ctx)
	if err != nil {
		httputil.HandleError(w, "Could not get forkchoice dump: "+err.Error(), http.StatusInternalServerError)
		return
	}

	nodes := make([]*structs.ForkChoiceNode, len(dump.ForkChoiceNodes))
	for i, n := range dump.ForkChoiceNodes {
		nodes[i] = &structs.ForkChoiceNode{
			Slot:               fmt.Sprintf("%d", n.Slot),
			BlockRoot:          hexutil.Encode(n.BlockRoot),
			ParentRoot:         hexutil.Encode(n.ParentRoot),
			JustifiedEpoch:     fmt.Sprintf("%d", n.JustifiedEpoch),
			FinalizedEpoch:     fmt.Sprintf("%d", n.FinalizedEpoch),
			Weight:             fmt.Sprintf("%d", n.Weight),
			ExecutionBlockHash: hexutil.Encode(n.ExecutionBlockHash),
			Validity:           n.Validity.String(),
			ExtraData: &structs.ForkChoiceNodeExtraData{
				UnrealizedJustifiedEpoch: fmt.Sprintf("%d", n.UnrealizedJustifiedEpoch),
				UnrealizedFinalizedEpoch: fmt.Sprintf("%d", n.UnrealizedFinalizedEpoch),
				Balance:                  fmt.Sprintf("%d", n.Balance),
				ExecutionOptimistic:      n.ExecutionOptimistic,
				TimeStamp:                n.Timestamp.String(),
				Target:                   fmt.Sprintf("%#x", n.Target),
			},
		}
	}
	resp := &structs.GetForkChoiceDumpResponse{
		JustifiedCheckpoint: structs.CheckpointFromConsensus(dump.JustifiedCheckpoint),
		FinalizedCheckpoint: structs.CheckpointFromConsensus(dump.FinalizedCheckpoint),
		ForkChoiceNodes:     nodes,
		ExtraData: &structs.ForkChoiceDumpExtraData{
			UnrealizedJustifiedCheckpoint: structs.CheckpointFromConsensus(dump.UnrealizedJustifiedCheckpoint),
			UnrealizedFinalizedCheckpoint: structs.CheckpointFromConsensus(dump.UnrealizedFinalizedCheckpoint),
			ProposerBoostRoot:             hexutil.Encode(dump.ProposerBoostRoot),
			PreviousProposerBoostRoot:     hexutil.Encode(dump.PreviousProposerBoostRoot),
			HeadRoot:                      hexutil.Encode(dump.HeadRoot),
		},
	}
	httputil.WriteJson(w, resp)
}

// DataColumnSidecars retrieves data column sidecars for a given block id.
func (s *Server) DataColumnSidecars(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "debug.DataColumnSidecars")
	defer span.End()

	// Check if we're before Fulu fork - data columns are only available from Fulu onwards
	fuluForkEpoch := params.BeaconConfig().FuluForkEpoch
	if fuluForkEpoch == math.MaxUint64 {
		httputil.HandleError(w, "Data columns are not supported - Fulu fork not configured", http.StatusBadRequest)
		return
	}

	// Check if we're before Fulu fork based on current slot
	currentSlot := s.GenesisTimeFetcher.CurrentSlot()
	currentEpoch := primitives.Epoch(currentSlot / params.BeaconConfig().SlotsPerEpoch)
	if currentEpoch < fuluForkEpoch {
		httputil.HandleError(w, "Data columns are not supported - before Fulu fork", http.StatusBadRequest)
		return
	}

	indices, err := parseDataColumnIndices(r.URL)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}
	segments := strings.Split(r.URL.Path, "/")
	blockId := segments[len(segments)-1]

	verifiedDataColumns, rpcErr := s.Blocker.DataColumns(ctx, blockId, indices)
	if rpcErr != nil {
		code := core.ErrorReasonToHTTP(rpcErr.Reason)
		switch code {
		case http.StatusBadRequest:
			httputil.HandleError(w, "Bad request: "+rpcErr.Err.Error(), code)
			return
		case http.StatusNotFound:
			httputil.HandleError(w, "Not found: "+rpcErr.Err.Error(), code)
			return
		case http.StatusInternalServerError:
			httputil.HandleError(w, "Internal server error: "+rpcErr.Err.Error(), code)
			return
		default:
			httputil.HandleError(w, rpcErr.Err.Error(), code)
			return
		}
	}

	blk, err := s.Blocker.Block(ctx, []byte(blockId))
	if !shared.WriteBlockFetchError(w, blk, err) {
		return
	}

	if httputil.RespondWithSsz(r) {
		sszResp, err := buildDataColumnSidecarsSSZResponse(verifiedDataColumns)
		if err != nil {
			httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(api.VersionHeader, version.String(blk.Version()))
		httputil.WriteSsz(w, sszResp)
		return
	}

	blkRoot, err := blk.Block().HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "Could not hash block: "+err.Error(), http.StatusInternalServerError)
		return
	}
	isOptimistic, err := s.OptimisticModeFetcher.IsOptimisticForRoot(ctx, blkRoot)
	if err != nil {
		httputil.HandleError(w, "Could not check if block is optimistic: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := buildDataColumnSidecarsJsonResponse(verifiedDataColumns)
	resp := &structs.GetDebugDataColumnSidecarsResponse{
		Version:             version.String(blk.Version()),
		Data:                data,
		ExecutionOptimistic: isOptimistic,
		Finalized:           s.FinalizationFetcher.IsFinalized(ctx, blkRoot),
	}
	w.Header().Set(api.VersionHeader, version.String(blk.Version()))
	httputil.WriteJson(w, resp)
}

// parseDataColumnIndices filters out invalid and duplicate data column indices
func parseDataColumnIndices(url *url.URL) ([]int, error) {
	numberOfColumns := params.BeaconConfig().NumberOfColumns
	rawIndices := url.Query()["indices"]
	indices := make([]int, 0, numberOfColumns)
	invalidIndices := make([]string, 0)
loop:
	for _, raw := range rawIndices {
		ix, err := strconv.Atoi(raw)
		if err != nil {
			invalidIndices = append(invalidIndices, raw)
			continue
		}
		if !(0 <= ix && uint64(ix) < numberOfColumns) {
			invalidIndices = append(invalidIndices, raw)
			continue
		}
		for i := range indices {
			if ix == indices[i] {
				continue loop
			}
		}
		indices = append(indices, ix)
	}

	if len(invalidIndices) > 0 {
		return nil, fmt.Errorf("requested data column indices %v are invalid", invalidIndices)
	}
	return indices, nil
}

func buildDataColumnSidecarsJsonResponse(verifiedDataColumns []blocks.VerifiedRODataColumn) []*structs.DataColumnSidecar {
	sidecars := make([]*structs.DataColumnSidecar, len(verifiedDataColumns))
	for i, dc := range verifiedDataColumns {
		column := make([]string, len(dc.Column))
		for j, cell := range dc.Column {
			column[j] = hexutil.Encode(cell)
		}

		kzgCommitments := make([]string, len(dc.KzgCommitments))
		for j, commitment := range dc.KzgCommitments {
			kzgCommitments[j] = hexutil.Encode(commitment)
		}

		kzgProofs := make([]string, len(dc.KzgProofs))
		for j, proof := range dc.KzgProofs {
			kzgProofs[j] = hexutil.Encode(proof)
		}

		kzgCommitmentsInclusionProof := make([]string, len(dc.KzgCommitmentsInclusionProof))
		for j, proof := range dc.KzgCommitmentsInclusionProof {
			kzgCommitmentsInclusionProof[j] = hexutil.Encode(proof)
		}

		sidecars[i] = &structs.DataColumnSidecar{
			Index:                        strconv.FormatUint(dc.Index, 10),
			Column:                       column,
			KzgCommitments:               kzgCommitments,
			KzgProofs:                    kzgProofs,
			SignedBeaconBlockHeader:      structs.SignedBeaconBlockHeaderFromConsensus(dc.SignedBlockHeader),
			KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
		}
	}
	return sidecars
}

// buildDataColumnSidecarsSSZResponse builds SSZ response for data column sidecars
func buildDataColumnSidecarsSSZResponse(verifiedDataColumns []blocks.VerifiedRODataColumn) ([]byte, error) {
	if len(verifiedDataColumns) == 0 {
		return []byte{}, nil
	}

	// Pre-allocate buffer for all sidecars using the known SSZ size
	sizePerSidecar := (&ethpb.DataColumnSidecar{}).SizeSSZ()
	ssz := make([]byte, 0, sizePerSidecar*len(verifiedDataColumns))

	// Marshal and append each sidecar
	for i, sidecar := range verifiedDataColumns {
		sszrep, err := sidecar.MarshalSSZ()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal data column sidecar at index %d", i)
		}
		ssz = append(ssz, sszrep...)
	}

	return ssz, nil
}
