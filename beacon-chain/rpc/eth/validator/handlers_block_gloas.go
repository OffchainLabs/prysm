package validator

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// ProduceBlockV4 requests a beacon node to produce a valid Gloas block.
// When include_payload=true (default), the response includes the execution payload
// envelope alongside the beacon block.
// Endpoint: GET /eth/v4/validator/blocks/{slot}
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
	rawSkipRandaoVerification := r.URL.Query().Get("skip_randao_verification")

	var bbFactor *wrapperspb.UInt64Value
	rawBbFactor, bbValue, ok := shared.UintFromQuery(w, r, "builder_boost_factor", false)
	if !ok {
		return
	}
	if rawBbFactor != "" {
		bbFactor = &wrapperspb.UInt64Value{Value: bbValue}
	}

	includePayload := true
	if raw := r.URL.Query().Get("include_payload"); raw == "false" {
		includePayload = false
	}

	var randaoReveal []byte
	if rawSkipRandaoVerification == "true" {
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

	v1alpha1resp, err := s.V1Alpha1Server.GetBeaconBlock(ctx, &eth.BlockRequest{
		Slot:                  primitives.Slot(slot),
		RandaoReveal:          randaoReveal,
		Graffiti:              graffiti,
		SkipMevBoost:          false,
		BuilderBoostFactor:    bbFactor,
		EagerPayloadStateRoot: includePayload,
	})
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// A self-built block carries its payload inline as GloasContents. An external builder bid (or
	// include_payload=false) yields the block alone; its payload is revealed separately (beacon-APIs #580).
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

	w.Header().Set(api.VersionHeader, version.String(version.Gloas))
	w.Header().Set(api.ConsensusBlockValueHeader, consensusBlockValue)
	w.Header().Set(api.ExecutionPayloadIncludedHeader, fmt.Sprintf("%v", includePayload))

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
		ConsensusBlockValue:      consensusBlockValue,
		ExecutionPayloadIncluded: false,
		Data:                     jsonBytes,
	})
}

// ExecutionPayloadEnvelope returns the cached envelope in blinded form (payload_root);
// HTR equivalence lets the VC sign the blinded form for the full envelope.
// Endpoint: GET /eth/v1/validator/execution_payload_envelopes/{slot}/{beacon_block_root}
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
	if resp.Blinded == nil {
		httputil.HandleError(w, "execution payload envelope not found", http.StatusNotFound)
		return
	}
	if !bytes.Equal(resp.Blinded.BeaconBlockRoot, beaconBlockRoot) {
		httputil.HandleError(w, "cached envelope beacon_block_root does not match request", http.StatusNotFound)
		return
	}
	blinded := resp.Blinded

	w.Header().Set(api.VersionHeader, version.String(version.Gloas))

	if httputil.RespondWithSsz(r) {
		sszBytes, err := blinded.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, "could not marshal blinded envelope to SSZ: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, sszBytes)
		return
	}

	jsonEnvelope, err := structs.BlindedExecutionPayloadEnvelopeFromConsensus(blinded)
	if err != nil {
		httputil.HandleError(w, "could not convert envelope to JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httputil.WriteJson(w, &structs.GetValidatorBlindedExecutionPayloadEnvelopeResponse{
		Version: version.String(version.Gloas),
		Data:    jsonEnvelope,
	})
}
