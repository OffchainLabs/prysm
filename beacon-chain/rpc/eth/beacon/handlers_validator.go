package beacon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	statenative "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

// GetValidators returns filterable list of validators with their balance, status and index.
func (s *Server) GetValidators(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetValidators")
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

	isOptimistic, err := helpers.IsOptimistic(ctx, []byte(stateId), s.OptimisticModeFetcher, s.Stater, s.ChainInfoFetcher, s.BeaconDB)
	if err != nil {
		helpers.HandleIsOptimisticError(w, err)
		return
	}
	blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "Could not calculate root of latest block header: "+err.Error(), http.StatusInternalServerError)
		return
	}
	isFinalized := s.FinalizationFetcher.IsFinalized(ctx, blockRoot)

	var req structs.GetValidatorsRequest
	if r.Method == http.MethodPost {
		err = json.NewDecoder(r.Body).Decode(&req)
		switch {
		case errors.Is(err, io.EOF):
			httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
			return
		case err != nil:
			httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	var statuses []string
	var rawIds []string
	if r.Method == http.MethodGet {
		rawIds = r.URL.Query()["id"]
		statuses = r.URL.Query()["status"]
	} else {
		rawIds = req.Ids
		statuses = req.Statuses
	}
	for i, ss := range statuses {
		statuses[i] = strings.ToLower(ss)
	}

	ids, ok := decodeIds(w, st, rawIds, true /* ignore unknown */)
	if !ok {
		return
	}
	// return no data if all IDs are ignored
	if len(rawIds) > 0 && len(ids) == 0 {
		resp := &structs.GetValidatorsResponse{
			Data:                []*structs.ValidatorContainer{},
			ExecutionOptimistic: isOptimistic,
			Finalized:           isFinalized,
		}
		httputil.WriteJson(w, resp)
		return
	}

	readOnlyVals, ok := valsFromIds(w, st, ids)
	if !ok {
		return
	}
	epoch := slots.ToEpoch(st.Slot())

	// Exit early if no matching validators were found or we don't want to further filter validators by status.
	if len(readOnlyVals) == 0 || len(statuses) == 0 {
		containers := make([]*structs.ValidatorContainer, len(readOnlyVals))
		for i, val := range readOnlyVals {
			valStatus, err := helpers.ValidatorSubStatus(val, epoch)
			if err != nil {
				httputil.HandleError(w, "Could not get validator status: "+err.Error(), http.StatusInternalServerError)
				return
			}
			id := primitives.ValidatorIndex(i)
			if len(ids) > 0 {
				id = ids[i]
			}
			balance, err := st.BalanceAtIndex(id)
			if err != nil {
				httputil.HandleError(w, "Could not get validator balance: "+err.Error(), http.StatusInternalServerError)
				return
			}
			containers[i] = valContainerFromReadOnlyVal(val, id, balance, valStatus)
		}
		resp := &structs.GetValidatorsResponse{
			Data:                containers,
			ExecutionOptimistic: isOptimistic,
			Finalized:           isFinalized,
		}
		httputil.WriteJson(w, resp)
		return
	}

	filteredStatuses := make(map[validator.Status]bool, len(statuses))
	for _, ss := range statuses {
		ok, vs := validator.StatusFromString(ss)
		if !ok {
			httputil.HandleError(w, "Invalid status "+ss, http.StatusBadRequest)
			return
		}
		filteredStatuses[vs] = true
	}
	valContainers := make([]*structs.ValidatorContainer, 0, len(readOnlyVals))
	for i, val := range readOnlyVals {
		valStatus, err := helpers.ValidatorStatus(val, epoch)
		if err != nil {
			httputil.HandleError(w, "Could not get validator status: "+err.Error(), http.StatusInternalServerError)
			return
		}
		valSubStatus, err := helpers.ValidatorSubStatus(val, epoch)
		if err != nil {
			httputil.HandleError(w, "Could not get validator status: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if filteredStatuses[valStatus] || filteredStatuses[valSubStatus] {
			var container *structs.ValidatorContainer
			id := primitives.ValidatorIndex(i)
			if len(ids) > 0 {
				id = ids[i]
			}
			balance, err := st.BalanceAtIndex(id)
			if err != nil {
				httputil.HandleError(w, "Could not get validator balance: "+err.Error(), http.StatusInternalServerError)
				return
			}
			container = valContainerFromReadOnlyVal(val, id, balance, valSubStatus)
			valContainers = append(valContainers, container)
		}
	}

	resp := &structs.GetValidatorsResponse{
		Data:                valContainers,
		ExecutionOptimistic: isOptimistic,
		Finalized:           isFinalized,
	}
	httputil.WriteJson(w, resp)
}

// GetValidator returns a validator specified by state and id or public key along with status and balance.
func (s *Server) GetValidator(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetValidator")
	defer span.End()

	stateId := r.PathValue("state_id")
	if stateId == "" {
		httputil.HandleError(w, "state_id is required in URL params", http.StatusBadRequest)
		return
	}
	valId := r.PathValue("validator_id")
	if valId == "" {
		httputil.HandleError(w, "validator_id is required in URL params", http.StatusBadRequest)
		return
	}

	st, err := s.Stater.State(ctx, []byte(stateId))
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}
	ids, ok := decodeIds(w, st, []string{valId}, false /* ignore unknown */)
	if !ok {
		return
	}
	readOnlyVals, ok := valsFromIds(w, st, ids)
	if !ok {
		return
	}
	if len(ids) == 0 || len(readOnlyVals) == 0 {
		httputil.HandleError(w, "No validator returned for the given ID", http.StatusInternalServerError)
		return
	}
	valSubStatus, err := helpers.ValidatorSubStatus(readOnlyVals[0], slots.ToEpoch(st.Slot()))
	if err != nil {
		httputil.HandleError(w, "Could not get validator status: "+err.Error(), http.StatusInternalServerError)
		return
	}
	bal, err := st.BalanceAtIndex(ids[0])
	if err != nil {
		httputil.HandleError(w, "Could not get validator balance: "+err.Error(), http.StatusInternalServerError)
		return
	}
	container := valContainerFromReadOnlyVal(readOnlyVals[0], ids[0], bal, valSubStatus)

	isOptimistic, err := helpers.IsOptimistic(ctx, []byte(stateId), s.OptimisticModeFetcher, s.Stater, s.ChainInfoFetcher, s.BeaconDB)
	if err != nil {
		helpers.HandleIsOptimisticError(w, err)
		return
	}
	blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "Could not calculate root of latest block header: "+err.Error(), http.StatusInternalServerError)
		return
	}
	isFinalized := s.FinalizationFetcher.IsFinalized(ctx, blockRoot)

	resp := &structs.GetValidatorResponse{
		Data:                container,
		ExecutionOptimistic: isOptimistic,
		Finalized:           isFinalized,
	}
	httputil.WriteJson(w, resp)
}

