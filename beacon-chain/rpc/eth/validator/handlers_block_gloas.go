package validator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ProduceBlockV4 requests a beacon node to produce a valid Gloas block.
// When include_payload=true, the response includes the execution payload
// envelope alongside the beacon block.
// The body carries a BuilderConfig naming external builders to request bids from.
// Endpoint: POST /eth/v4/validator/blocks/{slot}
func (s *Server) ProduceBlockV4(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.ProduceBlockV4")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	rawSlot := r.PathValue("slot")

	slot, valid := shared.ValidateUint(w, "slot", rawSlot)
	if !valid {
		return
	}
	if slots.ToEpoch(primitives.Slot(slot)) < params.BeaconConfig().GloasForkEpoch {
		httputil.HandleError(w, "ProduceBlockV4 is only supported for Gloas and later forks", http.StatusBadRequest)
		return
	}

	rawRandaoReveal := r.URL.Query().Get("randao_reveal")
	rawGraffiti := r.URL.Query().Get("graffiti")

	rawIncludePayload := r.URL.Query().Get("include_payload")
	if rawIncludePayload == "" {
		httputil.HandleError(w, "include_payload is required in query params", http.StatusBadRequest)
		return
	}
	includePayload, err := strconv.ParseBool(rawIncludePayload)
	if err != nil {
		httputil.HandleError(w, "invalid include_payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	var randaoReveal []byte
	if skipRandaoVerification(r) {
		randaoReveal = common.InfiniteSignature[:]
	} else {
		rr, err := bytesutil.DecodeHexWithLength(rawRandaoReveal, fieldparams.BLSSignatureLength)
		if err != nil {
			httputil.HandleError(w, errors.Wrap(err, "unable to decode randao reveal").Error(), http.StatusBadRequest)
			return
		}
		randaoReveal = rr
	}
	var graffiti []byte
	if rawGraffiti != "" {
		g, err := bytesutil.DecodeHexWithLength(rawGraffiti, 32)
		if err != nil {
			httputil.HandleError(w, errors.Wrap(err, "unable to decode graffiti").Error(), http.StatusBadRequest)
			return
		}
		graffiti = g
	}

	builderConfig, ok := decodeBuilderConfig(w, r)
	if !ok {
		return
	}

	// Gloas has no MEV-boost path: p2p-bid and per-builder boosts arrive inside BuilderConfig,
	// so the legacy skip_mev_boost/builder_boost_factor request fields are not used here.
	v1alpha1resp, err := s.V1Alpha1Server.GetBeaconBlock(ctx, &eth.BlockRequest{
		Slot:                  primitives.Slot(slot),
		RandaoReveal:          randaoReveal,
		Graffiti:              graffiti,
		EagerPayloadStateRoot: includePayload,
		BuilderConfig:         builderConfig,
	})
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// A self-built block carries its payload inline as GloasContents. An external builder bid (or
	// include_payload=false) yields the block alone; its payload is revealed separately.
	var block *eth.BeaconBlockGloas
	var contents *eth.BeaconBlockContentsGloas
	switch b := v1alpha1resp.Block.(type) {
	case *eth.GenericBeaconBlock_GloasContents:
		contents = b.GloasContents
		block = contents.Block
	case *eth.GenericBeaconBlock_Gloas:
		block = b.Gloas
		includePayload = false
	default:
		httputil.HandleError(w, fmt.Sprintf("expected Gloas block, got %T", v1alpha1resp.Block), http.StatusInternalServerError)
		return
	}

	consensusBlockValue, httpError := getConsensusBlockValue(ctx, s.BlockRewardFetcher, block)
	if httpError != nil {
		log.WithError(httpError).Debug("Failed to get consensus block value")
		consensusBlockValue = "0"
	}

	executionPayloadValue := v1alpha1resp.PayloadValue
	if executionPayloadValue == "" {
		executionPayloadValue = "0"
	}

	w.Header().Set(api.VersionHeader, version.String(version.Gloas))
	w.Header().Set(api.ExecutionPayloadValueHeader, executionPayloadValue)
	w.Header().Set(api.ConsensusBlockValueHeader, consensusBlockValue)
	w.Header().Set(api.ExecutionPayloadIncludedHeader, fmt.Sprintf("%v", includePayload))
	if v1alpha1resp.BuilderUrl != "" {
		w.Header().Set(api.BuilderUrlHeader, v1alpha1resp.BuilderUrl)
	}

	isSSZ := httputil.RespondWithSsz(r)

	if includePayload {
		if isSSZ {
			sszResp, err := contents.MarshalSSZ()
			if err != nil {
				httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			httputil.WriteSsz(w, sszResp)
			return
		}

		blockContents, err := structs.BlockContentsGloasFromConsensus(contents.Block, contents.ExecutionPayloadEnvelope, contents.KzgProofs, contents.Blobs)
		if err != nil {
			httputil.HandleError(w, errors.Wrap(err, "could not convert block contents").Error(), http.StatusInternalServerError)
			return
		}
		jsonBytes, err := json.Marshal(blockContents)
		if err != nil {
			httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteJson(w, &structs.ProduceBlockV4Response{
			Version:                  version.String(version.Gloas),
			ExecutionPayloadValue:    executionPayloadValue,
			ConsensusBlockValue:      consensusBlockValue,
			ExecutionPayloadIncluded: true,
			Data:                     jsonBytes,
		})
		return
	}

	// include_payload=false (or external builder bid): return only the beacon block.
	if isSSZ {
		sszResp, err := block.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, sszResp)
		return
	}

	structBlock, err := structs.BeaconBlockGloasFromConsensus(block)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonBytes, err := json.Marshal(structBlock)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httputil.WriteJson(w, &structs.ProduceBlockV4Response{
		Version:                  version.String(version.Gloas),
		ExecutionPayloadValue:    executionPayloadValue,
		ConsensusBlockValue:      consensusBlockValue,
		ExecutionPayloadIncluded: false,
		Data:                     jsonBytes,
	})
}

// decodeBuilderConfig reads a JSON- or SSZ-encoded BuilderConfig request body.
// On failure it writes the error response and returns false.
func decodeBuilderConfig(w http.ResponseWriter, r *http.Request) (*eth.BuilderConfig, bool) {
	if !requireGloasVersionHeader(w, r, "Builder config is only supported from the gloas fork") {
		return nil, false
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httputil.HandleError(w, "Could not read request body: "+err.Error(), http.StatusInternalServerError)
		return nil, false
	}
	if len(body) == 0 {
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return nil, false
	}
	if httputil.IsRequestSsz(r) {
		cfg := &eth.BuilderConfig{}
		if err := cfg.UnmarshalSSZ(body); err != nil {
			httputil.HandleError(w, "Could not decode SSZ builder config: "+err.Error(), http.StatusBadRequest)
			return nil, false
		}
		return cfg, true
	}
	var cfg structs.BuilderConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		httputil.HandleError(w, "Could not decode builder config: "+err.Error(), http.StatusBadRequest)
		return nil, false
	}
	consensusCfg, err := cfg.ToConsensus()
	if err != nil {
		httputil.HandleError(w, "Could not decode builder config: "+err.Error(), http.StatusBadRequest)
		return nil, false
	}
	return consensusCfg, true
}

// ExecutionPayloadEnvelope returns the cached execution payload envelope for the VC to sign and
// publish. Endpoint: GET /eth/v1/validator/execution_payload_envelopes/{slot}/{beacon_block_root}
func (s *Server) ExecutionPayloadEnvelope(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.ExecutionPayloadEnvelope")
	defer span.End()

	rawSlot := r.PathValue("slot")
	if rawSlot == "" {
		httputil.HandleError(w, "slot is required in URL params", http.StatusBadRequest)
		return
	}
	slot, err := strconv.ParseUint(rawSlot, 10, 64)
	if err != nil {
		httputil.HandleError(w, "invalid slot: "+err.Error(), http.StatusBadRequest)
		return
	}
	rawBeaconBlockRoot := r.PathValue("beacon_block_root")
	if rawBeaconBlockRoot == "" {
		httputil.HandleError(w, "beacon_block_root is required in URL params", http.StatusBadRequest)
		return
	}
	beaconBlockRoot, err := bytesutil.DecodeHexWithLength(rawBeaconBlockRoot, fieldparams.RootLength)
	if err != nil {
		httputil.HandleError(w, "invalid beacon_block_root: "+err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.V1Alpha1Server.GetExecutionPayloadEnvelope(ctx, &eth.ExecutionPayloadEnvelopeRequest{
		Slot: primitives.Slot(slot),
	})
	if err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.NotFound:
				httputil.HandleError(w, st.Message(), http.StatusNotFound)
			case codes.InvalidArgument:
				httputil.HandleError(w, st.Message(), http.StatusBadRequest)
			default:
				httputil.HandleError(w, st.Message(), http.StatusInternalServerError)
			}
			return
		}
		httputil.HandleError(w, "could not get execution payload envelope: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if resp.Envelope == nil {
		httputil.HandleError(w, "execution payload envelope not found", http.StatusNotFound)
		return
	}
	if !bytes.Equal(resp.Envelope.BeaconBlockRoot, beaconBlockRoot) {
		httputil.HandleError(w, "cached envelope beacon_block_root does not match request", http.StatusNotFound)
		return
	}
	envelope := resp.Envelope

	w.Header().Set(api.VersionHeader, version.String(version.Gloas))

	if httputil.RespondWithSsz(r) {
		sszBytes, err := envelope.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, "could not marshal envelope to SSZ: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, sszBytes)
		return
	}

	jsonEnvelope, err := structs.ExecutionPayloadEnvelopeFromConsensus(envelope)
	if err != nil {
		httputil.HandleError(w, "could not convert envelope to JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httputil.WriteJson(w, &structs.GetValidatorExecutionPayloadEnvelopeResponse{
		Version: version.String(version.Gloas),
		Data:    jsonEnvelope,
	})
}
