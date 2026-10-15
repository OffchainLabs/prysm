package validator

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpbalpha "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/pkg/errors"
)

// ProduceInclusionList returns the transactions of the execution engine's inclusion list for the current slot.
func (s *Server) ProduceInclusionList(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.ProduceInclusionList")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	_, rawSlot, ok := shared.UintFromQuery(w, r, "slot", true)
	if !ok {
		return
	}
	slot := primitives.Slot(rawSlot)
	if slots.ToEpoch(slot) < params.BeaconConfig().HezeForkEpoch {
		httputil.HandleError(w, "Inclusion lists are not available before the Heze fork", http.StatusBadRequest)
		return
	}
	// The spec builds the inclusion list at the beginning of its slot, so only the current slot is accepted.
	if currentSlot := s.TimeFetcher.CurrentSlot(); slot != currentSlot {
		httputil.HandleError(w, fmt.Sprintf("Requested slot %d is not the current slot %d", slot, currentSlot), http.StatusBadRequest)
		return
	}

	txs, err := s.ExecutionEngineCaller.GetInclusionList(ctx)
	if err != nil {
		httputil.HandleError(w, "Could not get inclusion list from the execution engine: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httputil.WriteJson(w, &structs.ProduceInclusionListResponse{
		Data: structs.TransactionsFromConsensus(inclusionListTransactions(txs)),
	})
}

// inclusionListTransactions drops empty and blob transactions and keeps the longest prefix within MAX_TRANSACTIONS_BYTES_PER_INCLUSION_LIST.
func inclusionListTransactions(txs [][]byte) [][]byte {
	limit := params.BeaconConfig().MaxTransactionsBytesPerInclusionList
	out := make([][]byte, 0, len(txs))
	var size uint64
	for _, tx := range txs {
		if len(tx) == 0 || tx[0] == gethtypes.BlobTxType {
			continue
		}
		if size+uint64(len(tx)) > limit {
			break
		}
		size += uint64(len(tx))
		out = append(out, tx)
	}
	return out
}

