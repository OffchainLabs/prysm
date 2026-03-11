package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	coreTime "github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetDutiesV2 returns the duties assigned to a list of validators specified
// in the request object.
//
// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
func (vs *Server) GetDutiesV2(ctx context.Context, req *ethpb.DutiesRequest) (*ethpb.DutiesV2Response, error) {
	if vs.SyncChecker.Syncing() {
		return nil, status.Error(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}
	return vs.dutiesv2(ctx, req)
}

// Compute the validator duties from the head state's corresponding epoch
// for validators public key / indices requested.
func (vs *Server) dutiesv2(ctx context.Context, req *ethpb.DutiesRequest) (*ethpb.DutiesV2Response, error) {
	currentEpoch := slots.ToEpoch(vs.TimeFetcher.CurrentSlot())
	if req.Epoch > currentEpoch+1 {
		return nil, status.Errorf(codes.Unavailable, "Request epoch %d can not be greater than next epoch %d", req.Epoch, currentEpoch+1)
	}

	// Load head state
	s, err := vs.HeadFetcher.HeadState(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get head state: %v", err)
	}

	// Advance to start of requested epoch if necessary
	s, err = vs.stateForEpoch(ctx, s, req.Epoch)
	if err != nil {
		return nil, err
	}

	// Build duties for each validator
	ctx, span := trace.StartSpan(ctx, "dutiesv2.BuildResponse")
	span.SetAttributes(trace.Int64Attribute("num_pubkeys", int64(len(req.PublicKeys))))
	defer span.End()

	// Collect validator indices from public keys and cache the lookups
	type validatorInfo struct {
		index primitives.ValidatorIndex
		found bool
	}
	validatorLookup := make(map[string]validatorInfo, len(req.PublicKeys))
	requestIndices := make([]primitives.ValidatorIndex, 0, len(req.PublicKeys))

	for _, pubKey := range req.PublicKeys {
		key := string(pubKey)
		if _, exists := validatorLookup[key]; !exists {
			idx, ok := s.ValidatorIndexByPubkey(bytesutil.ToBytes48(pubKey))
			validatorLookup[key] = validatorInfo{index: idx, found: ok}
			if ok {
				requestIndices = append(requestIndices, idx)
			}
		}
	}

	// Use core service for attester and proposer duties
	currentAttesterDuties, rpcErr := vs.CoreService.AttesterDuties(ctx, s, req.Epoch, requestIndices)
	if rpcErr != nil {
		return nil, status.Errorf(core.ErrorReasonToGRPC(rpcErr.Reason), "%v", rpcErr.Err)
	}
	nextAttesterDuties, rpcErr := vs.CoreService.AttesterDuties(ctx, s, req.Epoch+1, requestIndices)
	if rpcErr != nil {
		return nil, status.Errorf(core.ErrorReasonToGRPC(rpcErr.Reason), "%v", rpcErr.Err)
	}
	proposerDuties, rpcErr := vs.CoreService.ProposerDuties(ctx, s, req.Epoch)
	if rpcErr != nil {
		return nil, status.Errorf(core.ErrorReasonToGRPC(rpcErr.Reason), "%v", rpcErr.Err)
	}
	currentPTCDuties, rpcErr := vs.CoreService.PTCDuties(ctx, s, req.Epoch, requestIndices)
	if rpcErr != nil {
		return nil, status.Errorf(core.ErrorReasonToGRPC(rpcErr.Reason), "%v", rpcErr.Err)
	}
	nextPTCDuties, rpcErr := vs.CoreService.PTCDuties(ctx, s, req.Epoch+1, requestIndices)
	if rpcErr != nil {
		return nil, status.Errorf(core.ErrorReasonToGRPC(rpcErr.Reason), "%v", rpcErr.Err)
	}

	// Build index maps for O(1) lookup
	currentAttesterMap := buildAttesterMap(currentAttesterDuties)
	nextAttesterMap := buildAttesterMap(nextAttesterDuties)
	proposerMap := buildProposerMap(proposerDuties)
	currentPTCAssignments := buildPTCMap(currentPTCDuties)
	nextPTCAssignments := buildPTCMap(nextPTCDuties)
	validatorAssignments := make([]*ethpb.DutiesV2Response_Duty, 0, len(req.PublicKeys))
	nextValidatorAssignments := make([]*ethpb.DutiesV2Response_Duty, 0, len(req.PublicKeys))

	// Build duties using cached validator index lookups
	for _, pubKey := range req.PublicKeys {
		if ctx.Err() != nil {
			return nil, status.Errorf(codes.Aborted, "Could not continue fetching assignments: %v", ctx.Err())
		}

		info := validatorLookup[string(pubKey)]
		if !info.found {
			unknownDuty := &ethpb.DutiesV2Response_Duty{
				PublicKey: pubKey,
				Status:    ethpb.ValidatorStatus_UNKNOWN_STATUS,
			}
			validatorAssignments = append(validatorAssignments, unknownDuty)
			nextValidatorAssignments = append(nextValidatorAssignments, unknownDuty)
			continue
		}

		statusEnum := assignmentStatus(s, info.index)

		// Current epoch assignment
		assignment := &ethpb.DutiesV2Response_Duty{
			PublicKey:      pubKey,
			ValidatorIndex: info.index,
			Status:         statusEnum,
			ProposerSlots:  proposerMap[info.index],
		}
		if ad, ok := currentAttesterMap[info.index]; ok {
			assignment.AttesterSlot = ad.Slot
			assignment.CommitteeIndex = ad.CommitteeIndex
			assignment.CommitteeLength = ad.CommitteeLength
			assignment.CommitteesAtSlot = ad.CommitteesAtSlot
			assignment.ValidatorCommitteeIndex = ad.ValidatorCommitteeIndex
		}
		if ptcSlots, ok := currentPTCAssignments[info.index]; ok {
			assignment.PtcSlots = ptcSlots
		}

		// Next epoch assignment
		nextDuty := &ethpb.DutiesV2Response_Duty{
			PublicKey:      pubKey,
			ValidatorIndex: info.index,
			Status:         statusEnum,
		}
		if ad, ok := nextAttesterMap[info.index]; ok {
			nextDuty.AttesterSlot = ad.Slot
			nextDuty.CommitteeIndex = ad.CommitteeIndex
			nextDuty.CommitteeLength = ad.CommitteeLength
			nextDuty.CommitteesAtSlot = ad.CommitteesAtSlot
			nextDuty.ValidatorCommitteeIndex = ad.ValidatorCommitteeIndex
		}
		if ptcSlots, ok := nextPTCAssignments[info.index]; ok {
			nextDuty.PtcSlots = ptcSlots
		}

		// Sync committee flags
		if coreTime.HigherEqualThanAltairVersionAndEpoch(s, req.Epoch) {
			inSync, err := helpers.IsCurrentPeriodSyncCommittee(s, info.index)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "Could not determine current epoch sync committee: %v", err)
			}
			assignment.IsSyncCommittee = inSync
			nextDuty.IsSyncCommittee = inSync
			if inSync {
				if err := core.RegisterSyncSubnetCurrentPeriodProto(s, req.Epoch, pubKey, statusEnum); err != nil {
					return nil, status.Errorf(codes.Internal, "Could not register sync subnet current period: %v", err)
				}
			}

			// Next epoch sync committee duty is assigned with next period sync committee only during
			// sync period epoch boundary (ie. EPOCHS_PER_SYNC_COMMITTEE_PERIOD - 1). Else wise
			// next epoch sync committee duty is the same as current epoch.
			nextEpoch := req.Epoch.Add(1)
			stateEpoch := coreTime.CurrentEpoch(s)
			n := slots.SyncCommitteePeriod(nextEpoch)
			c := slots.SyncCommitteePeriod(stateEpoch)
			if n > c {
				nextInSync, err := helpers.IsNextPeriodSyncCommittee(s, info.index)
				if err != nil {
					return nil, status.Errorf(codes.Internal, "Could not determine next epoch sync committee: %v", err)
				}
				nextDuty.IsSyncCommittee = nextInSync
				if nextInSync {
					if err := core.RegisterSyncSubnetNextPeriodProto(s, req.Epoch, pubKey, statusEnum); err != nil {
						log.WithError(err).Warn("Could not register sync subnet next period")
					}
				}
			}
		}

		validatorAssignments = append(validatorAssignments, assignment)
		nextValidatorAssignments = append(nextValidatorAssignments, nextDuty)
	}

	// Dependent roots for fork choice
	currDependentRoot, err := vs.ForkchoiceFetcher.DependentRoot(currentEpoch)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get dependent root: %v", err)
	}
	prevDependentRoot := currDependentRoot
	if currDependentRoot != [32]byte{} && currentEpoch > 0 {
		prevDependentRoot, err = vs.ForkchoiceFetcher.DependentRoot(currentEpoch - 1)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not get previous dependent root: %v", err)
		}
	}

	return &ethpb.DutiesV2Response{
		PreviousDutyDependentRoot: prevDependentRoot[:],
		CurrentDutyDependentRoot:  currDependentRoot[:],
		CurrentEpochDuties:        validatorAssignments,
		NextEpochDuties:           nextValidatorAssignments,
	}, nil
}

