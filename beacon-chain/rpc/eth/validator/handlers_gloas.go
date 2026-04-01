package validator

import (
	"encoding/json"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// ProduceBlockV4 requests a beacon node to produce a valid Gloas block.
// Endpoint: GET /eth/v4/validator/blocks/{slot}
func (s *Server) ProduceBlockV4(w http.ResponseWriter, r *http.Request) {
	s.ProduceBlockV3(w, r)
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

func handleProduceGloasV4(
	w http.ResponseWriter,
	isSSZ bool,
	blk *eth.GenericBeaconBlock_Gloas,
	executionPayloadValue string,
	consensusBlockValue string,
) {
	if isSSZ {
		sszResp, err := blk.Gloas.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, sszResp)
		return
	}

	block, err := structs.BeaconBlockGloasFromConsensus(blk.Gloas)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonBytes, err := json.Marshal(block)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httputil.WriteJson(w, &structs.ProduceBlockV3Response{
		Version:                 version.String(version.Gloas),
		ExecutionPayloadBlinded: false,
		ExecutionPayloadValue:   executionPayloadValue,
		ConsensusBlockValue:     consensusBlockValue,
		Data:                    jsonBytes,
	})
}