// GetValidatorBalances returns a filterable list of validator balances.
func (s *Server) GetValidatorBalances(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetValidatorBalances")
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

	isOptimistic, err := helpers.IsOptimistic(ctx, []byte(stateId), s.OptimisticModeFetcher, s.Stater, s.ChainInfoFetcher, s.BeaconDB)
	if err != nil {
		helpers.HandleIsOptimisticError(w, err)
		return
	}
	blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "Could not calculate root of latest block header: "+err.Error(), http.StatusInternalServerError)
		return
	}
	isFinalized := s.FinalizationFetcher.IsFinalized(ctx, blockRoot)

	var rawIds []string
	if r.Method == http.MethodGet {
		rawIds = r.URL.Query()["id"]
	} else {
		err = json.NewDecoder(r.Body).Decode(&rawIds)
		switch {
		case errors.Is(err, io.EOF):
			httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
			return
		case err != nil:
			httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	ids, ok := decodeIds(w, st, rawIds, true /* ignore unknown */)
	if !ok {
		return
	}
	// return no data if all IDs are ignored
	if len(rawIds) > 0 && len(ids) == 0 {
		resp := &structs.GetValidatorBalancesResponse{
			Data:                []*structs.ValidatorBalance{},
			ExecutionOptimistic: isOptimistic,
			Finalized:           isFinalized,
		}
		httputil.WriteJson(w, resp)
		return
	}

	bals := st.Balances()
	var valBalances []*structs.ValidatorBalance
	if len(ids) == 0 {
		valBalances = make([]*structs.ValidatorBalance, len(bals))
		for i, b := range bals {
			valBalances[i] = &structs.ValidatorBalance{
				Index:   strconv.FormatUint(uint64(i), 10),
				Balance: strconv.FormatUint(b, 10),
			}
		}
	} else {
		valBalances = make([]*structs.ValidatorBalance, len(ids))
		for i, id := range ids {
			valBalances[i] = &structs.ValidatorBalance{
				Index:   strconv.FormatUint(uint64(id), 10),
				Balance: strconv.FormatUint(bals[id], 10),
			}
		}
	}

	resp := &structs.GetValidatorBalancesResponse{
		Data:                valBalances,
		ExecutionOptimistic: isOptimistic,
		Finalized:           isFinalized,
	}
	httputil.WriteJson(w, resp)
}

// GetValidatorIdentities returns a filterable list of validators identities.
func (s *Server) GetValidatorIdentities(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetValidatorIdentities")
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

	var rawIds []string
	err = json.NewDecoder(r.Body).Decode(&rawIds)
	switch {
	case errors.Is(err, io.EOF):
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	ids, ok := decodeIds(w, st, rawIds, true /* ignore unknown */)
	if !ok {
		return
	}

	if httputil.RespondWithSsz(r) {
		s.getValidatorIdentitiesSSZ(w, st, rawIds, ids)
	} else {
		s.getValidatorIdentitiesJSON(r.Context(), w, st, stateId, rawIds, ids)
	}
}

func (s *Server) getValidatorIdentitiesSSZ(w http.ResponseWriter, st state.BeaconState, rawIds []string, ids []primitives.ValidatorIndex) {
	// return no data if all IDs are ignored
	if len(rawIds) > 0 && len(ids) == 0 {
		httputil.WriteSsz(w, []byte{})
		return
	}

	vals := st.ValidatorsReadOnly()
	var identities []*eth.ValidatorIdentity
	if len(ids) == 0 {
		identities = make([]*eth.ValidatorIdentity, len(vals))
		for i, v := range vals {
			pubkey := v.PublicKey()
			identities[i] = &eth.ValidatorIdentity{
				Index:           primitives.ValidatorIndex(i),
				Pubkey:          pubkey[:],
				ActivationEpoch: v.ActivationEpoch(),
			}
		}
	} else {
		identities = make([]*eth.ValidatorIdentity, len(ids))
		for i, id := range ids {
			pubkey := vals[id].PublicKey()
			identities[i] = &eth.ValidatorIdentity{
				Index:           id,
				Pubkey:          pubkey[:],
				ActivationEpoch: vals[id].ActivationEpoch(),
			}
		}
	}

	sszLen := (&eth.ValidatorIdentity{}).SizeSSZ()
	resp := make([]byte, len(identities)*sszLen)
	for i, vi := range identities {
		ssz, err := vi.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, "Could not marshal validator identity to SSZ: "+err.Error(), http.StatusInternalServerError)
			return
		}
		copy(resp[i*sszLen:(i+1)*sszLen], ssz)
	}
	httputil.WriteSsz(w, resp)
}

