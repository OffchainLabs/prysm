package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	coreTime "github.com/OffchainLabs/prysm/v6/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
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

	meta, err := loadDutiesMetadata(ctx, s, req.Epoch, requestIndices)
	if err != nil {
		return nil, err
	}

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

		currentAssignment := vs.getValidatorAssignment(meta.current, info.index)
		nextAssignment := vs.getValidatorAssignment(meta.next, info.index)

		assignment, nextDuty, err := vs.buildValidatorDuty(pubKey, info.index, s, req.Epoch, meta, currentAssignment, nextAssignment)
		if err != nil {
			return nil, err
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

// dutiesMetadata bundles together related data needed for duty
// construction.
type dutiesMetadata struct {
	current *metadata
	next    *metadata
}

type metadata struct {
	committeesAtSlot     uint64
	proposalSlots        map[primitives.ValidatorIndex][]primitives.Slot
	committeeAssignments map[primitives.ValidatorIndex]*helpers.CommitteeAssignment
}

func loadDutiesMetadata(ctx context.Context, s state.BeaconState, reqEpoch primitives.Epoch, requestIndices []primitives.ValidatorIndex) (*dutiesMetadata, error) {
	meta := &dutiesMetadata{}
	var err error
	meta.current, err = loadMetadata(ctx, s, reqEpoch, requestIndices)
	if err != nil {
		return nil, err
	}
	// note: we only set the proposer slots for the current assignment and not the next epoch assignment
	meta.current.proposalSlots, err = helpers.ProposerAssignments(ctx, s, reqEpoch)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not compute proposer slots: %v", err)
	}

	meta.next, err = loadMetadata(ctx, s, reqEpoch+1, requestIndices)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

func loadMetadata(ctx context.Context, s state.BeaconState, reqEpoch primitives.Epoch, requestIndices []primitives.ValidatorIndex) (*metadata, error) {
	meta := &metadata{}

	if err := helpers.VerifyAssignmentEpoch(reqEpoch, s); err != nil {
		return nil, err
	}

	activeValidatorCount, err := helpers.ActiveValidatorCount(ctx, s, reqEpoch)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get active validator count: %v", err)
	}
	meta.committeesAtSlot = helpers.SlotCommitteeCount(activeValidatorCount)

	// Use CommitteeAssignments which only computes committees for requested validators
	meta.committeeAssignments, err = helpers.CommitteeAssignments(ctx, s, reqEpoch, requestIndices)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not compute committee assignments: %v", err)
	}

	return meta, nil
}

// findValidatorIndexInCommittee finds the position of a validator in a committee.
func findValidatorIndexInCommittee(committee []primitives.ValidatorIndex, validatorIndex primitives.ValidatorIndex) uint64 {
	for i, vIdx := range committee {
		if vIdx == validatorIndex {
			return uint64(i)
		}
	}
	return 0
}

// getValidatorAssignment retrieves the assignment for a validator from CommitteeAssignments.
func (vs *Server) getValidatorAssignment(meta *metadata, validatorIndex primitives.ValidatorIndex) *helpers.LiteAssignment {
	if assignment, exists := meta.committeeAssignments[validatorIndex]; exists {
		return &helpers.LiteAssignment{
			AttesterSlot:            assignment.AttesterSlot,
			CommitteeIndex:          assignment.CommitteeIndex,
			CommitteeLength:         uint64(len(assignment.Committee)),
			ValidatorCommitteeIndex: findValidatorIndexInCommittee(assignment.Committee, validatorIndex),
		}
	}
	return &helpers.LiteAssignment{}
}

// buildValidatorDuty builds both current‑epoch and next‑epoch V2 duty objects
// for a single validator index.
func (vs *Server) buildValidatorDuty(
	pubKey []byte,
	idx primitives.ValidatorIndex,
	s state.BeaconState,
	reqEpoch primitives.Epoch,
	meta *dutiesMetadata,
	currentAssignment *helpers.LiteAssignment,
	nextAssignment *helpers.LiteAssignment,
) (*ethpb.DutiesV2Response_Duty, *ethpb.DutiesV2Response_Duty, error) {
	assignment := &ethpb.DutiesV2Response_Duty{PublicKey: pubKey}
	nextDuty := &ethpb.DutiesV2Response_Duty{PublicKey: pubKey}

	statusEnum := assignmentStatus(s, idx)
	assignment.ValidatorIndex = idx
	assignment.Status = statusEnum
	assignment.CommitteesAtSlot = meta.current.committeesAtSlot
	assignment.ProposerSlots = meta.current.proposalSlots[idx]
	populateCommitteeFields(assignment, currentAssignment)

	nextDuty.ValidatorIndex = idx
	nextDuty.Status = statusEnum
	nextDuty.CommitteesAtSlot = meta.next.committeesAtSlot
	populateCommitteeFields(nextDuty, nextAssignment)

	// Sync committee flags
	if coreTime.HigherEqualThanAltairVersionAndEpoch(s, reqEpoch) {
		inSync, err := helpers.IsCurrentPeriodSyncCommittee(s, idx)
		if err != nil {
			return nil, nil, status.Errorf(codes.Internal, "Could not determine current epoch sync committee: %v", err)
		}
		assignment.IsSyncCommittee = inSync
		nextDuty.IsSyncCommittee = inSync
		if inSync {
			if err := core.RegisterSyncSubnetCurrentPeriodProto(s, reqEpoch, pubKey, statusEnum); err != nil {
				return nil, nil, status.Errorf(codes.Internal, "Could not register sync subnet current period: %v", err)
			}
		}

		// Next epoch sync committee duty is assigned with next period sync committee only during
		// sync period epoch boundary (ie. EPOCHS_PER_SYNC_COMMITTEE_PERIOD - 1). Else wise
		// next epoch sync committee duty is the same as current epoch.
		nextEpoch := reqEpoch + 1
		currentEpoch := coreTime.CurrentEpoch(s)
		n := slots.SyncCommitteePeriod(nextEpoch)
		c := slots.SyncCommitteePeriod(currentEpoch)
		if n > c {
			nextInSync, err := helpers.IsNextPeriodSyncCommittee(s, idx)
			if err != nil {
				return nil, nil, status.Errorf(codes.Internal, "Could not determine next epoch sync committee: %v", err)
			}
			nextDuty.IsSyncCommittee = nextInSync
			if nextInSync {
				if err := core.RegisterSyncSubnetNextPeriodProto(s, reqEpoch, pubKey, statusEnum); err != nil {
					log.WithError(err).Warn("Could not register sync subnet next period")
				}
			}
		}
	}

	return assignment, nextDuty, nil
}

func populateCommitteeFields(duty *ethpb.DutiesV2Response_Duty, la *helpers.LiteAssignment) {
	if duty == nil || la == nil {
		// should never be the case as previous functions should set
		return
	}
	duty.CommitteeLength = la.CommitteeLength
	duty.CommitteeIndex = la.CommitteeIndex
	duty.ValidatorCommitteeIndex = la.ValidatorCommitteeIndex
	duty.AttesterSlot = la.AttesterSlot
}
