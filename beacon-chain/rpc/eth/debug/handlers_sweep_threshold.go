package debug

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/sweepthreshold"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/pkg/errors"
)

// SetSweepThresholdRequestsBody is the request body of PostSweepThresholdRequests.
type SetSweepThresholdRequestsBody struct {
	Requests []*structs.SetSweepThresholdRequest `json:"requests"`
}

// SetSweepThresholdRequestsResponse is the response body of the sweep threshold debug endpoints.
type SetSweepThresholdRequestsResponse struct {
	Data    []*structs.SetSweepThresholdRequest `json:"data"`
	Pending int                                 `json:"pending"`
}

// PostSweepThresholdRequests queues mocked EIP-8148 set sweep threshold requests for
// inclusion in the next block this node proposes.
//
// DEVNET ONLY. EIP-8148 requests are EIP-7685 request type 0x05 and normally come back from
// the execution client in the getPayload response. No execution client implements that
// request type yet, so this endpoint stands in for one. Requests queued here are visible
// only to this node: on a network where other nodes source requests from their execution
// clients, the blocks this node produces will be rejected.
func (s *Server) PostSweepThresholdRequests(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "debug.PostSweepThresholdRequests")
	defer span.End()

	if s.MockSweepThresholdPool == nil {
		httputil.HandleError(w, "Set sweep threshold request pool is not available", http.StatusServiceUnavailable)
		return
	}

	var body SetSweepThresholdRequestsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if len(body.Requests) == 0 {
		httputil.HandleError(w, "At least one request is required", http.StatusBadRequest)
		return
	}

	requests := make([]*enginev1.SetSweepThresholdRequest, 0, len(body.Requests))
	for i, req := range body.Requests {
		converted, err := req.ToConsensus()
		if err != nil {
			httputil.HandleError(w, fmt.Sprintf("Could not decode requests[%d]: %v", i, err), http.StatusBadRequest)
			return
		}

		requests = append(requests, converted)
	}

	if err := s.MockSweepThresholdPool.Insert(requests...); err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, sweepthreshold.ErrPoolFull) {
			code = http.StatusConflict
		}

		httputil.HandleError(w, "Could not queue requests: "+err.Error(), code)
		return
	}

	log.WithField("count", len(requests)).Warn(
		"DEVNET ONLY: queued mocked EIP-8148 set sweep threshold requests for the next locally produced block. Peers sourcing requests from their execution clients will not see them.")

	// Echo back only what this call queued, not the whole queue.
	httputil.WriteJson(w, &SetSweepThresholdRequestsResponse{
		Data:    structs.SetSweepThresholdRequestsFromConsensus(requests),
		Pending: s.MockSweepThresholdPool.Pending(),
	})
}

// GetSweepThresholdRequests returns the mocked set sweep threshold requests still waiting to
// be included in a block. DEVNET ONLY, see PostSweepThresholdRequests.
func (s *Server) GetSweepThresholdRequests(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "debug.GetSweepThresholdRequests")
	defer span.End()

	if s.MockSweepThresholdPool == nil {
		httputil.HandleError(w, "Set sweep threshold request pool is not available", http.StatusServiceUnavailable)
		return
	}

	queued := s.MockSweepThresholdPool.Copy()
	httputil.WriteJson(w, &SetSweepThresholdRequestsResponse{
		Data:    structs.SetSweepThresholdRequestsFromConsensus(queued),
		Pending: len(queued),
	})
}

// DeleteSweepThresholdRequests drops every queued mocked request without including it in a
// block. DEVNET ONLY, see PostSweepThresholdRequests.
func (s *Server) DeleteSweepThresholdRequests(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "debug.DeleteSweepThresholdRequests")
	defer span.End()

	if s.MockSweepThresholdPool == nil {
		httputil.HandleError(w, "Set sweep threshold request pool is not available", http.StatusServiceUnavailable)
		return
	}

	dropped := s.MockSweepThresholdPool.Drain()
	log.WithFields(map[string]any{
		"count": len(dropped),
		"limit": params.BeaconConfig().MaxSetSweepThresholdRequestsPerPayload,
	}).Info("Dropped queued mocked EIP-8148 set sweep threshold requests")

	httputil.WriteJson(w, &SetSweepThresholdRequestsResponse{
		Data:    structs.SetSweepThresholdRequestsFromConsensus(dropped),
		Pending: 0,
	})
}
