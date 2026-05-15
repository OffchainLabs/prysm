package validator

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/builder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	rpchelpers "github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	validator2 "github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	mvslice "github.com/OffchainLabs/prysm/v7/container/multi-value-slice"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpbalpha "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation/aggregation/attestations"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

// GetAggregateAttestationV2 aggregates all attestations matching the given attestation data root and slot, returning the aggregated result.
func (s *Server) GetAggregateAttestationV2(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "validator.GetAggregateAttestationV2")
	defer span.End()

	_, attDataRoot, ok := shared.HexFromQuery(w, r, "attestation_data_root", fieldparams.RootLength, true)
	if !ok {
		return
	}
	_, slot, ok := shared.UintFromQuery(w, r, "slot", true)
	if !ok {
		return
	}
	_, index, ok := shared.UintFromQuery(w, r, "committee_index", true)
	if !ok {
		return
	}

	v := slots.ToForkVersion(primitives.Slot(slot))
	agg := s.aggregatedAttestation(w, primitives.Slot(slot), attDataRoot, primitives.CommitteeIndex(index))
	if agg == nil {
		return
	}

	if httputil.RespondWithSsz(r) {
		var data []byte
		var err error
		if v >= version.Electra {
			typedAgg, ok := agg.(*ethpbalpha.AttestationElectra)
			if !ok {
				httputil.HandleError(w, fmt.Sprintf("Attestation is not of type %T", &ethpbalpha.AttestationElectra{}), http.StatusInternalServerError)
				return
			}
			data, err = typedAgg.MarshalSSZ()
			if err != nil {
				httputil.HandleError(w, "Could not marshal attestation: "+err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			typedAgg, ok := agg.(*ethpbalpha.Attestation)
			if !ok {
				httputil.HandleError(w, fmt.Sprintf("Attestation is not of type %T", &ethpbalpha.Attestation{}), http.StatusInternalServerError)
				return
			}
			data, err = typedAgg.MarshalSSZ()
			if err != nil {
				httputil.HandleError(w, "Could not marshal attestation: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set(api.VersionHeader, version.String(v))
		httputil.WriteSsz(w, data)
		return
	}

	resp := &structs.AggregateAttestationResponse{
		Version: version.String(v),
	}
	if v >= version.Electra {
		typedAgg, ok := agg.(*ethpbalpha.AttestationElectra)
		if !ok {
			httputil.HandleError(w, fmt.Sprintf("Attestation is not of type %T", &ethpbalpha.AttestationElectra{}), http.StatusInternalServerError)
			return
		}
		data, err := json.Marshal(structs.AttElectraFromConsensus(typedAgg))
		if err != nil {
			httputil.HandleError(w, "Could not marshal attestation: "+err.Error(), http.StatusInternalServerError)
			return
		}
		resp.Data = data
	} else {
		typedAgg, ok := agg.(*ethpbalpha.Attestation)
		if !ok {
			httputil.HandleError(w, fmt.Sprintf("Attestation is not of type %T", &ethpbalpha.Attestation{}), http.StatusInternalServerError)
			return
		}
		data, err := json.Marshal(structs.AttFromConsensus(typedAgg))
		if err != nil {
			httputil.HandleError(w, "Could not marshal attestation: "+err.Error(), http.StatusInternalServerError)
			return
		}
		resp.Data = data
	}
	w.Header().Set(api.VersionHeader, version.String(v))
	httputil.WriteJson(w, resp)
}

func (s *Server) aggregatedAttestation(w http.ResponseWriter, slot primitives.Slot, attDataRoot []byte, index primitives.CommitteeIndex) ethpbalpha.Att {
	var match []ethpbalpha.Att
	var err error

	if features.Get().EnableExperimentalAttestationPool {
		match, err = matchingAtts(s.AttestationCache.GetAll(), slot, attDataRoot, index)
		if err != nil {
			httputil.HandleError(w, "Could not get matching attestations: "+err.Error(), http.StatusInternalServerError)
			return nil
		}
	} else {
		match, err = matchingAtts(s.AttestationsPool.AggregatedAttestations(), slot, attDataRoot, index)
		if err != nil {
			httputil.HandleError(w, "Could not get matching attestations: "+err.Error(), http.StatusInternalServerError)
			return nil
		}
	}

	if len(match) > 0 {
		// If there are multiple matching aggregated attestations,
		// then we return the one with the most aggregation bits.
		slices.SortFunc(match, func(a, b ethpbalpha.Att) int {
			return cmp.Compare(b.GetAggregationBits().Count(), a.GetAggregationBits().Count())
		})
		return match[0]
	}

	// No match was found and the new pool doesn't store aggregated and unaggregated attestations separately.
	if features.Get().EnableExperimentalAttestationPool {
		return nil
	}

	atts := s.AttestationsPool.UnaggregatedAttestations()
	match, err = matchingAtts(atts, slot, attDataRoot, index)
	if err != nil {
		httputil.HandleError(w, "Could not get matching attestations: "+err.Error(), http.StatusInternalServerError)
		return nil
	}
	if len(match) == 0 {
		httputil.HandleError(w, "No matching attestations found", http.StatusNotFound)
		return nil
	}
	agg, err := attestations.Aggregate(match)
	if err != nil {
		httputil.HandleError(w, "Could not aggregate unaggregated attestations: "+err.Error(), http.StatusInternalServerError)
		return nil
	}

	// Aggregating unaggregated attestations will in theory always return just one aggregate,
	// so we can take the first one and be done with it.
	return agg[0]
}

func matchingAtts(atts []ethpbalpha.Att, slot primitives.Slot, attDataRoot []byte, index primitives.CommitteeIndex) ([]ethpbalpha.Att, error) {
	if len(atts) == 0 {
		return []ethpbalpha.Att{}, nil
	}

	postElectra := slots.ToForkVersion(slot) >= version.Electra
	result := make([]ethpbalpha.Att, 0)
	for _, att := range atts {
		if att.GetData().Slot != slot {
			continue
		}

		root, err := att.GetData().HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "could not get attestation data root")
		}
		if !bytes.Equal(root[:], attDataRoot) {
			continue
		}

		// We ignore the committee index from the request before Electra.
		// This is because before Electra the committee index is part of the attestation data,
		// meaning that comparing the data root is sufficient.
		// Post-Electra the committee index in the data root is always 0, so we need to
		// compare the committee index separately.
		if (!postElectra && att.Version() < version.Electra) || (postElectra && att.Version() >= version.Electra && att.GetCommitteeIndex() == index) {
			result = append(result, att)
		}
	}

	return result, nil
}

// SubmitContributionAndProofs publishes multiple signed sync committee contribution and proofs.
func (s *Server) SubmitContributionAndProofs(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.SubmitContributionAndProofs")
	defer span.End()

	var reqData []json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		if errors.Is(err, io.EOF) {
			httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		} else {
			httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		}
		return
	}
	if len(reqData) == 0 {
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	}

	var failures []*server.IndexedError
	var failedBroadcasts []*server.IndexedError

	for i, item := range reqData {
		var contribution structs.SignedContributionAndProof
		if err := json.Unmarshal(item, &contribution); err != nil {
			failures = append(failures, &server.IndexedError{
				Index:   i,
				Message: "Could not unmarshal message: " + err.Error(),
			})
			continue
		}
		consensusItem, err := contribution.ToConsensus()
		if err != nil {
			failures = append(failures, &server.IndexedError{
				Index:   i,
				Message: "Could not convert request contribution to consensus contribution: " + err.Error(),
			})
			continue
		}

		rpcError := s.CoreService.SubmitSignedContributionAndProof(ctx, consensusItem)
		if rpcError != nil {
			var broadcastFailedErr *server.BroadcastFailedError
			if errors.As(rpcError.Err, &broadcastFailedErr) {
				failedBroadcasts = append(failedBroadcasts, &server.IndexedError{
					Index:   i,
					Message: rpcError.Err.Error(),
				})
				continue
			} else {
				httputil.HandleError(w, rpcError.Err.Error(), core.ErrorReasonToHTTP(rpcError.Reason))
				return
			}
		}
	}

	if len(failures) > 0 {
		failuresErr := &server.IndexedErrorContainer{
			Code:     http.StatusBadRequest,
			Message:  server.ErrIndexedValidationFail,
			Failures: failures,
		}
		httputil.WriteError(w, failuresErr)
		return
	}
	if len(failedBroadcasts) > 0 {
		failuresErr := &server.IndexedErrorContainer{
			Code:     http.StatusInternalServerError,
			Message:  server.ErrIndexedBroadcastFail,
			Failures: failedBroadcasts,
		}
		httputil.WriteError(w, failuresErr)
		return
	}
}

// SubmitAggregateAndProofsV2 verifies given aggregate and proofs and publishes them on appropriate gossipsub topic.
func (s *Server) SubmitAggregateAndProofsV2(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.SubmitAggregateAndProofsV2")
	defer span.End()

	var reqData []json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		if errors.Is(err, io.EOF) {
			httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		} else {
			httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		}
		return
	}
	if len(reqData) == 0 {
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	}

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

	var failures []*server.IndexedError
	var failedBroadcasts []*server.IndexedError

	var rpcError *core.RpcError
	for i, raw := range reqData {
		if v >= version.Electra {
			var signedAggregate structs.SignedAggregateAttestationAndProofElectra
			err = json.Unmarshal(raw, &signedAggregate)
			if err != nil {
				failures = append(failures, &server.IndexedError{
					Index:   i,
					Message: "Could not parse message: " + err.Error(),
				})
				continue
			}
			consensusItem, err := signedAggregate.ToConsensus()
			if err != nil {
				failures = append(failures, &server.IndexedError{
					Index:   i,
					Message: "Could not convert request aggregate to consensus aggregate: " + err.Error(),
				})
				continue
			}
			rpcError = s.CoreService.SubmitSignedAggregateSelectionProof(ctx, consensusItem)
		} else {
			var signedAggregate structs.SignedAggregateAttestationAndProof
			err = json.Unmarshal(raw, &signedAggregate)
			if err != nil {
				failures = append(failures, &server.IndexedError{
					Index:   i,
					Message: "Could not parse message: " + err.Error(),
				})
				continue
			}
			consensusItem, err := signedAggregate.ToConsensus()
			if err != nil {
				failures = append(failures, &server.IndexedError{
					Index:   i,
					Message: "Could not convert request aggregate to consensus aggregate: " + err.Error(),
				})
				continue
			}
			rpcError = s.CoreService.SubmitSignedAggregateSelectionProof(ctx, consensusItem)
		}

		if rpcError != nil {
			var broadcastFailedErr *server.BroadcastFailedError
			if errors.As(rpcError.Err, &broadcastFailedErr) {
				failedBroadcasts = append(failedBroadcasts, &server.IndexedError{
					Index:   i,
					Message: rpcError.Err.Error(),
				})
				continue
			} else {
				httputil.HandleError(w, rpcError.Err.Error(), core.ErrorReasonToHTTP(rpcError.Reason))
				return
			}
		}
	}

	if len(failures) > 0 {
		failuresErr := &server.IndexedErrorContainer{
			Code:     http.StatusBadRequest,
			Message:  server.ErrIndexedValidationFail,
			Failures: failures,
		}
		httputil.WriteError(w, failuresErr)
		return
	}
	if len(failedBroadcasts) > 0 {
		failuresErr := &server.IndexedErrorContainer{
			Code:     http.StatusInternalServerError,
			Message:  server.ErrIndexedBroadcastFail,
			Failures: failedBroadcasts,
		}
		httputil.WriteError(w, failuresErr)
		return
	}
}

