package validator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
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
		var blobs, kzgProofs [][]byte
		if contents, ok := s.ExecutionPayloadEnvelopeCache.Contents(); ok &&
			contents.Envelope.Payload.SlotNumber == primitives.Slot(slot) {
			blobs, kzgProofs, err = blobsAndProofsFromDataColumns(contents.DataColumns)
			if err != nil {
				httputil.HandleError(w, errors.Wrap(err, "could not derive blobs from cached data columns").Error(), http.StatusInternalServerError)
				return
			}
		}

		if isSSZ {
			sszResp, err := (&eth.BeaconBlockContentsGloas{
				Block:                    gloasBlock.Gloas,
				ExecutionPayloadEnvelope: envelopeResp.Envelope,
				KzgProofs:                kzgProofs,
				Blobs:                    blobs,
			}).MarshalSSZ()
			if err != nil {
				httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			httputil.WriteSsz(w, sszResp)
			return
		}

		blockContents, err := structs.BlockContentsGloasFromConsensus(gloasBlock.Gloas, envelopeResp.Envelope, kzgProofs, blobs)
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
// Endpoint: GET /eth/v1/validator/execution_payload_envelope/{slot}
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

	jsonEnvelope, err := structs.ExecutionPayloadEnvelopeFromConsensus(resp.Envelope)
	if err != nil {
		httputil.HandleError(w, "could not convert envelope to JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(api.VersionHeader, version.String(version.Gloas))
	httputil.WriteJson(w, &structs.GetValidatorExecutionPayloadEnvelopeResponse{
		Version: version.String(version.Gloas),
		Data:    jsonEnvelope,
	})
}

// blobsAndProofsFromDataColumns derives raw blobs and the flat KZG proofs
// vector (indexed [blob*numCols + col]) from cached sidecars. Pure memory
// shuffling: ReconstructBlobs hits its cheap branch since we have every column.
func blobsAndProofsFromDataColumns(sidecars []consensusblocks.RODataColumn) ([][]byte, [][]byte, error) {
	if len(sidecars) == 0 {
		return nil, nil, nil
	}
	const numColumns = fieldparams.NumberOfColumns
	if len(sidecars) != numColumns {
		return nil, nil, errors.Errorf("expected %d data column sidecars, got %d", numColumns, len(sidecars))
	}

	verified := make([]consensusblocks.VerifiedRODataColumn, len(sidecars))
	for i, sc := range sidecars {
		verified[i] = consensusblocks.NewVerifiedRODataColumn(sc)
	}
	blobCount := len(sidecars[0].Column())
	blobs, err := peerdas.ReconstructBlobs(verified, nil, blobCount)
	if err != nil {
		return nil, nil, errors.Wrap(err, "reconstruct blobs from data columns")
	}

	proofs := make([][]byte, blobCount*numColumns)
	for blobIdx := 0; blobIdx < blobCount; blobIdx++ {
		for col := 0; col < numColumns; col++ {
			proofs[blobIdx*numColumns+col] = sidecars[col].KzgProofs()[blobIdx]
		}
	}
	return blobs, proofs, nil
}
