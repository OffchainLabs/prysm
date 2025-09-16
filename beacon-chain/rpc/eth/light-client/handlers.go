package lightclient

import (
	"fmt"
	"net/http"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	lightclient "github.com/OffchainLabs/prysm/v6/beacon-chain/light-client"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
)

// GetLightClientBootstrap - implements https://github.com/ethereum/beacon-APIs/blob/263f4ed6c263c967f13279c7a9f5629b51c5fc55/apis/beacon/light_client/bootstrap.yaml
func (s *Server) GetLightClientBootstrap(w http.ResponseWriter, req *http.Request) {
	// Prepare
	ctx, span := trace.StartSpan(req.Context(), "beacon.GetLightClientBootstrap")
	defer span.End()

	// Get the block
	blockRootParam, err := hexutil.Decode(req.PathValue("block_root"))
	if err != nil {
		httputil.HandleError(w, "Invalid block root: "+err.Error(), http.StatusBadRequest)
		return
	}

	blockRoot := bytesutil.ToBytes32(blockRootParam)
	bootstrap, err := s.LCStore.LightClientBootstrap(ctx, blockRoot)
	if err != nil {
		if errors.Is(err, lightclient.ErrLightClientBootstrapNotFound) {
			httputil.HandleError(w, "Light client bootstrap not found", http.StatusNotFound)
		} else {
			httputil.HandleError(w, "Could not get light client bootstrap: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set(api.VersionHeader, version.String(bootstrap.Version()))

	if httputil.RespondWithSsz(req) {
		ssz, err := bootstrap.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, "Could not marshal bootstrap to SSZ: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, ssz)
	} else {
		data, err := structs.LightClientBootstrapFromConsensus(bootstrap)
		if err != nil {
			httputil.HandleError(w, "Could not marshal bootstrap to JSON: "+err.Error(), http.StatusInternalServerError)
			return
		}
		response := &structs.LightClientBootstrapResponse{
			Version: version.String(bootstrap.Version()),
			Data:    data,
		}
		httputil.WriteJson(w, response)
	}
}

// GetLightClientUpdatesByRange - implements https://github.com/ethereum/beacon-APIs/blob/263f4ed6c263c967f13279c7a9f5629b51c5fc55/apis/beacon/light_client/updates.yaml
func (s *Server) GetLightClientUpdatesByRange(w http.ResponseWriter, req *http.Request) {
	ctx, span := trace.StartSpan(req.Context(), "beacon.GetLightClientUpdatesByRange")
	defer span.End()

	config := params.BeaconConfig()

	_, count, gotCount := shared.UintFromQuery(w, req, "count", true)
	if !gotCount {
		return
	} else if count == 0 {
		httputil.HandleError(w, fmt.Sprintf("Got invalid 'count' query variable '%d': count must be greater than 0", count), http.StatusBadRequest)
		return
	}

	if count > config.MaxRequestLightClientUpdates {
		count = config.MaxRequestLightClientUpdates
	}

	_, startPeriod, gotStartPeriod := shared.UintFromQuery(w, req, "start_period", true)
	if !gotStartPeriod {
		return
	}

	if startPeriod*uint64(config.EpochsPerSyncCommitteePeriod) < uint64(config.AltairForkEpoch) {
		httputil.HandleError(w, "Invalid 'start_period': before Altair fork", http.StatusBadRequest)
		return
	}

	endPeriod := startPeriod + count - 1

	headBlock, err := s.HeadFetcher.HeadBlock(ctx)
	if err != nil {
		httputil.HandleError(w, "Could not get head block: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// get updates
	updates, err := s.LCStore.LightClientUpdates(ctx, startPeriod, endPeriod, headBlock)
	if err != nil {
		httputil.HandleError(w, "Could not get light client updates from DB: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if httputil.RespondWithSsz(req) {
		w.Header().Set("Content-Type", "application/octet-stream")

		for _, update := range updates {
			if ctx.Err() != nil {
				httputil.HandleError(w, "Context error: "+ctx.Err().Error(), http.StatusInternalServerError)
			}

			updateSlot := update.AttestedHeader().Beacon().Slot
			updateEpoch := slots.ToEpoch(updateSlot)
			updateEntry := params.GetNetworkScheduleEntry(updateEpoch)
			updateSSZ, err := update.MarshalSSZ()
			if err != nil {
				httputil.HandleError(w, "Could not marshal update to SSZ: "+err.Error(), http.StatusInternalServerError)
				return
			}

			var chunkLength []byte
			chunkLength = ssz.MarshalUint64(chunkLength, uint64(len(updateSSZ)+4))
			if _, err := w.Write(chunkLength); err != nil {
				httputil.HandleError(w, "Could not write chunk length: "+err.Error(), http.StatusInternalServerError)
			}
			if _, err := w.Write(updateEntry.ForkDigest[:]); err != nil {
				httputil.HandleError(w, "Could not write fork digest: "+err.Error(), http.StatusInternalServerError)
			}
			if _, err := w.Write(updateSSZ); err != nil {
				httputil.HandleError(w, "Could not write update SSZ: "+err.Error(), http.StatusInternalServerError)
			}
		}
	} else {
		updatesResponses := make([]*structs.LightClientUpdateResponse, 0, len(updates))

		for _, update := range updates {
			if ctx.Err() != nil {
				httputil.HandleError(w, "Context error: "+ctx.Err().Error(), http.StatusInternalServerError)
			}

			updateJson, err := structs.LightClientUpdateFromConsensus(update)
			if err != nil {
				httputil.HandleError(w, "Could not convert light client update: "+err.Error(), http.StatusInternalServerError)
				return
			}
			updateResponse := &structs.LightClientUpdateResponse{
				Version: version.String(update.Version()),
				Data:    updateJson,
			}
			updatesResponses = append(updatesResponses, updateResponse)
		}

		httputil.WriteJson(w, updatesResponses)
	}
}

// GetLightClientFinalityUpdate - implements https://github.com/ethereum/beacon-APIs/blob/263f4ed6c263c967f13279c7a9f5629b51c5fc55/apis/beacon/light_client/finality_update.yaml
func (s *Server) GetLightClientFinalityUpdate(w http.ResponseWriter, req *http.Request) {
	_, span := trace.StartSpan(req.Context(), "beacon.GetLightClientFinalityUpdate")
	defer span.End()

	update := s.LCStore.LastFinalityUpdate()
	if update == nil {
		httputil.HandleError(w, "No light client finality update available", http.StatusNotFound)
		return
	}

	w.Header().Set(api.VersionHeader, version.String(update.Version()))
	if httputil.RespondWithSsz(req) {
		data, err := update.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, "Could not marshal finality update to SSZ: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, data)
	} else {
		data, err := structs.LightClientFinalityUpdateFromConsensus(update)
		if err != nil {
			httputil.HandleError(w, "Could not convert light client finality update to API struct: "+err.Error(), http.StatusInternalServerError)
			return
		}
		response := &structs.LightClientFinalityUpdateResponse{
			Version: version.String(update.Version()),
			Data:    data,
		}
		httputil.WriteJson(w, response)
	}
}

// GetLightClientOptimisticUpdate - implements https://github.com/ethereum/beacon-APIs/blob/263f4ed6c263c967f13279c7a9f5629b51c5fc55/apis/beacon/light_client/optimistic_update.yaml
func (s *Server) GetLightClientOptimisticUpdate(w http.ResponseWriter, req *http.Request) {
	_, span := trace.StartSpan(req.Context(), "beacon.GetLightClientOptimisticUpdate")
	defer span.End()

	update := s.LCStore.LastOptimisticUpdate()
	if update == nil {
		httputil.HandleError(w, "No light client optimistic update available", http.StatusNotFound)
		return
	}

	w.Header().Set(api.VersionHeader, version.String(update.Version()))
	if httputil.RespondWithSsz(req) {
		data, err := update.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, "Could not marshal optimistic update to SSZ: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, data)
	} else {
		data, err := structs.LightClientOptimisticUpdateFromConsensus(update)
		if err != nil {
			httputil.HandleError(w, "Could not convert light client optimistic update to API struct: "+err.Error(), http.StatusInternalServerError)
			return
		}
		response := &structs.LightClientOptimisticUpdateResponse{
			Version: version.String(update.Version()),
			Data:    data,
		}
		httputil.WriteJson(w, response)
	}
}
