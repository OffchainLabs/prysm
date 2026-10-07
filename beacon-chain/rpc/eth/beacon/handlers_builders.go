package beacon

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

// Builder statuses as defined by the beacon API spec.
const (
	builderStatusPending = "pending"
	builderStatusActive  = "active"
	builderStatusExited  = "exited"
)

// GetStateBuilders returns a filterable list of builders with their status and index.
// The request body is optional; when it is omitted all builders are returned.
func (s *Server) GetStateBuilders(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetStateBuilders")
	defer span.End()

	stateId := r.PathValue("state_id")
	if stateId == "" {
		httputil.HandleError(w, "state_id is required in URL params", http.StatusBadRequest)
		return
	}
	st, err := s.Stater.State(ctx, []byte(stateId))
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}
	if st.Version() < version.Gloas {
		httputil.HandleError(w, "state_id is prior to gloas", http.StatusBadRequest)
		return
	}

	isOptimistic, err := helpers.IsOptimistic(ctx, []byte(stateId), s.OptimisticModeFetcher, s.Stater, s.ChainInfoFetcher, s.BeaconDB)
	if err != nil {
		helpers.HandleIsOptimisticError(w, err)
		return
	}
	blockRoot, err := helpers.BlockRootFromState(ctx, st)
	if err != nil {
		httputil.HandleError(w, "Could not calculate block root: "+err.Error(), http.StatusInternalServerError)
		return
	}
	isFinalized := s.FinalizationFetcher.IsFinalized(ctx, blockRoot)

	var req structs.GetStateBuildersRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil && !errors.Is(err, io.EOF) {
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	statusFilter := make(map[string]bool, len(req.Statuses))
	for _, rawStatus := range req.Statuses {
		status := strings.ToLower(rawStatus)
		if status != builderStatusPending && status != builderStatusActive && status != builderStatusExited {
			httputil.HandleError(w, "Invalid status "+rawStatus, http.StatusBadRequest)
			return
		}
		statusFilter[status] = true
	}

	builders, err := st.Builders()
	if err != nil {
		httputil.HandleError(w, "Could not get builders: "+err.Error(), http.StatusInternalServerError)
		return
	}

	ids, ok := decodeBuilderIds(w, st, req.Ids, uint64(len(builders)))
	if !ok {
		return
	}
	// An empty ids slice means either that no filter was supplied (return all builders) or
	// that every supplied ID was unknown (return nothing). Expand to the full registry only
	// in the first case.
	if len(ids) == 0 && len(req.Ids) == 0 {
		ids = make([]primitives.BuilderIndex, len(builders))
		for i := range builders {
			ids[i] = primitives.BuilderIndex(i)
		}
	}

	data := make([]*structs.BuilderResponse, 0, len(ids))
	for _, id := range ids {
		builder := builders[id]
		status, err := builderStatus(st, id, builder)
		if err != nil {
			httputil.HandleError(w, "Could not get builder status: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if len(statusFilter) > 0 && !statusFilter[status] {
			continue
		}
		data = append(data, &structs.BuilderResponse{
			Index:   strconv.FormatUint(uint64(id), 10),
			Status:  status,
			Builder: structs.BuilderFromConsensus(builder),
		})
	}

	httputil.WriteJson(w, &structs.GetStateBuildersResponse{
		ExecutionOptimistic: isOptimistic,
		Finalized:           isFinalized,
		Data:                data,
	})
}

// builderStatus returns the beacon API status of a builder. A builder that has initiated an exit
// is "exited", a builder whose placement in the registry is finalized is "active" and any other
// builder is "pending".
func builderStatus(st state.BeaconState, id primitives.BuilderIndex, builder *eth.Builder) (string, error) {
	if builder.WithdrawableEpoch != params.BeaconConfig().FarFutureEpoch {
		return builderStatusExited, nil
	}
	active, err := st.IsActiveBuilder(id)
	if err != nil {
		return "", err
	}
	if active {
		return builderStatusActive, nil
	}
	return builderStatusPending, nil
}

// decodeBuilderIds translates raw builder identifiers (hex encoded pubkeys or builder indices)
// into builder indices. Malformed identifiers result in a 400 response and a false return value.
// Identifiers that don't match a known builder are ignored, as required by the beacon API spec.
func decodeBuilderIds(w http.ResponseWriter, st state.BeaconState, rawIds []string, builderCount uint64) ([]primitives.BuilderIndex, bool) {
	ids := make([]primitives.BuilderIndex, 0, len(rawIds))
	for _, rawId := range rawIds {
		pubkey, err := hexutil.Decode(rawId)
		if err == nil {
			if len(pubkey) != fieldparams.BLSPubkeyLength {
				httputil.HandleError(w, fmt.Sprintf("Pubkey length is %d instead of %d", len(pubkey), fieldparams.BLSPubkeyLength), http.StatusBadRequest)
				return nil, false
			}
			builderIndex, ok := st.BuilderIndexByPubkey(bytesutil.ToBytes48(pubkey))
			if !ok {
				continue
			}
			ids = append(ids, builderIndex)
			continue
		}

		index, err := strconv.ParseUint(rawId, 10, 64)
		if err != nil {
			httputil.HandleError(w, fmt.Sprintf("Invalid builder ID %s", rawId), http.StatusBadRequest)
			return nil, false
		}
		if index >= builderCount {
			continue
		}
		ids = append(ids, primitives.BuilderIndex(index))
	}
	return ids, true
}
