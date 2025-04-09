package beacon

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/shared"
	statenative "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/validator"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/eth/v1"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// GetValidatorCount is a HTTP handler that serves the GET /eth/v1/beacon/states/{state_id}/validator_count endpoint.
// It returns the total validator count according to the given statuses provided as a query parameter.
//
// The state ID is expected to be a valid Beacon Chain state identifier.
// The status query parameter can be an array of strings with the following values: pending_initialized, pending_queued, active_ongoing,
// active_exiting, active_slashed, exited_unslashed, exited_slashed, withdrawal_possible, withdrawal_done, active, pending, exited, withdrawal.
// The response is a JSON object containing the total validator count for the specified statuses.
//
// Example usage:
//
//	GET /eth/v1/beacon/states/12345/validator_count?status=active&status=pending
//
// The above request will return a JSON response like:
//
//	{
//		"execution_optimistic": "false",
//		"finalized": "true",
//		"data": [
//			{
//				"status": "active",
//				"count": "13"
//			},
//			{
//				"status": "pending",
//				"count": "6"
//			}
//		]
//	}
func (s *Server) GetValidatorCount(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetValidatorCount")
	defer span.End()

	stateID := r.PathValue("state_id")

	isOptimistic, err := helpers.IsOptimistic(ctx, []byte(stateID), s.OptimisticModeFetcher, s.Stater, s.ChainInfoFetcher, s.BeaconDB)
	if err != nil {
		errJson := &httputil.DefaultJsonError{
			Message: fmt.Sprintf("could not check if slot's block is optimistic: %v", err),
			Code:    http.StatusInternalServerError,
		}
		httputil.WriteError(w, errJson)
		return
	}

	st, err := s.Stater.State(ctx, []byte(stateID))
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}

	blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		errJson := &httputil.DefaultJsonError{
			Message: fmt.Sprintf("could not calculate root of latest block header: %v", err),
			Code:    http.StatusInternalServerError,
		}
		httputil.WriteError(w, errJson)
		return
	}

	isFinalized := s.FinalizationFetcher.IsFinalized(ctx, blockRoot)

	var statusVals []validator.Status
	for _, status := range r.URL.Query()["status"] {
		statusVal, ok := ethpb.ValidatorStatus_value[strings.ToUpper(status)]
		if !ok {
			errJson := &httputil.DefaultJsonError{
				Message: fmt.Sprintf("invalid status query parameter: %v", status),
				Code:    http.StatusBadRequest,
			}
			httputil.WriteError(w, errJson)
			return
		}

		statusVals = append(statusVals, validator.Status(statusVal))
	}

	// If no status was provided then consider all the statuses to return validator count for each status.
	if len(statusVals) == 0 {
		for _, val := range ethpb.ValidatorStatus_value {
			statusVals = append(statusVals, validator.Status(val))
		}
	}

	epoch := slots.ToEpoch(st.Slot())
	valCount, err := validatorCountByStatus(st.Validators(), statusVals, epoch)
	if err != nil {
		errJson := &httputil.DefaultJsonError{
			Message: fmt.Sprintf("could not get validator count: %v", err),
			Code:    http.StatusInternalServerError,
		}
		httputil.WriteError(w, errJson)
		return
	}

	valCountResponse := &structs.GetValidatorCountResponse{
		ExecutionOptimistic: strconv.FormatBool(isOptimistic),
		Finalized:           strconv.FormatBool(isFinalized),
		Data:                valCount,
	}

	httputil.WriteJson(w, valCountResponse)
}

// validatorCountByStatus returns a slice of validator count for each status in the given epoch.
func validatorCountByStatus(validators []*eth.Validator, statuses []validator.Status, epoch primitives.Epoch) ([]*structs.ValidatorCount, error) {
	countByStatus := make(map[validator.Status]uint64)
	for _, val := range validators {
		readOnlyVal, err := statenative.NewValidator(val)
		if err != nil {
			return nil, fmt.Errorf("could not convert validator: %w", err)
		}
		valStatus, err := helpers.ValidatorStatus(readOnlyVal, epoch)
		if err != nil {
			return nil, fmt.Errorf("could not get validator status: %w", err)
		}
		valSubStatus, err := helpers.ValidatorSubStatus(readOnlyVal, epoch)
		if err != nil {
			return nil, fmt.Errorf("could not get validator sub status: %w", err)
		}

		for _, status := range statuses {
			if valStatus == status || valSubStatus == status {
				countByStatus[status]++
			}
		}
	}

	var resp []*structs.ValidatorCount
	for status, count := range countByStatus {
		resp = append(resp, &structs.ValidatorCount{
			Status: status.String(),
			Count:  strconv.FormatUint(count, 10),
		})
	}

	// Sort the response slice according to status strings for deterministic ordering of validator count response.
	sort.Slice(resp, func(i, j int) bool {
		return resp[i].Status < resp[j].Status
	})

	return resp, nil
}