// stateForEpoch returns a state advanced (with empty slot transitions) to the
// start slot of the requested epoch.
func (vs *Server) stateForEpoch(ctx context.Context, s state.BeaconState, reqEpoch primitives.Epoch) (state.BeaconState, error) {
	epochStartSlot, err := slots.EpochStart(reqEpoch)
	if err != nil {
		return nil, err
	}
	if s.Slot() >= epochStartSlot {
		return s, nil
	}
	headRoot, err := vs.HeadFetcher.HeadRoot(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not retrieve head root: %v", err)
	}
	s, err = transition.ProcessSlotsUsingNextSlotCache(ctx, s, headRoot, epochStartSlot)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not process slots up to %d: %v", epochStartSlot, err)
	}
	return s, nil
}

// buildAttesterMap creates a map from validator index to attester duty for O(1) lookup.
func buildAttesterMap(duties []*core.AttesterDutyResult) map[primitives.ValidatorIndex]*core.AttesterDutyResult {
	m := make(map[primitives.ValidatorIndex]*core.AttesterDutyResult, len(duties))
	for _, d := range duties {
		m[d.ValidatorIndex] = d
	}
	return m
}

// buildProposerMap creates a map from validator index to proposal slots for O(1) lookup.
func buildProposerMap(duties []*core.ProposerDutyResult) map[primitives.ValidatorIndex][]primitives.Slot {
	m := make(map[primitives.ValidatorIndex][]primitives.Slot)
	for _, d := range duties {
		m[d.ValidatorIndex] = append(m[d.ValidatorIndex], d.Slot)
	}
	return m
}

// buildPTCMap creates a map from validator index to PTC slots for O(1) lookup.
func buildPTCMap(duties []*core.PTCDutyResult) map[primitives.ValidatorIndex][]primitives.Slot {
	m := make(map[primitives.ValidatorIndex][]primitives.Slot)
	for _, d := range duties {
		m[d.ValidatorIndex] = append(m[d.ValidatorIndex], d.Slot)
	}
	return m
}