// SubmitSyncCommitteeSubscription subscribe to a number of sync committee subnets.
//
// Subscribing to sync committee subnets is an action performed by VC to enable
// network participation, and only required if the VC has an active
// validator in an active sync committee.
func (s *Server) SubmitSyncCommitteeSubscription(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.SubmitSyncCommitteeSubscription")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	var req structs.SubmitSyncCommitteeSubscriptionsRequest
	err := json.NewDecoder(r.Body).Decode(&req.Data)
	switch {
	case errors.Is(err, io.EOF):
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Data) == 0 {
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	}

	st, err := s.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		httputil.HandleError(w, "Could not get head state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	currEpoch := slots.ToEpoch(st.Slot())
	validators := make([]state.ReadOnlyValidator, len(req.Data))
	subscriptions := make([]*validator2.SyncCommitteeSubscription, len(req.Data))
	for i, item := range req.Data {
		consensusItem, err := item.ToConsensus()
		if err != nil {
			httputil.HandleError(w, "Could not convert request subscription to consensus subscription: "+err.Error(), http.StatusBadRequest)
			return
		}
		subscriptions[i] = consensusItem
		val, err := st.ValidatorAtIndexReadOnly(consensusItem.ValidatorIndex)
		if err != nil {
			httputil.HandleError(
				w,
				fmt.Sprintf("Could not get validator at index %d: %s", consensusItem.ValidatorIndex, err.Error()),
				http.StatusInternalServerError,
			)
			return
		}
		valStatus, err := rpchelpers.ValidatorSubStatus(val, currEpoch)
		if err != nil {
			httputil.HandleError(
				w,
				fmt.Sprintf("Could not get validator status at index %d: %s", consensusItem.ValidatorIndex, err.Error()),
				http.StatusInternalServerError,
			)
			return
		}
		if valStatus != validator2.ActiveOngoing && valStatus != validator2.ActiveExiting {
			httputil.HandleError(
				w,
				fmt.Sprintf("Validator at index %d is not active or exiting", consensusItem.ValidatorIndex),
				http.StatusBadRequest,
			)
			return
		}
		validators[i] = val
	}

	startEpoch, err := slots.SyncCommitteePeriodStartEpoch(currEpoch)
	if err != nil {
		httputil.HandleError(w, "Could not get sync committee period start epoch: "+err.Error(), http.StatusInternalServerError)
		return
	}

	for i, sub := range subscriptions {
		if sub.UntilEpoch <= currEpoch {
			httputil.HandleError(
				w,
				fmt.Sprintf("Epoch for subscription at index %d is in the past. It must be at least %d", i, currEpoch+1),
				http.StatusBadRequest,
			)
			return
		}
		maxValidUntilEpoch := startEpoch + params.BeaconConfig().EpochsPerSyncCommitteePeriod*2
		if sub.UntilEpoch > maxValidUntilEpoch {
			httputil.HandleError(
				w,
				fmt.Sprintf("Epoch for subscription at index %d is too far in the future. It can be at most %d", i, maxValidUntilEpoch),
				http.StatusBadRequest,
			)
			return
		}
	}

	for i, sub := range subscriptions {
		pubkey48 := validators[i].PublicKey()
		// Handle overflow in the event current epoch is less than end epoch.
		// This is an impossible condition, so it is a defensive check.
		epochsToWatch, err := sub.UntilEpoch.SafeSub(uint64(startEpoch))
		if err != nil {
			epochsToWatch = 0
		}
		epochDuration := time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot)) * time.Second
		totalDuration := epochDuration * time.Duration(epochsToWatch)

		subcommitteeSize := params.BeaconConfig().SyncCommitteeSize / params.BeaconConfig().SyncCommitteeSubnetCount
		seen := make(map[uint64]bool)
		var subnetIndices []uint64

		for _, idx := range sub.SyncCommitteeIndices {
			subnetIdx := idx / subcommitteeSize
			if !seen[subnetIdx] {
				seen[subnetIdx] = true
				subnetIndices = append(subnetIndices, subnetIdx)
			}
		}
		cache.SyncSubnetIDs.AddSyncCommitteeSubnets(pubkey48[:], startEpoch, subnetIndices, totalDuration)
	}
}