func (s *Server) getValidatorIdentitiesJSON(
	ctx context.Context,
	w http.ResponseWriter,
	st state.BeaconState,
	stateId string,
	rawIds []string,
	ids []primitives.ValidatorIndex,
) {
	isOptimistic, err := helpers.IsOptimistic(ctx, []byte(stateId), s.OptimisticModeFetcher, s.Stater, s.ChainInfoFetcher, s.BeaconDB)
	if err != nil {
		helpers.HandleIsOptimisticError(w, err)
		return
	}
	blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "Could not calculate root of latest block header: "+err.Error(), http.StatusInternalServerError)
		return
	}
	isFinalized := s.FinalizationFetcher.IsFinalized(ctx, blockRoot)

	// return no data if all IDs are ignored
	if len(rawIds) > 0 && len(ids) == 0 {
		resp := &structs.GetValidatorIdentitiesResponse{
			Data:                []*structs.ValidatorIdentity{},
			ExecutionOptimistic: isOptimistic,
			Finalized:           isFinalized,
		}
		httputil.WriteJson(w, resp)
		return
	}

	vals := st.ValidatorsReadOnly()
	var identities []*structs.ValidatorIdentity
	if len(ids) == 0 {
		identities = make([]*structs.ValidatorIdentity, len(vals))
		for i, v := range vals {
			pubkey := v.PublicKey()
			identities[i] = &structs.ValidatorIdentity{
				Index:           strconv.FormatUint(uint64(i), 10),
				Pubkey:          hexutil.Encode(pubkey[:]),
				ActivationEpoch: strconv.FormatUint(uint64(v.ActivationEpoch()), 10),
			}
		}
	} else {
		identities = make([]*structs.ValidatorIdentity, len(ids))
		for i, id := range ids {
			pubkey := vals[id].PublicKey()
			identities[i] = &structs.ValidatorIdentity{
				Index:           strconv.FormatUint(uint64(id), 10),
				Pubkey:          hexutil.Encode(pubkey[:]),
				ActivationEpoch: strconv.FormatUint(uint64(vals[id].ActivationEpoch()), 10),
			}
		}
	}

	resp := &structs.GetValidatorIdentitiesResponse{
		Data:                identities,
		ExecutionOptimistic: isOptimistic,
		Finalized:           isFinalized,
	}
	httputil.WriteJson(w, resp)
}

