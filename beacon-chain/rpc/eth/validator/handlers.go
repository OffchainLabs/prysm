package validator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/builder"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/transition"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/core"
	rpchelpers "github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/shared"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	consensus_types "github.com/prysmaticlabs/prysm/v5/consensus-types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	validator2 "github.com/prysmaticlabs/prysm/v5/consensus-types/validator"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	ethpbalpha "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetAggregateAttestation aggregates all attestations matching the given attestation data root and slot, returning the aggregated result.
func (s *Server) GetAggregateAttestation(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "validator.GetAggregateAttestation")
	defer span.End()

	_, attDataRoot, ok := shared.HexFromQuery(w, r, "attestation_data_root", fieldparams.RootLength, true)
	if !ok {
		return
	}

	_, slot, ok := shared.UintFromQuery(w, r, "slot", true)
	if !ok {
		return
	}

	var match ethpbalpha.Att
	var err error

	match, err = matchingAtt(s.AttestationsPool.AggregatedAttestations(), primitives.Slot(slot), attDataRoot)
	if err != nil {
		httputil.HandleError(w, "Could not get matching attestation: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if match == nil {
		atts, err := s.AttestationsPool.UnaggregatedAttestations()
		if err != nil {
			httputil.HandleError(w, "Could not get unaggregated attestations: "+err.Error(), http.StatusInternalServerError)
			return
		}
		match, err = matchingAtt(atts, primitives.Slot(slot), attDataRoot)
		if err != nil {
			httputil.HandleError(w, "Could not get matching attestation: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if match == nil {
		httputil.HandleError(w, "No matching attestation found", http.StatusNotFound)
		return
	}

	response := &structs.AggregateAttestationResponse{
		Data: &structs.Attestation{
			AggregationBits: hexutil.Encode(match.GetAggregationBits()),
			Data: &structs.AttestationData{
				Slot:            strconv.FormatUint(uint64(match.GetData().Slot), 10),
				CommitteeIndex:  strconv.FormatUint(uint64(match.GetData().CommitteeIndex), 10),
				BeaconBlockRoot: hexutil.Encode(match.GetData().BeaconBlockRoot),
				Source: &structs.Checkpoint{
					Epoch: strconv.FormatUint(uint64(match.GetData().Source.Epoch), 10),
					Root:  hexutil.Encode(match.GetData().Source.Root),
				},
				Target: &structs.Checkpoint{
					Epoch: strconv.FormatUint(uint64(match.GetData().Target.Epoch), 10),
					Root:  hexutil.Encode(match.GetData().Target.Root),
				},
			},
			Signature: hexutil.Encode(match.GetSignature()),
		}}
	httputil.WriteJson(w, response)
}

func matchingAtt(atts []ethpbalpha.Att, slot primitives.Slot, attDataRoot []byte) (ethpbalpha.Att, error) {
	for _, att := range atts {
		if att.GetData().Slot == slot {
			root, err := att.GetData().HashTreeRoot()
			if err != nil {
				return nil, errors.Wrap(err, "could not get attestation data root")
			}
			if bytes.Equal(root[:], attDataRoot) {
				return att, nil
			}
		}
	}
	return nil, nil
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

	for _, item := range reqData {
		var contribution structs.SignedContributionAndProof
		if err := json.Unmarshal(item, &contribution); err != nil {
			httputil.HandleError(w, "Could not decode item: "+err.Error(), http.StatusBadRequest)
			return
		}
		consensusItem, err := contribution.ToConsensus()
		if err != nil {
			httputil.HandleError(w, "Could not convert contribution to consensus format: "+err.Error(), http.StatusBadRequest)
			return
		}
		if rpcError := s.CoreService.SubmitSignedContributionAndProof(ctx, consensusItem); rpcError != nil {
			httputil.HandleError(w, rpcError.Err.Error(), core.ErrorReasonToHTTP(rpcError.Reason))
			return
		}
	}
}

// SubmitAggregateAndProofs verifies given aggregate and proofs and publishes them on appropriate gossipsub topic.
func (s *Server) SubmitAggregateAndProofs(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.SubmitAggregateAndProofs")
	defer span.End()

	var req structs.SubmitAggregateAndProofsRequest
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

	broadcastFailed := false
	for _, item := range req.Data {
		var signedAggregate structs.SignedAggregateAttestationAndProof
		err := json.Unmarshal(item, &signedAggregate)
		if err != nil {
			httputil.HandleError(w, "Could not decode item: "+err.Error(), http.StatusBadRequest)
			return
		}
		consensusItem, err := signedAggregate.ToConsensus()
		if err != nil {
			httputil.HandleError(w, "Could not convert request aggregate to consensus aggregate: "+err.Error(), http.StatusBadRequest)
			return
		}
		rpcError := s.CoreService.SubmitSignedAggregateSelectionProof(ctx, consensusItem)
		if rpcError != nil {
			var aggregateBroadcastFailedError *core.AggregateBroadcastFailedError
			ok := errors.As(rpcError.Err, &aggregateBroadcastFailedError)
			if ok {
				broadcastFailed = true
			} else {
				httputil.HandleError(w, rpcError.Err.Error(), core.ErrorReasonToHTTP(rpcError.Reason))
				return
			}
		}
	}

	if broadcastFailed {
		httputil.HandleError(w, "Could not broadcast one or more signed aggregated attestations", http.StatusInternalServerError)
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
	}
	v, err := version.FromString(versionHeader)
	if err != nil {
		httputil.HandleError(w, "Invalid version: "+err.Error(), http.StatusBadRequest)
		return
	}

	broadcastFailed := false
	var rpcError *core.RpcError
	for _, raw := range reqData {
		if v >= version.Electra {
			var signedAggregate structs.SignedAggregateAttestationAndProofElectra
			err = json.Unmarshal(raw, &signedAggregate)
			if err != nil {
				httputil.HandleError(w, "Failed to parse aggregate attestation and proof: "+err.Error(), http.StatusBadRequest)
				return
			}
			consensusItem, err := signedAggregate.ToConsensus()
			if err != nil {
				httputil.HandleError(w, "Could not convert request aggregate to consensus aggregate: "+err.Error(), http.StatusBadRequest)
				return
			}
			rpcError = s.CoreService.SubmitSignedAggregateSelectionProof(ctx, consensusItem)
		} else {
			var signedAggregate structs.SignedAggregateAttestationAndProof
			err = json.Unmarshal(raw, &signedAggregate)
			if err != nil {
				httputil.HandleError(w, "Failed to parse aggregate attestation and proof: "+err.Error(), http.StatusBadRequest)
				return
			}
			consensusItem, err := signedAggregate.ToConsensus()
			if err != nil {
				httputil.HandleError(w, "Could not convert request aggregate to consensus aggregate: "+err.Error(), http.StatusBadRequest)
				return
			}
			rpcError = s.CoreService.SubmitSignedAggregateSelectionProof(ctx, consensusItem)
		}

		if rpcError != nil {
			var aggregateBroadcastFailedError *core.AggregateBroadcastFailedError
			if errors.As(rpcError.Err, &aggregateBroadcastFailedError) {
				broadcastFailed = true
			} else {
				httputil.HandleError(w, rpcError.Err.Error(), core.ErrorReasonToHTTP(rpcError.Reason))
				return
			}
		}
	}
	if broadcastFailed {
		httputil.HandleError(w, "Could not broadcast one or more signed aggregated attestations", http.StatusInternalServerError)
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

		cache.SyncSubnetIDs.AddSyncCommitteeSubnets(pubkey48[:], startEpoch, sub.SyncCommitteeIndices, totalDuration)
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
			if errors.Is(err, consensus_types.ErrOutOfBounds) {
				httputil.HandleError(w, "Could not get validator: "+err.Error(), http.StatusBadRequest)
				return
			}
			httputil.HandleError(w, "Could not get validator: "+err.Error(), http.StatusInternalServerError)
			return
		}
		validators[i] = val
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

	_, slot, ok := shared.UintFromQuery(w, r, "slot", true)
	if !ok {
		return
	}
	_, committeeIndex, ok := shared.UintFromQuery(w, r, "committee_index", true)
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

	_, index, ok := shared.UintFromQuery(w, r, "subcommittee_index", true)
	if !ok {
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

// PrepareBeaconProposer endpoint saves the fee recipient given a validator index, this is used when proposing a block.
func (s *Server) PrepareBeaconProposer(w http.ResponseWriter, r *http.Request) {
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
	var validatorIndices []primitives.ValidatorIndex
	// filter for found fee recipients
	for _, r := range jsonFeeRecipients {
		validatorIndex, valid := shared.ValidateUint(w, "validator_index", r.ValidatorIndex)
		if !valid {
			return
		}
		feeRecipientBytes, valid := shared.ValidateHex(w, "fee_recipient", r.FeeRecipient, fieldparams.FeeRecipientLength)
		if !valid {
			return
		}
		// Use default address if the burn address is return
		feeRecipient := primitives.ExecutionAddress(feeRecipientBytes)
		if feeRecipient == primitives.ExecutionAddress([20]byte{}) {
			feeRecipient = primitives.ExecutionAddress(params.BeaconConfig().DefaultFeeRecipient)
			if feeRecipient == primitives.ExecutionAddress([20]byte{}) {
				log.WithField("validatorIndex", validatorIndex).Warn("fee recipient is the burn address")
			}
		}
		val := cache.TrackedValidator{
			Active:       true, // TODO: either check or add the field in the request
			Index:        primitives.ValidatorIndex(validatorIndex),
			FeeRecipient: feeRecipient,
		}
		s.TrackedValidatorsCache.Set(val)
		validatorIndices = append(validatorIndices, primitives.ValidatorIndex(validatorIndex))
	}
	if len(validatorIndices) == 0 {
		return
	}
	log.WithFields(logrus.Fields{
		"validatorIndices": validatorIndices,
	}).Info("Updated fee recipient addresses")
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

	var startSlot primitives.Slot
	if requestedEpoch == nextEpoch {
		startSlot, err = slots.EpochStart(currentEpoch)
	} else {
		startSlot, err = slots.EpochStart(requestedEpoch)
	}
	if err != nil {
		httputil.HandleError(w, fmt.Sprintf("Could not get start slot from epoch %d: %v", requestedEpoch, err), http.StatusInternalServerError)
		return
	}

	st, err := s.Stater.StateBySlot(ctx, startSlot)
	if err != nil {
		httputil.HandleError(w, "Could not get state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	assignments, err := helpers.CommitteeAssignments(ctx, st, requestedEpoch, requestedValIndices)
	if err != nil {
		httputil.HandleError(w, "Could not compute committee assignments: "+err.Error(), http.StatusInternalServerError)
		return
	}
	activeValidatorCount, err := helpers.ActiveValidatorCount(ctx, st, requestedEpoch)
	if err != nil {
		httputil.HandleError(w, "Could not get active validator count: "+err.Error(), http.StatusInternalServerError)
		return
	}
	committeesAtSlot := helpers.SlotCommitteeCount(activeValidatorCount)

	duties := make([]*structs.AttesterDuty, 0, len(requestedValIndices))
	for _, index := range requestedValIndices {
		pubkey := st.PubkeyAtIndex(index)
		var zeroPubkey [fieldparams.BLSPubkeyLength]byte
		if bytes.Equal(pubkey[:], zeroPubkey[:]) {
			httputil.HandleError(w, fmt.Sprintf("Invalid validator index %d", index), http.StatusBadRequest)
			return
		}
		committee := assignments[index]
		if committee == nil {
			continue
		}
		var valIndexInCommittee int
		// valIndexInCommittee will be 0 in case we don't get a match. This is a potential false positive,
		// however it's an impossible condition because every validator must be assigned to a committee.
		for cIndex, vIndex := range committee.Committee {
			if vIndex == index {
				valIndexInCommittee = cIndex
				break
			}
		}
		duties = append(duties, &structs.AttesterDuty{
			Pubkey:                  hexutil.Encode(pubkey[:]),
			ValidatorIndex:          strconv.FormatUint(uint64(index), 10),
			CommitteeIndex:          strconv.FormatUint(uint64(committee.CommitteeIndex), 10),
			CommitteeLength:         strconv.Itoa(len(committee.Committee)),
			CommitteesAtSlot:        strconv.FormatUint(committeesAtSlot, 10),
			ValidatorCommitteeIndex: strconv.Itoa(valIndexInCommittee),
			Slot:                    strconv.FormatUint(uint64(committee.AttesterSlot), 10),
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
		dependentRoot, err = attestationDependentRoot(st, requestedEpoch)
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

// GetProposerDuties requests beacon node to provide all validators that are scheduled to propose a block in the given epoch.
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
		// If the request is for the next epoch, we use the current epoch's state to compute duties.
		requestedEpoch = currentEpoch
		nextEpochLookahead = true
	}

	epochStartSlot, err := slots.EpochStart(requestedEpoch)
	if err != nil {
		httputil.HandleError(w, fmt.Sprintf("Could not get start slot of epoch %d: %v", requestedEpoch, err), http.StatusInternalServerError)
		return
	}
	var st state.BeaconState
	// if the requested epoch is new, use the head state and the next slot cache
	if requestedEpoch < currentEpoch {
		st, err = s.Stater.StateBySlot(ctx, epochStartSlot)
		if err != nil {
			httputil.HandleError(w, fmt.Sprintf("Could not get state for slot %d: %v ", epochStartSlot, err), http.StatusInternalServerError)
			return
		}
	} else {
		st, err = s.HeadFetcher.HeadState(ctx)
		if err != nil {
			httputil.HandleError(w, fmt.Sprintf("Could not get head state: %v ", err), http.StatusInternalServerError)
			return
		}
		// Advance state with empty transitions up to the requested epoch start slot.
		if st.Slot() < epochStartSlot {
			headRoot, err := s.HeadFetcher.HeadRoot(ctx)
			if err != nil {
				httputil.HandleError(w, fmt.Sprintf("Could not get head root: %v ", err), http.StatusInternalServerError)
				return
			}
			st, err = transition.ProcessSlotsUsingNextSlotCache(ctx, st, headRoot, epochStartSlot)
			if err != nil {
				httputil.HandleError(w, fmt.Sprintf("Could not process slots up to %d: %v ", epochStartSlot, err), http.StatusInternalServerError)
				return
			}
		}
	}

	var assignments map[primitives.ValidatorIndex][]primitives.Slot
	if nextEpochLookahead {
		assignments, err = helpers.ProposerAssignments(ctx, st, nextEpoch)
	} else {
		assignments, err = helpers.ProposerAssignments(ctx, st, requestedEpoch)
	}
	if err != nil {
		httputil.HandleError(w, "Could not compute committee assignments: "+err.Error(), http.StatusInternalServerError)
		return
	}

	duties := make([]*structs.ProposerDuty, 0)
	for index, proposalSlots := range assignments {
		val, err := st.ValidatorAtIndexReadOnly(index)
		if err != nil {
			httputil.HandleError(w, fmt.Sprintf("Could not get validator at index %d: %v", index, err), http.StatusInternalServerError)
			return
		}
		pubkey48 := val.PublicKey()
		pubkey := pubkey48[:]
		for _, slot := range proposalSlots {
			duties = append(duties, &structs.ProposerDuty{
				Pubkey:         hexutil.Encode(pubkey),
				ValidatorIndex: strconv.FormatUint(uint64(index), 10),
				Slot:           strconv.FormatUint(uint64(slot), 10),
			})
		}
	}

	var dependentRoot []byte
	if requestedEpoch == 0 {
		r, err := s.BeaconDB.GenesisBlockRoot(ctx)
		if err != nil {
			httputil.HandleError(w, "Could not get genesis block root: "+err.Error(), http.StatusInternalServerError)
			return
		}
		dependentRoot = r[:]
	} else {
		dependentRoot, err = proposalDependentRoot(st, requestedEpoch)
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
	if !sortProposerDuties(w, duties) {
		return
	}

	resp := &structs.GetProposerDutiesResponse{
		DependentRoot:       hexutil.Encode(dependentRoot),
		Data:                duties,
		ExecutionOptimistic: isOptimistic,
	}
	httputil.WriteJson(w, resp)
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
	lastValidEpoch := syncCommitteeDutiesLastValidEpoch(currentEpoch)
	if requestedEpoch > lastValidEpoch {
		httputil.HandleError(w, fmt.Sprintf("Epoch is too far in the future, maximum valid epoch is %d", lastValidEpoch), http.StatusBadRequest)
		return
	}

	startingEpoch := requestedEpoch
	if startingEpoch > currentEpoch {
		startingEpoch = currentEpoch
	}
	slot, err := slots.EpochStart(startingEpoch)
	if err != nil {
		httputil.HandleError(w, "Could not get sync committee slot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	st, err := s.Stater.State(ctx, []byte(strconv.FormatUint(uint64(slot), 10)))
	if err != nil {
		httputil.HandleError(w, "Could not get sync committee state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	currentSyncCommitteeFirstEpoch, err := slots.SyncCommitteePeriodStartEpoch(startingEpoch)
	if err != nil {
		httputil.HandleError(w, "Could not get sync committee period start epoch: "+err.Error(), http.StatusInternalServerError)
		return
	}
	nextSyncCommitteeFirstEpoch := currentSyncCommitteeFirstEpoch + params.BeaconConfig().EpochsPerSyncCommitteePeriod
	isCurrentCommitteeRequested := requestedEpoch < nextSyncCommitteeFirstEpoch
	var committee *ethpbalpha.SyncCommittee
	if isCurrentCommitteeRequested {
		committee, err = st.CurrentSyncCommittee()
		if err != nil {
			httputil.HandleError(w, "Could not get sync committee: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		committee, err = st.NextSyncCommittee()
		if err != nil {
			httputil.HandleError(w, "Could not get sync committee: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	committeePubkeys := make(map[[fieldparams.BLSPubkeyLength]byte][]string)
	for j, pubkey := range committee.Pubkeys {
		pubkey48 := bytesutil.ToBytes48(pubkey)
		committeePubkeys[pubkey48] = append(committeePubkeys[pubkey48], strconv.FormatUint(uint64(j), 10))
	}
	duties, vals, err := syncCommitteeDutiesAndVals(st, requestedValIndices, committeePubkeys)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	var registerSyncSubnet func(state.BeaconState, primitives.Epoch, []byte, validator2.Status) error
	if isCurrentCommitteeRequested {
		registerSyncSubnet = core.RegisterSyncSubnetCurrentPeriod
	} else {
		registerSyncSubnet = core.RegisterSyncSubnetNextPeriod
	}
	for _, v := range vals {
		pk := v.PublicKey()
		valStatus, err := rpchelpers.ValidatorStatus(v, requestedEpoch)
		if err != nil {
			httputil.HandleError(w, "Could not get validator status: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := registerSyncSubnet(st, requestedEpoch, pk[:], valStatus); err != nil {
			httputil.HandleError(w, fmt.Sprintf("Could not register sync subnet for pubkey %#x", pk), http.StatusInternalServerError)
			return
		}
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
			httputil.HandleError(w, "Could not get slot for requested epoch: "+err.Error(), http.StatusInternalServerError)
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

// attestationDependentRoot is get_block_root_at_slot(state, compute_start_slot_at_epoch(epoch - 1) - 1)
// or the genesis block root in the case of underflow.
func attestationDependentRoot(s state.BeaconState, epoch primitives.Epoch) ([]byte, error) {
	var dependentRootSlot primitives.Slot
	if epoch <= 1 {
		dependentRootSlot = 0
	} else {
		prevEpochStartSlot, err := slots.EpochStart(epoch.Sub(1))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not obtain epoch's start slot: %v", err)
		}
		dependentRootSlot = prevEpochStartSlot.Sub(1)
	}
	root, err := helpers.BlockRootAtSlot(s, dependentRootSlot)
	if err != nil {
		return nil, errors.Wrap(err, "could not get block root")
	}
	return root, nil
}

// proposalDependentRoot is get_block_root_at_slot(state, compute_start_slot_at_epoch(epoch) - 1)
// or the genesis block root in the case of underflow.
func proposalDependentRoot(s state.BeaconState, epoch primitives.Epoch) ([]byte, error) {
	var dependentRootSlot primitives.Slot
	if epoch == 0 {
		dependentRootSlot = 0
	} else {
		epochStartSlot, err := slots.EpochStart(epoch)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not obtain epoch's start slot: %v", err)
		}
		dependentRootSlot = epochStartSlot.Sub(1)
	}
	root, err := helpers.BlockRootAtSlot(s, dependentRootSlot)
	if err != nil {
		return nil, errors.Wrap(err, "could not get block root")
	}
	return root, nil
}

func syncCommitteeDutiesLastValidEpoch(currentEpoch primitives.Epoch) primitives.Epoch {
	currentSyncPeriodIndex := currentEpoch / params.BeaconConfig().EpochsPerSyncCommitteePeriod
	// Return the last epoch of the next sync committee.
	// To do this we go two periods ahead to find the first invalid epoch, and then subtract 1.
	return (currentSyncPeriodIndex+2)*params.BeaconConfig().EpochsPerSyncCommitteePeriod - 1
}

// syncCommitteeDutiesAndVals takes a list of requested validator indices and the actual sync committee pubkeys.
// It returns duties for the validator indices that are part of the sync committee.
// Additionally, it returns read-only validator objects for these validator indices.
func syncCommitteeDutiesAndVals(
	st state.BeaconState,
	requestedValIndices []primitives.ValidatorIndex,
	committeePubkeys map[[fieldparams.BLSPubkeyLength]byte][]string,
) ([]*structs.SyncCommitteeDuty, []state.ReadOnlyValidator, error) {
	duties := make([]*structs.SyncCommitteeDuty, 0)
	vals := make([]state.ReadOnlyValidator, 0)
	for _, index := range requestedValIndices {
		duty := &structs.SyncCommitteeDuty{
			ValidatorIndex: strconv.FormatUint(uint64(index), 10),
		}
		valPubkey := st.PubkeyAtIndex(index)
		var zeroPubkey [fieldparams.BLSPubkeyLength]byte
		if bytes.Equal(valPubkey[:], zeroPubkey[:]) {
			return nil, nil, errors.Errorf("Invalid validator index %d", index)
		}
		duty.Pubkey = hexutil.Encode(valPubkey[:])
		indices, ok := committeePubkeys[valPubkey]
		if ok {
			duty.ValidatorSyncCommitteeIndices = indices
			duties = append(duties, duty)
			v, err := st.ValidatorAtIndexReadOnly(index)
			if err != nil {
				return nil, nil, fmt.Errorf("could not get validator at index %d", index)
			}
			vals = append(vals, v)
		}
	}
	return duties, vals, nil
}

func sortProposerDuties(w http.ResponseWriter, duties []*structs.ProposerDuty) bool {
	ok := true
	sort.Slice(duties, func(i, j int) bool {
		si, err := strconv.ParseUint(duties[i].Slot, 10, 64)
		if err != nil {
			httputil.HandleError(w, "Could not parse slot: "+err.Error(), http.StatusInternalServerError)
			ok = false
			return false
		}
		sj, err := strconv.ParseUint(duties[j].Slot, 10, 64)
		if err != nil {
			httputil.HandleError(w, "Could not parse slot: "+err.Error(), http.StatusInternalServerError)
			ok = false
			return false
		}
		return si < sj
	})
	return ok
}