// SubmitBeaconCommitteeSubscription searches using discv5 for peers related to the provided subnet information
// and replaces current peers with those ones if necessary.
func (s *Server) SubmitBeaconCommitteeSubscription(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.SubmitBeaconCommitteeSubscription")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	var req structs.SubmitBeaconCommitteeSubscriptionsRequest
	err := json.NewDecoder(r.Body).Decode(&req.Data)
	switch {
	case errors.Is(err, io.EOF):
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Data) == 0 {
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	}

	st, err := s.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		httputil.HandleError(w, "Could not get head state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Verify validators at the beginning to return early if request is invalid.
	validators := make([]state.ReadOnlyValidator, len(req.Data))
	subscriptions := make([]*validator2.BeaconCommitteeSubscription, len(req.Data))
	for i, item := range req.Data {
		consensusItem, err := item.ToConsensus()
		if err != nil {
			httputil.HandleError(w, "Could not convert request subscription to consensus subscription: "+err.Error(), http.StatusBadRequest)
			return
		}
		subscriptions[i] = consensusItem
		val, err := st.ValidatorAtIndexReadOnly(consensusItem.ValidatorIndex)
		if err != nil {
			if errors.Is(err, mvslice.ErrOutOfBounds) {
				httputil.HandleError(w, "Could not get validator: "+err.Error(), http.StatusBadRequest)
				return
			}
			httputil.HandleError(w, "Could not get validator: "+err.Error(), http.StatusInternalServerError)
			return
		}
		validators[i] = val
		if s.SubscribedValidatorsCache != nil {
			s.SubscribedValidatorsCache.Add(consensusItem.ValidatorIndex)
		}
	}

	fetchValsLen := func(slot primitives.Slot) (uint64, error) {
		wantedEpoch := slots.ToEpoch(slot)
		vals, err := s.HeadFetcher.HeadValidatorsIndices(ctx, wantedEpoch)
		if err != nil {
			return 0, err
		}
		return uint64(len(vals)), nil
	}

	// Request the head validator indices of epoch represented by the first requested slot.
	currValsLen, err := fetchValsLen(subscriptions[0].Slot)
	if err != nil {
		httputil.HandleError(w, "Could not retrieve head validator length: "+err.Error(), http.StatusInternalServerError)
		return
	}
	currEpoch := slots.ToEpoch(subscriptions[0].Slot)
	for _, sub := range subscriptions {
		// If epoch has changed, re-request active validators length
		if currEpoch != slots.ToEpoch(sub.Slot) {
			currValsLen, err = fetchValsLen(sub.Slot)
			if err != nil {
				httputil.HandleError(w, "Could not retrieve head validator length: "+err.Error(), http.StatusInternalServerError)
				return
			}
			currEpoch = slots.ToEpoch(sub.Slot)
		}
		subnet := helpers.ComputeSubnetFromCommitteeAndSlot(currValsLen, sub.CommitteeIndex, sub.Slot)
		cache.SubnetIDs.AddAttesterSubnetID(sub.Slot, subnet)
		if sub.IsAggregator {
			cache.SubnetIDs.AddAggregatorSubnetID(sub.Slot, subnet)
		}
	}
}

// GetAttestationData requests that the beacon node produces attestation data for
// the requested committee index and slot based on the nodes current head.
func (s *Server) GetAttestationData(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.GetAttestationData")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	isPostGloas := slots.ToEpoch(s.TimeFetcher.CurrentSlot()) >= params.BeaconConfig().GloasForkEpoch

	_, slot, ok := shared.UintFromQuery(w, r, "slot", true)
	if !ok {
		return
	}
	_, committeeIndex, ok := shared.UintFromQuery(w, r, "committee_index", !isPostGloas)
	if !ok {
		return
	}

	attestationData, rpcError := s.CoreService.GetAttestationData(ctx, &ethpbalpha.AttestationDataRequest{
		Slot:           primitives.Slot(slot),
		CommitteeIndex: primitives.CommitteeIndex(committeeIndex),
	})

	if rpcError != nil {
		httputil.HandleError(w, rpcError.Err.Error(), core.ErrorReasonToHTTP(rpcError.Reason))
		return
	}

	if httputil.RespondWithSsz(r) {
		data, err := attestationData.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, "Could not marshal attestation data: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, data)
		return
	}

	response := &structs.GetAttestationDataResponse{
		Data: &structs.AttestationData{
			Slot:            strconv.FormatUint(uint64(attestationData.Slot), 10),
			CommitteeIndex:  strconv.FormatUint(uint64(attestationData.CommitteeIndex), 10),
			BeaconBlockRoot: hexutil.Encode(attestationData.BeaconBlockRoot),
			Source: &structs.Checkpoint{
				Epoch: strconv.FormatUint(uint64(attestationData.Source.Epoch), 10),
				Root:  hexutil.Encode(attestationData.Source.Root),
			},
			Target: &structs.Checkpoint{
				Epoch: strconv.FormatUint(uint64(attestationData.Target.Epoch), 10),
				Root:  hexutil.Encode(attestationData.Target.Root),
			},
		},
	}
	httputil.WriteJson(w, response)
}

// ProduceSyncCommitteeContribution requests that the beacon node produce a sync committee contribution.
func (s *Server) ProduceSyncCommitteeContribution(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.ProduceSyncCommitteeContribution")
	defer span.End()

	isOptimistic, err := s.OptimisticModeFetcher.IsOptimistic(ctx)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isOptimistic {
		httputil.HandleError(w, "Beacon node is currently syncing and not serving request on that endpoint", http.StatusServiceUnavailable)
		return
	}

	_, index, ok := shared.UintFromQuery(w, r, "subcommittee_index", true)
	if !ok {
		return
	}
	if index >= params.BeaconConfig().SyncCommitteeSubnetCount {
		httputil.HandleError(w, fmt.Sprintf("Subcommittee index needs to be between 0 and %d, %d is outside of this range.", params.BeaconConfig().SyncCommitteeSubnetCount-1, index), http.StatusBadRequest)
		return
	}
	_, slot, ok := shared.UintFromQuery(w, r, "slot", true)
	if !ok {
		return
	}
	rawBlockRoot := r.URL.Query().Get("beacon_block_root")
	blockRoot, err := hexutil.Decode(rawBlockRoot)
	if err != nil {
		httputil.HandleError(w, "Invalid Beacon Block Root: "+err.Error(), http.StatusBadRequest)
		return
	}
	contribution, ok := s.produceSyncCommitteeContribution(ctx, w, primitives.Slot(slot), index, blockRoot)
	if !ok {
		return
	}
	response := &structs.ProduceSyncCommitteeContributionResponse{
		Data: contribution,
	}
	httputil.WriteJson(w, response)
}

// ProduceSyncCommitteeContribution requests that the beacon node produce a sync committee contribution.
func (s *Server) produceSyncCommitteeContribution(
	ctx context.Context,
	w http.ResponseWriter,
	slot primitives.Slot,
	index uint64,
	blockRoot []byte,
) (*structs.SyncCommitteeContribution, bool) {
	msgs, err := s.SyncCommitteePool.SyncCommitteeMessages(slot)
	if err != nil {
		httputil.HandleError(w, "Could not get sync subcommittee messages: "+err.Error(), http.StatusInternalServerError)
		return nil, false
	}
	if len(msgs) == 0 {
		httputil.HandleError(w, "No subcommittee messages found", http.StatusNotFound)
		return nil, false
	}
	sig, aggregatedBits, err := s.CoreService.AggregatedSigAndAggregationBits(
		ctx,
		&ethpbalpha.AggregatedSigAndAggregationBitsRequest{
			Msgs:      msgs,
			Slot:      slot,
			SubnetId:  index,
			BlockRoot: blockRoot,
		},
	)
	if err != nil {
		httputil.HandleError(w, "Could not get contribution data: "+err.Error(), http.StatusInternalServerError)
		return nil, false
	}

	return &structs.SyncCommitteeContribution{
		Slot:              strconv.FormatUint(uint64(slot), 10),
		BeaconBlockRoot:   hexutil.Encode(blockRoot),
		SubcommitteeIndex: strconv.FormatUint(index, 10),
		AggregationBits:   hexutil.Encode(aggregatedBits),
		Signature:         hexutil.Encode(sig),
	}, true
}

// RegisterValidator requests that the beacon node stores valid validator registrations and calls the builder apis to update the custom builder
func (s *Server) RegisterValidator(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.RegisterValidators")
	defer span.End()

	if slots.ToEpoch(s.TimeFetcher.CurrentSlot()) >= params.BeaconConfig().GloasForkEpoch {
		log.Warn("/eth/v1/validator/register_validator is deprecated post-Gloas; validator clients should use SignedProposerPreferences instead. Request accepted as a no-op.")
		return
	}

	if s.BlockBuilder == nil || !s.BlockBuilder.Configured() {
		httputil.HandleError(w, fmt.Sprintf("Could not register block builder: %v", builder.ErrNoBuilder), http.StatusBadRequest)
		return
	}

	var jsonRegistrations []*structs.SignedValidatorRegistration
	err := json.NewDecoder(r.Body).Decode(&jsonRegistrations)
	switch {
	case errors.Is(err, io.EOF):
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	registrations := make([]*ethpbalpha.SignedValidatorRegistrationV1, len(jsonRegistrations))
	for i, registration := range jsonRegistrations {
		reg, err := registration.ToConsensus()
		if err != nil {
			httputil.HandleError(w, err.Error(), http.StatusBadRequest)
			return
		}

		registrations[i] = reg
	}
	if len(registrations) == 0 {
		httputil.HandleError(w, "Validator registration request is empty", http.StatusBadRequest)
		return
	}
	if err := s.BlockBuilder.RegisterValidator(ctx, registrations); err != nil {
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// PrepareBeaconProposer saves the per-validator fee recipient default. The VC
// switches to SignedProposerPreferences one epoch before Gloas; from that
// point this endpoint is deprecated and accepts requests as a no-op.
func (s *Server) PrepareBeaconProposer(w http.ResponseWriter, r *http.Request) {
	if deprecateGate, err := params.BeaconConfig().GloasForkEpoch.SafeSub(1); err == nil &&
		slots.ToEpoch(s.TimeFetcher.CurrentSlot()) >= deprecateGate {
		log.Warn("/eth/v1/validator/prepare_beacon_proposer is deprecated; use SignedProposerPreferences. Request accepted as a no-op.")
		return
	}
	var jsonFeeRecipients []*structs.FeeRecipient
	err := json.NewDecoder(r.Body).Decode(&jsonFeeRecipients)
	switch {
	case errors.Is(err, io.EOF):
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	for _, r := range jsonFeeRecipients {
		validatorIndex, valid := shared.ValidateUint(w, "validator_index", r.ValidatorIndex)
		if !valid {
			return
		}
		feeRecipientBytes, valid := shared.ValidateHex(w, "fee_recipient", r.FeeRecipient, fieldparams.FeeRecipientLength)
		if !valid {
			return
		}
		feeRecipient := feeRecipientBytes
		if common.BytesToAddress(feeRecipient) == (common.Address{}) {
			feeRecipient = params.BeaconConfig().DefaultFeeRecipient.Bytes()
		}
		s.ProposerPreferencesCache.Set(cache.ProposerPreference{
			ValidatorIndex: primitives.ValidatorIndex(validatorIndex),
			FeeRecipient:   bytesutil.ToBytes20(feeRecipient),
		})
	}
}

// GetAttesterDuties requests the beacon node to provide a set of attestation duties,
// which should be performed by validators, for a particular epoch.
func (s *Server) GetAttesterDuties(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.GetAttesterDuties")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	_, requestedEpochUint, ok := shared.UintFromRoute(w, r, "epoch")
	if !ok {
		return
	}
	requestedEpoch := primitives.Epoch(requestedEpochUint)
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

	cs := s.TimeFetcher.CurrentSlot()
	currentEpoch := slots.ToEpoch(cs)
	nextEpoch := currentEpoch + 1
	if requestedEpoch > nextEpoch {
		httputil.HandleError(
			w,
			fmt.Sprintf("Request epoch %d can not be greater than next epoch %d", requestedEpoch, nextEpoch),
			http.StatusBadRequest,
		)
		return
	}

	// For next epoch requests, we use the current epoch's state since committee
	// assignments for next epoch can be computed from current epoch's state.
	epochForState := requestedEpoch
	if requestedEpoch == nextEpoch {
		epochForState = currentEpoch
	}
	st, err := s.Stater.StateByEpoch(ctx, epochForState)
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}

	coreDuties, rpcErr := s.CoreService.AttesterDuties(ctx, st, requestedEpoch, requestedValIndices)
	if rpcErr != nil {
		httputil.HandleError(w, rpcErr.Err.Error(), core.ErrorReasonToHTTP(rpcErr.Reason))
		return
	}

	duties := make([]*structs.AttesterDuty, 0, len(coreDuties))
	for _, d := range coreDuties {
		duties = append(duties, &structs.AttesterDuty{
			Pubkey:                  hexutil.Encode(d.Pubkey[:]),
			ValidatorIndex:          strconv.FormatUint(uint64(d.ValidatorIndex), 10),
			CommitteeIndex:          strconv.FormatUint(uint64(d.CommitteeIndex), 10),
			CommitteeLength:         strconv.FormatUint(d.CommitteeLength, 10),
			CommitteesAtSlot:        strconv.FormatUint(d.CommitteesAtSlot, 10),
			ValidatorCommitteeIndex: strconv.FormatUint(d.ValidatorCommitteeIndex, 10),
			Slot:                    strconv.FormatUint(uint64(d.Slot), 10),
		})
	}

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

	response := &structs.GetAttesterDutiesResponse{
		DependentRoot:       hexutil.Encode(dependentRoot),
		Data:                duties,
		ExecutionOptimistic: isOptimistic,
	}
	httputil.WriteJson(w, response)
}

// Deprecated: GetProposerDuties requests beacon node to provide all validators that are scheduled to propose a block in the given epoch.
// Use GetProposerDutiesV2 instead, which computes a fork-aware dependent root.
func (s *Server) GetProposerDuties(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.GetProposerDuties")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	_, requestedEpochUint, ok := shared.UintFromRoute(w, r, "epoch")
	if !ok {
		return
	}
	requestedEpoch := primitives.Epoch(requestedEpochUint)

	cs := s.TimeFetcher.CurrentSlot()
	currentEpoch := slots.ToEpoch(cs)
	nextEpoch := currentEpoch + 1
	var nextEpochLookahead bool
	if requestedEpoch > nextEpoch {
		httputil.HandleError(
			w,
			fmt.Sprintf("Request epoch %d can not be greater than next epoch %d", requestedEpoch, currentEpoch+1),
			http.StatusBadRequest,
		)
		return
	} else if requestedEpoch == nextEpoch {
		requestedEpoch = currentEpoch
		nextEpochLookahead = true
	}

	st, err := s.Stater.StateByEpoch(ctx, requestedEpoch)
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}

	dutyEpoch := requestedEpoch
	if nextEpochLookahead {
		dutyEpoch = nextEpoch
	}
	coreDuties, rpcErr := s.CoreService.ProposerDuties(ctx, st, dutyEpoch)
	if rpcErr != nil {
		httputil.HandleError(w, rpcErr.Err.Error(), core.ErrorReasonToHTTP(rpcErr.Reason))
		return
	}

	duties := make([]*structs.ProposerDuty, 0, len(coreDuties))
	for _, d := range coreDuties {
		duties = append(duties, &structs.ProposerDuty{
			Pubkey:         hexutil.Encode(d.Pubkey[:]),
			ValidatorIndex: strconv.FormatUint(uint64(d.ValidatorIndex), 10),
			Slot:           strconv.FormatUint(uint64(d.Slot), 10),
		})
	}

	isOptimistic, err := s.OptimisticModeFetcher.IsOptimistic(ctx)
	if err != nil {
		httputil.HandleError(w, "Could not check optimistic status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var dependentRoot []byte
	if requestedEpoch == 0 {
		root, err := s.BeaconDB.GenesisBlockRoot(ctx)
		if err != nil {
			httputil.HandleError(w, "Could not get genesis block root: "+err.Error(), http.StatusInternalServerError)
			return
		}
		dependentRoot = root[:]
	} else {
		root, err := core.ProposalDependentRoot(st, requestedEpoch)
		if err != nil {
			httputil.HandleError(w, "Could not get dependent root: "+err.Error(), http.StatusInternalServerError)
			return
		}
		dependentRoot = root
	}

	httputil.WriteJson(w, &structs.GetProposerDutiesResponse{
		DependentRoot:       hexutil.Encode(dependentRoot),
		Data:                duties,
		ExecutionOptimistic: isOptimistic,
	})
}

// GetProposerDutiesV2 requests beacon node to provide all validators that are scheduled to propose a block in the given epoch.
// V2 computes a fork-aware dependent root using core.ProposalDependentRootV2.
// Post-Fulu (EIP-7917) this follows the previous-epoch dependency semantics.
func (s *Server) GetProposerDutiesV2(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.GetProposerDutiesV2")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	_, requestedEpochUint, ok := shared.UintFromRoute(w, r, "epoch")
	if !ok {
		return
	}
	requestedEpoch := primitives.Epoch(requestedEpochUint)
	dutiesEpoch := requestedEpoch

	cs := s.TimeFetcher.CurrentSlot()
	currentEpoch := slots.ToEpoch(cs)
	nextEpoch := currentEpoch + 1
	var nextEpochLookahead bool
	if requestedEpoch > nextEpoch {
		httputil.HandleError(
			w,
			fmt.Sprintf("Request epoch %d can not be greater than next epoch %d", requestedEpoch, currentEpoch+1),
			http.StatusBadRequest,
		)
		return
	} else if requestedEpoch == nextEpoch {
		requestedEpoch = currentEpoch
		nextEpochLookahead = true
	}

	st, err := s.Stater.StateByEpoch(ctx, requestedEpoch)
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}

	dutyEpoch := requestedEpoch
	if nextEpochLookahead {
		dutyEpoch = nextEpoch
	}
	coreDuties, rpcErr := s.CoreService.ProposerDuties(ctx, st, dutyEpoch)
	if rpcErr != nil {
		httputil.HandleError(w, rpcErr.Err.Error(), core.ErrorReasonToHTTP(rpcErr.Reason))
		return
	}

	duties := make([]*structs.ProposerDuty, 0, len(coreDuties))
	for _, d := range coreDuties {
		duties = append(duties, &structs.ProposerDuty{
			Pubkey:         hexutil.Encode(d.Pubkey[:]),
			ValidatorIndex: strconv.FormatUint(uint64(d.ValidatorIndex), 10),
			Slot:           strconv.FormatUint(uint64(d.Slot), 10),
		})
	}

	isOptimistic, err := s.OptimisticModeFetcher.IsOptimistic(ctx)
	if err != nil {
		httputil.HandleError(w, "Could not check optimistic status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var dependentRoot []byte
	if dutiesEpoch == 0 || (dutiesEpoch == 1 && st.Version() >= version.Fulu) {
		root, err := s.BeaconDB.GenesisBlockRoot(ctx)
		if err != nil {
			httputil.HandleError(w, "Could not get genesis block root: "+err.Error(), http.StatusInternalServerError)
			return
		}
		dependentRoot = root[:]
	} else {
		root, err := core.ProposalDependentRootV2(st, dutiesEpoch)
		if err != nil {
			httputil.HandleError(w, "Could not get dependent root: "+err.Error(), http.StatusInternalServerError)
			return
		}
		dependentRoot = root
	}

	httputil.WriteJson(w, &structs.GetProposerDutiesResponse{
		DependentRoot:       hexutil.Encode(dependentRoot),
		Data:                duties,
		ExecutionOptimistic: isOptimistic,
	})
}

// GetSyncCommitteeDuties provides a set of sync committee duties for a particular epoch.
//
// The logic for calculating epoch validity comes from https://ethereum.github.io/beacon-APIs/?urls.primaryName=dev#/Validator/getSyncCommitteeDuties
// where `epoch` is described as `epoch // EPOCHS_PER_SYNC_COMMITTEE_PERIOD <= current_epoch // EPOCHS_PER_SYNC_COMMITTEE_PERIOD + 1`.
//
// Algorithm:
//   - Get the last valid epoch. This is the last epoch of the next sync committee period.
//   - Get the state for the requested epoch. If it's a future epoch from the current sync committee period
//     or an epoch from the next sync committee period, then get the current state.
//   - Get the state's current sync committee. If it's an epoch from the next sync committee period, then get the next sync committee.
//   - Get duties.
func (s *Server) GetSyncCommitteeDuties(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.GetSyncCommitteeDuties")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	_, requestedEpochUint, ok := shared.UintFromRoute(w, r, "epoch")
	if !ok {
		return
	}
	requestedEpoch := primitives.Epoch(requestedEpochUint)
	if requestedEpoch < params.BeaconConfig().AltairForkEpoch {
		httputil.HandleError(w, "Sync committees are not supported for Phase0", http.StatusBadRequest)
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
	lastValidEpoch := core.SyncCommitteeDutiesLastValidEpoch(currentEpoch)
	if requestedEpoch > lastValidEpoch {
		httputil.HandleError(w, fmt.Sprintf("Epoch is too far in the future, maximum valid epoch is %d", lastValidEpoch), http.StatusBadRequest)
		return
	}

	currentCommitteeFirstEpoch, err := slots.SyncCommitteePeriodStartEpoch(currentEpoch)
	if err != nil {
		httputil.HandleError(w, "Could not get sync committee period start epoch: "+err.Error(), http.StatusInternalServerError)
		return
	}

	requestedCommitteeFirstEpoch, err := slots.SyncCommitteePeriodStartEpoch(requestedEpoch)
	if err != nil {
		httputil.HandleError(w, "Could not get sync committee period start epoch: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Sync committee assignments are computed at the start of the sync committee period and don't change during the period.
	// - For the current period we use the current epoch to avoid expensive state replays.
	// - For the next period we also use the current epoch and later read NextSyncCommittee (known one period in advance) to avoid fetching a future state.
	// - For a past period we fall back to the first epoch of that period.
	targetEpoch := currentEpoch
	if requestedCommitteeFirstEpoch < currentCommitteeFirstEpoch {
		targetEpoch = requestedCommitteeFirstEpoch
	}

	st, err := s.Stater.StateByEpoch(ctx, targetEpoch)
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}

	coreDuties, rpcErr := s.CoreService.SyncCommitteeDuties(ctx, st, requestedEpoch, currentEpoch, requestedValIndices)
	if rpcErr != nil {
		httputil.HandleError(w, rpcErr.Err.Error(), core.ErrorReasonToHTTP(rpcErr.Reason))
		return
	}

	duties := make([]*structs.SyncCommitteeDuty, 0, len(coreDuties))
	for _, d := range coreDuties {
		syncIndices := make([]string, len(d.ValidatorSyncCommitteeIndices))
		for i, idx := range d.ValidatorSyncCommitteeIndices {
			syncIndices[i] = strconv.FormatUint(idx, 10)
		}
		duties = append(duties, &structs.SyncCommitteeDuty{
			Pubkey:                        hexutil.Encode(d.Pubkey[:]),
			ValidatorIndex:                strconv.FormatUint(uint64(d.ValidatorIndex), 10),
			ValidatorSyncCommitteeIndices: syncIndices,
		})
	}

	isOptimistic, err := s.OptimisticModeFetcher.IsOptimistic(ctx)
	if err != nil {
		httputil.HandleError(w, "Could not check optimistic status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := &structs.GetSyncCommitteeDutiesResponse{
		Data:                duties,
		ExecutionOptimistic: isOptimistic,
	}
	httputil.WriteJson(w, resp)
}

// ptcDuty represents a validator's PTC assignment for a slot.
type ptcDuty struct {
	validatorIndex primitives.ValidatorIndex
	slot           primitives.Slot
}

// ptcDuties returns PTC slot assignments for the requested validators in the given epoch.
// The state must be from an epoch that allows computing PTC assignments for the target epoch.
// Validators not in any PTC for the epoch will not appear in the result.
func ptcDuties(
	ctx context.Context,
	st state.ReadOnlyBeaconState,
	epoch primitives.Epoch,
	validators map[primitives.ValidatorIndex]struct{},
) ([]ptcDuty, error) {
	if len(validators) == 0 || st.Version() < version.Gloas {
		return nil, nil
	}
	startSlot, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, err
	}

	var duties []ptcDuty
	endSlot := startSlot + params.BeaconConfig().SlotsPerEpoch

	for slot := startSlot; slot < endSlot; slot++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		ptc, err := st.PayloadCommitteeReadOnly(slot)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get PTC for slot %d", slot)
		}

		// Check which requested validators are in this PTC, deduplicating within the slot.
		seen := make(map[primitives.ValidatorIndex]struct{})
		for _, valIdx := range ptc {
			if _, ok := validators[valIdx]; !ok {
				continue
			}
			if _, already := seen[valIdx]; already {
				continue
			}
			seen[valIdx] = struct{}{}
			duties = append(duties, ptcDuty{
				validatorIndex: valIdx,
				slot:           slot,
			})
		}
	}

	return duties, nil
}

// GetPTCDuties retrieves the payload timeliness committee (PTC) duties for the requested epoch.
// The PTC is responsible for attesting to payload timeliness in ePBS (Gloas fork and later).
func (s *Server) GetPTCDuties(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.GetPTCDuties")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	_, requestedEpochUint, ok := shared.UintFromRoute(w, r, "epoch")
	if !ok {
		return
	}
	requestedEpoch := primitives.Epoch(requestedEpochUint)

	// PTC duties are only available from Gloas fork onwards.
	if requestedEpoch < params.BeaconConfig().GloasForkEpoch {
		httputil.HandleError(w, "PTC duties are not available before Gloas fork", http.StatusBadRequest)
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

	// Limit how far in the future we can query (current + 1 epoch).
	cs := s.TimeFetcher.CurrentSlot()
	currentEpoch := slots.ToEpoch(cs)
	nextEpoch := currentEpoch.Add(1)
	if requestedEpoch > nextEpoch {
		httputil.HandleError(w,
			fmt.Sprintf("Request epoch %d can not be greater than next epoch %d", requestedEpoch, nextEpoch),
			http.StatusBadRequest)
		return
	}

	// For next epoch requests, we use the current epoch's state since PTC
	// assignments for next epoch can be computed from current epoch's state.
	epochForState := requestedEpoch
	if requestedEpoch == nextEpoch {
		epochForState = currentEpoch
	}
	st, err := s.Stater.StateByEpoch(ctx, epochForState)
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}

	// Build a set of requested validators (also deduplicates per spec's uniqueItems requirement).
	// Validate that each index exists in the validator registry.
	requestedSet := make(map[primitives.ValidatorIndex]struct{}, len(requestedValIndices))
	var zeroPubkey [fieldparams.BLSPubkeyLength]byte
	for _, idx := range requestedValIndices {
		// Skip duplicates.
		if _, exists := requestedSet[idx]; exists {
			continue
		}
		// Validate index exists.
		pubkey := st.PubkeyAtIndex(idx)
		if bytes.Equal(pubkey[:], zeroPubkey[:]) {
			httputil.HandleError(w, fmt.Sprintf("Invalid validator index %d", idx), http.StatusBadRequest)
			return
		}
		requestedSet[idx] = struct{}{}
	}

	// Compute PTC duties.
	computedDuties, err := ptcDuties(ctx, st, requestedEpoch, requestedSet)
	if err != nil {
		httputil.HandleError(w, "Could not compute PTC duties: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert to response format.
	duties := make([]*structs.PTCDuty, 0, len(computedDuties))
	for _, duty := range computedDuties {
		pubkey := st.PubkeyAtIndex(duty.validatorIndex)
		duties = append(duties, &structs.PTCDuty{
			Pubkey:         hexutil.Encode(pubkey[:]),
			ValidatorIndex: strconv.FormatUint(uint64(duty.validatorIndex), 10),
			Slot:           strconv.FormatUint(uint64(duty.slot), 10),
		})
	}

	// Get dependent root. Per the spec, dependent_root is:
	// get_block_root_at_slot(state, compute_start_slot_at_epoch(epoch - 1) - 1)
	// or the genesis block root in the case of underflow.
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

	resp := &structs.GetPTCDutiesResponse{
		DependentRoot:       hexutil.Encode(dependentRoot),
		ExecutionOptimistic: isOptimistic,
		Data:                duties,
	}
	httputil.WriteJson(w, resp)
}

// GetLiveness requests the beacon node to indicate if a validator has been observed to be live in a given epoch.
// The beacon node might detect liveness by observing messages from the validator on the network,
// in the beacon chain, from its API or from any other source.
// A beacon node SHOULD support the current and previous epoch, however it MAY support earlier epoch.
// It is important to note that the values returned by the beacon node are not canonical;
// they are best-effort and based upon a subjective view of the network.
// A beacon node that was recently started or suffered a network partition may indicate that a validator is not live when it actually is.
func (s *Server) GetLiveness(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.GetLiveness")
	defer span.End()

	_, requestedEpochUint, ok := shared.UintFromRoute(w, r, "epoch")
	if !ok {
		return
	}
	requestedEpoch := primitives.Epoch(requestedEpochUint)
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

	// First we check if the requested epoch is the current epoch.
	// If it is, then we won't be able to fetch the state at the end of the epoch.
	// In that case we get participation info from the head state.
	// We can also use the head state to get participation info for the previous epoch.
	headSt, err := s.HeadFetcher.HeadState(ctx)
	if err != nil {
		httputil.HandleError(w, "Could not get head state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	currEpoch := slots.ToEpoch(headSt.Slot())
	if requestedEpoch > currEpoch {
		httputil.HandleError(w, "Requested epoch cannot be in the future", http.StatusBadRequest)
		return
	}

	var st state.BeaconState
	var participation []byte
	if requestedEpoch == currEpoch {
		st = headSt
		participation, err = st.CurrentEpochParticipation()
		if err != nil {
			httputil.HandleError(w, "Could not get current epoch participation: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else if requestedEpoch == currEpoch-1 {
		st = headSt
		participation, err = st.PreviousEpochParticipation()
		if err != nil {
			httputil.HandleError(w, "Could not get previous epoch participation: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		epochEnd, err := slots.EpochEnd(requestedEpoch)
		if err != nil {
			httputil.HandleError(w, "Could not get requested epoch's end slot: "+err.Error(), http.StatusInternalServerError)
			return
		}
		st, err = s.Stater.StateBySlot(ctx, epochEnd)
		if err != nil {
			shared.WriteStateFetchError(w, err)
			return
		}
		participation, err = st.CurrentEpochParticipation()
		if err != nil {
			httputil.HandleError(w, "Could not get current epoch participation: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	resp := &structs.GetLivenessResponse{
		Data: make([]*structs.Liveness, len(requestedValIndices)),
	}
	for i, vi := range requestedValIndices {
		if vi >= primitives.ValidatorIndex(len(participation)) {
			httputil.HandleError(w, fmt.Sprintf("Validator index %d is invalid", vi), http.StatusBadRequest)
			return
		}
		resp.Data[i] = &structs.Liveness{
			Index:  strconv.FormatUint(uint64(vi), 10),
			IsLive: participation[vi] != 0,
		}
	}

	httputil.WriteJson(w, resp)
}

// BeaconCommitteeSelections responds with appropriate message and status code according the spec:
// https://ethereum.github.io/beacon-APIs/#/Validator/submitBeaconCommitteeSelections.
func (s *Server) BeaconCommitteeSelections(w http.ResponseWriter, _ *http.Request) {
	httputil.HandleError(w, "Endpoint not implemented", 501)
}

// SyncCommitteeSelections responds with appropriate message and status code according the spec:
// https://ethereum.github.io/beacon-APIs/#/Validator/submitSyncCommitteeSelections.
func (s *Server) SyncCommitteeSelections(w http.ResponseWriter, _ *http.Request) {
	httputil.HandleError(w, "Endpoint not implemented", 501)
}
