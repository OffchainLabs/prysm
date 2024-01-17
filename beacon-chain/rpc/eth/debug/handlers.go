package debug

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/mux"
	"github.com/prysmaticlabs/prysm/v5/api"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/shared"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"go.opencensus.io/trace"
)

const errMsgStateFromConsensus = "Could not convert consensus state to response"

// GetBeaconStateV2 returns the full beacon state for a given state ID.
func (s *Server) GetBeaconStateV2(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "debug.GetBeaconStateV2")
	defer span.End()

	stateId := mux.Vars(r)["state_id"]
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
		httputil.HandleError(w, "Could not check if state is optimistic: "+err.Error(), http.StatusInternalServerError)
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
	httputil.WriteSsz(w, sszState, "beacon_state.ssz")
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
				TimeStamp:                fmt.Sprintf("%d", n.Timestamp),
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
			SafeHeadRoot:                  hexutil.Encode(dump.SafeHeadRoot),
			ProposerBoostRoot:             hexutil.Encode(dump.ProposerBoostRoot),
			PreviousProposerBoostRoot:     hexutil.Encode(dump.PreviousProposerBoostRoot),
			HeadRoot:                      hexutil.Encode(dump.HeadRoot),
		},
	}
	httputil.WriteJson(w, resp)
}
