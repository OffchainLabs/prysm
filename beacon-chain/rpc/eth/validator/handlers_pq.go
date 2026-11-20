package validator

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const vrfProofBytes = fieldparams.VRFProofLength * fieldparams.RootLength

func isPQEpoch(epoch primitives.Epoch) bool {
	return epoch >= params.BeaconConfig().PQForkEpoch
}

// The registry stores XMSS keys with a zero prefix, the API shows the 32 byte key.
func dutyPubkey(epoch primitives.Epoch, pubkey []byte) string {
	if isPQEpoch(epoch) && len(pubkey) == fieldparams.BLSPubkeyLength {
		return hexutil.Encode(pubkey[stateutil.XMSSPubkeyPrefixLength:])
	}
	return hexutil.Encode(pubkey)
}

func writeGRPCError(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	msg := err.Error()
	if st, ok := status.FromError(err); ok {
		msg = st.Message()
		switch st.Code() {
		case codes.NotFound:
			code = http.StatusNotFound
		case codes.InvalidArgument, codes.FailedPrecondition:
			code = http.StatusBadRequest
		case codes.Unavailable:
			code = http.StatusServiceUnavailable
		}
	}
	httputil.HandleError(w, msg, code)
}

// The request body is a builder config, which self-build ignores.
func (s *Server) produceBlockPQ(w http.ResponseWriter, r *http.Request, slot primitives.Slot, includePayload bool) {
	ctx := r.Context()
	if skipRandaoVerification(r) {
		httputil.HandleError(w, "PQ verifies every RANDAO proof", http.StatusBadRequest)
		return
	}
	randaoReveal, err := bytesutil.DecodeHexWithLength(r.URL.Query().Get("randao_reveal"), vrfProofBytes)
	if err != nil {
		httputil.HandleError(w, errors.Wrap(err, "unable to decode randao reveal").Error(), http.StatusBadRequest)
		return
	}
	var graffiti []byte
	if raw := r.URL.Query().Get("graffiti"); raw != "" {
		graffiti, err = bytesutil.DecodeHexWithLength(raw, 32)
		if err != nil {
			httputil.HandleError(w, errors.Wrap(err, "unable to decode graffiti").Error(), http.StatusBadRequest)
			return
		}
	}
	resp, err := s.V1Alpha1Server.GetBeaconBlock(ctx, &eth.BlockRequest{Slot: slot, RandaoReveal: randaoReveal, Graffiti: graffiti})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	b, ok := resp.Block.(*eth.GenericBeaconBlock_Pq)
	if !ok {
		httputil.HandleError(w, fmt.Sprintf("expected a PQ block, got %T", resp.Block), http.StatusInternalServerError)
		return
	}
	payloadValue := resp.PayloadValue
	if payloadValue == "" {
		payloadValue = "0"
	}
	jsonBytes, err := json.Marshal(structs.BeaconBlockPQFromConsensus(b.Pq))
	if includePayload {
		envelope, kzgProofs, blobs, err := s.PQValidator.ExecutionPayloadContentsPQ(slot)
		if err != nil {
			writeGRPCError(w, err)
			return
		}
		contents, err := structs.BlockContentsPQFromConsensus(b.Pq, envelope, kzgProofs, blobs)
		if err != nil {
			httputil.HandleError(w, errors.Wrap(err, "could not convert block contents").Error(), http.StatusInternalServerError)
			return
		}
		jsonBytes, err = json.Marshal(contents)
		if err != nil {
			httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(api.VersionHeader, version.String(version.PQ))
	w.Header().Set(api.ExecutionPayloadValueHeader, payloadValue)
	w.Header().Set(api.ConsensusBlockValueHeader, "0")
	w.Header().Set(api.ExecutionPayloadIncludedHeader, fmt.Sprintf("%v", includePayload))
	httputil.WriteJson(w, &structs.ProduceBlockV4Response{
		Version:                  version.String(version.PQ),
		ExecutionPayloadValue:    payloadValue,
		ConsensusBlockValue:      "0",
		ExecutionPayloadIncluded: includePayload,
		Data:                     jsonBytes,
	})
}

func (s *Server) aggregateAttestationPQ(w http.ResponseWriter, r *http.Request, slot primitives.Slot, index primitives.CommitteeIndex, dataRoot []byte) {
	att, proof, err := s.PQValidator.AggregateAttestationPQ(r.Context(), slot, index, bytesutil.ToBytes32(dataRoot))
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	data, err := json.Marshal(structs.AggregatePQFromConsensus(att, proof))
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(api.VersionHeader, version.String(version.PQ))
	httputil.WriteJson(w, &structs.AggregateAttestationResponse{Version: version.String(version.PQ), Data: data})
}

func (s *Server) submitAggregateAndProofsPQ(w http.ResponseWriter, r *http.Request) {
	var aggregates []*structs.SignedAggregateAttestationAndProofPQ
	if err := json.NewDecoder(r.Body).Decode(&aggregates); err != nil {
		if errors.Is(err, io.EOF) {
			httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
			return
		}
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(aggregates) == 0 {
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	}
	var failures []*server.IndexedError
	for i, aggregate := range aggregates {
		signed, err := aggregate.ToConsensus()
		if err == nil {
			_, err = s.V1Alpha1Server.SubmitSignedAggregateAndProofPQ(r.Context(), signed)
		}
		if err != nil {
			failures = append(failures, &server.IndexedError{Index: i, Message: err.Error()})
		}
	}
	if len(failures) > 0 {
		httputil.WriteError(w, &server.IndexedErrorContainer{
			Code:     http.StatusBadRequest,
			Message:  "One or more aggregates failed",
			Failures: failures,
		})
	}
}

func slotIsPQ(slot primitives.Slot) bool {
	return isPQEpoch(slots.ToEpoch(slot))
}
