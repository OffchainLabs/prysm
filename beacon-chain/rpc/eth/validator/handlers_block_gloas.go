package validator

import (
	"encoding/json"
	"fmt"
	"net/http"

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

	gloasBlock, ok := v1alpha1resp.Block.(*eth.GenericBeaconBlock_Gloas)
	if !ok {
		httputil.HandleError(w, fmt.Sprintf("expected Gloas block, got %T", v1alpha1resp.Block), http.StatusInternalServerError)
		return
	}

	consensusBlockValue, httpError := getConsensusBlockValue(ctx, s.BlockRewardFetcher, v1alpha1resp.Block)
	if httpError != nil {
		log.WithError(httpError).Debug("Failed to get consensus block value")
		consensusBlockValue = "0"
	}

	w.Header().Set(api.VersionHeader, version.String(version.Gloas))
	w.Header().Set(api.ConsensusBlockValueHeader, consensusBlockValue)
	w.Header().Set(api.ExecutionPayloadIncludedHeader, fmt.Sprintf("%v", includePayload))

	isSSZ := httputil.RespondWithSsz(r)

	if includePayload {
		envelopeResp, err := s.V1Alpha1Server.GetExecutionPayloadEnvelope(ctx, &eth.ExecutionPayloadEnvelopeRequest{
			Slot: primitives.Slot(slot),
		})
		if err != nil {
			httputil.HandleError(w, errors.Wrap(err, "could not get execution payload envelope").Error(), http.StatusInternalServerError)
			return
		}

		if isSSZ {
			sszResp, err := (&eth.BeaconBlockContentsGloas{
				Block:                    gloasBlock.Gloas,
				ExecutionPayloadEnvelope: envelopeResp.Envelope,
			}).MarshalSSZ()
			if err != nil {
				httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			httputil.WriteSsz(w, sszResp)
			return
		}

		blockContents, err := structs.BlockContentsGloasFromConsensus(gloasBlock.Gloas, envelopeResp.Envelope)
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

	// include_payload=false: return only the beacon block.
	if isSSZ {
		sszResp, err := gloasBlock.Gloas.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, sszResp)
		return
	}

	block, err := structs.BeaconBlockGloasFromConsensus(gloasBlock.Gloas)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonBytes, err := json.Marshal(block)
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

// ExecutionPayloadEnvelope retrieves a cached execution payload envelope.
//
// TODO: Implement envelope retrieval from cache.
// Endpoint: GET /eth/v1/validator/execution_payload_envelope/{slot}/{builder_index}
func (s *Server) ExecutionPayloadEnvelope(w http.ResponseWriter, r *http.Request) {
	httputil.HandleError(w, "ExecutionPayloadEnvelope not yet implemented", http.StatusNotImplemented)
}

// PublishExecutionPayloadEnvelope broadcasts a signed execution payload envelope.
//
// TODO: Implement envelope validation and broadcast.
// Endpoint: POST /eth/v1/beacon/execution_payload_envelope
func (s *Server) PublishExecutionPayloadEnvelope(w http.ResponseWriter, r *http.Request) {
	httputil.HandleError(w, "PublishExecutionPayloadEnvelope not yet implemented", http.StatusNotImplemented)
}