// decodeIds takes in a list of validator ID strings (as either a pubkey or a validator index)
// and returns the corresponding validator indices. It can be configured to ignore well-formed but unknown indices.
func decodeIds(w http.ResponseWriter, st state.BeaconState, rawIds []string, ignoreUnknown bool) ([]primitives.ValidatorIndex, bool) {
	ids := make([]primitives.ValidatorIndex, 0, len(rawIds))
	numVals := uint64(st.NumValidators())
	for _, rawId := range rawIds {
		pubkey, err := hexutil.Decode(rawId)
		if err == nil {
			if len(pubkey) != fieldparams.BLSPubkeyLength {
				httputil.HandleError(w, fmt.Sprintf("Pubkey length is %d instead of %d", len(pubkey), fieldparams.BLSPubkeyLength), http.StatusBadRequest)
				return nil, false
			}
			valIndex, ok := st.ValidatorIndexByPubkey(bytesutil.ToBytes48(pubkey))
			if !ok {
				if ignoreUnknown {
					continue
				}
				httputil.HandleError(w, fmt.Sprintf("Unknown validator: %s", hexutil.Encode(pubkey)), http.StatusNotFound)
				return nil, false
			}
			ids = append(ids, valIndex)
			continue
		}

		index, err := strconv.ParseUint(rawId, 10, 64)
		if err != nil {
			httputil.HandleError(w, fmt.Sprintf("Invalid validator index %s", rawId), http.StatusBadRequest)
			return nil, false
		}
		if index >= numVals {
			if ignoreUnknown {
				continue
			}
			httputil.HandleError(w, fmt.Sprintf("Invalid validator index %d", index), http.StatusBadRequest)
			return nil, false
		}
		ids = append(ids, primitives.ValidatorIndex(index))
	}
	return ids, true
}

// valsFromIds returns read-only validators based on the supplied validator indices.
func valsFromIds(w http.ResponseWriter, st state.BeaconState, ids []primitives.ValidatorIndex) ([]state.ReadOnlyValidator, bool) {
	var vals []state.ReadOnlyValidator
	if len(ids) == 0 {
		vals = st.ValidatorsReadOnly()
	} else {
		vals = make([]state.ReadOnlyValidator, 0, len(ids))
		for _, id := range ids {
			val, err := st.ValidatorAtIndex(id)
			if err != nil {
				httputil.HandleError(w, fmt.Sprintf("Could not get validator at index %d: %s", id, err.Error()), http.StatusInternalServerError)
				return nil, false
			}

			readOnlyVal, err := statenative.NewValidator(val)
			if err != nil {
				httputil.HandleError(w, "Could not convert validator: "+err.Error(), http.StatusInternalServerError)
				return nil, false
			}
			vals = append(vals, readOnlyVal)
		}
	}

	return vals, true
}

func valContainerFromReadOnlyVal(
	val state.ReadOnlyValidator,
	id primitives.ValidatorIndex,
	bal uint64,
	valStatus validator.Status,
) *structs.ValidatorContainer {
	pubkey := val.PublicKey()
	return &structs.ValidatorContainer{
		Index:   strconv.FormatUint(uint64(id), 10),
		Balance: strconv.FormatUint(bal, 10),
		Status:  valStatus.String(),
		Validator: &structs.Validator{
			Pubkey:                     hexutil.Encode(pubkey[:]),
			WithdrawalCredentials:      hexutil.Encode(val.GetWithdrawalCredentials()),
			EffectiveBalance:           strconv.FormatUint(val.EffectiveBalance(), 10),
			Slashed:                    val.Slashed(),
			ActivationEligibilityEpoch: strconv.FormatUint(uint64(val.ActivationEligibilityEpoch()), 10),
			ActivationEpoch:            strconv.FormatUint(uint64(val.ActivationEpoch()), 10),
			ExitEpoch:                  strconv.FormatUint(uint64(val.ExitEpoch()), 10),
			WithdrawableEpoch:          strconv.FormatUint(uint64(val.WithdrawableEpoch()), 10),
		},
	}
}