// PublishInclusionList validates and stores a signed inclusion list, broadcasts it, and sends the inclusion_list event.
func (s *Server) PublishInclusionList(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.PublishInclusionList")
	defer span.End()

	versionHeader := r.Header.Get(api.VersionHeader)
	if versionHeader == "" {
		httputil.HandleError(w, api.VersionHeader+" header is required", http.StatusBadRequest)
		return
	}
	v, err := version.FromString(versionHeader)
	if err != nil {
		httputil.HandleError(w, "Invalid version: "+err.Error(), http.StatusBadRequest)
		return
	}
	if v < version.Heze {
		httputil.HandleError(w, "Inclusion lists are only supported from the heze fork", http.StatusBadRequest)
		return
	}

	signed, err := decodeSignedInclusionList(r)
	if err != nil {
		if errors.Is(err, io.EOF) {
			httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
			return
		}
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.InclusionListReceiver.OnInclusionList(ctx, signed); err != nil {
		httputil.HandleError(w, "Invalid inclusion list: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.Broadcaster.Broadcast(ctx, signed); err != nil {
		httputil.HandleError(w, "Could not broadcast inclusion list: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.OperationNotifier.OperationFeed().Send(&feed.Event{
		Type: operation.InclusionListReceived,
		Data: &operation.InclusionListReceivedData{SignedInclusionList: signed},
	})
}

// decodeSignedInclusionList reads an SSZ SignedInclusionList or a JSON {"data": SignedInclusionList} body.
func decodeSignedInclusionList(r *http.Request) (*ethpbalpha.SignedInclusionList, error) {
	if httputil.IsRequestSsz(r) {
		body, err := io.ReadAll(io.LimitReader(r.Body, fieldparams.MaxSignedInclusionListSize+1))
		if err != nil {
			return nil, err
		}
		if len(body) == 0 {
			return nil, io.EOF
		}
		if len(body) > fieldparams.MaxSignedInclusionListSize {
			return nil, errors.Errorf("body is larger than %d bytes", fieldparams.MaxSignedInclusionListSize)
		}
		signed := &ethpbalpha.SignedInclusionList{}
		if err := signed.UnmarshalSSZ(body); err != nil {
			return nil, err
		}
		return signed, nil
	}
	var req structs.PublishInclusionListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	if req.Data == nil {
		return nil, io.EOF
	}
	return req.Data.ToConsensus()
}

// GetInclusionListDuties returns the inclusion list committee duties of the requested validators for the epoch.
func (s *Server) GetInclusionListDuties(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.GetInclusionListDuties")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	_, requestedEpochUint, ok := shared.UintFromRoute(w, r, "epoch")
	if !ok {
		return
	}
	requestedEpoch := primitives.Epoch(requestedEpochUint)
	if requestedEpoch < params.BeaconConfig().HezeForkEpoch {
		httputil.HandleError(w, "Inclusion list duties are not available before the Heze fork", http.StatusBadRequest)
		return
	}

	var indices []string
	err := json.NewDecoder(r.Body).Decode(&indices)
	switch {
	case errors.Is(err, io.EOF):
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(indices) == 0 {
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	}
	requestedValIndices := make([]primitives.ValidatorIndex, len(indices))
	for i, ix := range indices {
		valIx, valid := shared.ValidateUint(w, fmt.Sprintf("ValidatorIndices[%d]", i), ix)
		if !valid {
			return
		}
		requestedValIndices[i] = primitives.ValidatorIndex(valIx)
	}

	currentEpoch := slots.ToEpoch(s.TimeFetcher.CurrentSlot())
	nextEpoch := currentEpoch.Add(1)
	if requestedEpoch > nextEpoch {
		httputil.HandleError(w,
			fmt.Sprintf("Request epoch %d can not be greater than next epoch %d", requestedEpoch, nextEpoch),
			http.StatusBadRequest)
		return
	}

	// The current epoch state also gives the next epoch committees.
	epochForState := requestedEpoch
	if requestedEpoch == nextEpoch {
		epochForState = currentEpoch
	}
	st, err := s.Stater.StateByEpoch(ctx, epochForState)
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}

	coreDuties, rpcErr := s.CoreService.InclusionListDuties(ctx, st, requestedEpoch, requestedValIndices)
	if rpcErr != nil {
		httputil.HandleError(w, rpcErr.Err.Error(), core.ErrorReasonToHTTP(rpcErr.Reason))
		return
	}
	duties := make([]*structs.InclusionListDuty, 0, len(coreDuties))
	for _, d := range coreDuties {
		duties = append(duties, &structs.InclusionListDuty{
			Pubkey:         hexutil.Encode(d.Pubkey[:]),
			ValidatorIndex: strconv.FormatUint(uint64(d.ValidatorIndex), 10),
			Slot:           strconv.FormatUint(uint64(d.Slot), 10),
		})
	}

	// The dependent root is the attester duties root, which is also get_shuffling_dependent_root for the epoch.
	var dependentRoot []byte
	if requestedEpoch <= 1 {
		r, err := s.BeaconDB.GenesisBlockRoot(ctx)
		if err != nil {
			httputil.HandleError(w, "Could not get genesis block root: "+err.Error(), http.StatusInternalServerError)
			return
		}
		dependentRoot = r[:]
	} else {
		dependentRoot, err = core.AttestationDependentRoot(st, requestedEpoch)
		if err != nil {
			httputil.HandleError(w, "Could not get dependent root: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	isOptimistic, err := s.OptimisticModeFetcher.IsOptimistic(ctx)
	if err != nil {
		httputil.HandleError(w, "Could not check optimistic status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	httputil.WriteJson(w, &structs.GetInclusionListDutiesResponse{
		DependentRoot:       hexutil.Encode(dependentRoot),
		ExecutionOptimistic: isOptimistic,
		Data:                duties,
	})
}
