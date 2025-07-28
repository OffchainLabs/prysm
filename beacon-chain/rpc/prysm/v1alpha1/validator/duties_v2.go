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

	// Load committee and proposer metadata
	meta, err := loadDutiesMetadata(ctx, s, req.Epoch)
	if err != nil {
		return nil, err
	}

	validatorAssignments := make([]*ethpb.DutiesV2Response_Duty, 0, len(req.PublicKeys))
	nextValidatorAssignments := make([]*ethpb.DutiesV2Response_Duty, 0, len(req.PublicKeys))

	// start loop for assignments for current and next epochs
	for _, pubKey := range req.PublicKeys {
		if ctx.Err() != nil {
			return nil, status.Errorf(codes.Aborted, "Could not continue fetching assignments: %v", ctx.Err())
		}

		idx, ok := s.ValidatorIndexByPubkey(bytesutil.ToBytes48(pubKey))
		if !ok {
			// Unknown validator: still append placeholder duty with UNKNOWN_STATUS
			validatorAssignments = append(validatorAssignments, &ethpb.DutiesV2Response_Duty{
				PublicKey: pubKey,
				Status:    ethpb.ValidatorStatus_UNKNOWN_STATUS,
			})
			nextValidatorAssignments = append(nextValidatorAssignments, &ethpb.DutiesV2Response_Duty{
				PublicKey: pubKey,
				Status:    ethpb.ValidatorStatus_UNKNOWN_STATUS,
			})
			continue
		}

		meta.current.liteAssignment = helpers.AssignmentForValidator(meta.current.committeesBySlot, meta.current.startSlot, idx)
		meta.next.liteAssignment = helpers.AssignmentForValidator(meta.next.committeesBySlot, meta.next.startSlot, idx)

		assignment, nextAssignment, err := vs.buildValidatorDuty(pubKey, idx, s, req.Epoch, meta)
		if err != nil {
			return nil, err
		}
		validatorAssignments = append(validatorAssignments, assignment)
		nextValidatorAssignments = append(nextValidatorAssignments, nextAssignment)
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
	committeesAtSlot uint64
	proposalSlots    map[primitives.ValidatorIndex][]primitives.Slot
	startSlot        primitives.Slot
	committeesBySlot [][][]primitives.ValidatorIndex
	liteAssignment   *helpers.LiteAssignment
}

func loadDutiesMetadata(ctx context.Context, s state.BeaconState, reqEpoch primitives.Epoch) (*dutiesMetadata, error) {
	meta := &dutiesMetadata{}
	var err error
	meta.current, err = loadMetadata(ctx, s, reqEpoch)
	if err != nil {
		return nil, err
	}
	// note: we only set the proposer slots for the current assignment and not the next epoch assignment
	meta.current.proposalSlots, err = helpers.ProposerAssignments(ctx, s, reqEpoch)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not compute proposer slots: %v", err)
	}

	meta.next, err = loadMetadata(ctx, s, reqEpoch+1)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

func loadMetadata(ctx context.Context, s state.BeaconState, reqEpoch primitives.Epoch) (*metadata, error) {
	meta := &metadata{}

	if err := helpers.VerifyAssignmentEpoch(reqEpoch, s); err != nil {
		return nil, err
	}

	activeValidatorCount, err := helpers.ActiveValidatorCount(ctx, s, reqEpoch)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get active validator count: %v", err)
	}
	meta.committeesAtSlot = helpers.SlotCommitteeCount(activeValidatorCount)

	meta.startSlot, err = slots.EpochStart(reqEpoch)
	if err != nil {
		return nil, err
	}

	meta.committeesBySlot, err = helpers.PrecomputeCommittees(ctx, s, meta.startSlot)
	if err != nil {
		return nil, err
	}

	return meta, nil
}

// buildValidatorDuty builds both current‑epoch and next‑epoch V2 duty objects
// for a single validator index.
func (vs *Server) buildValidatorDuty(
	pubKey []byte,
	idx primitives.ValidatorIndex,
	s state.BeaconState,
	reqEpoch primitives.Epoch,
	meta *dutiesMetadata,
) (*ethpb.DutiesV2Response_Duty, *ethpb.DutiesV2Response_Duty, error) {
	assignment := &ethpb.DutiesV2Response_Duty{PublicKey: pubKey}
	nextAssignment := &ethpb.DutiesV2Response_Duty{PublicKey: pubKey}

	statusEnum := assignmentStatus(s, idx)
	assignment.ValidatorIndex = idx
	assignment.Status = statusEnum
	assignment.CommitteesAtSlot = meta.current.committeesAtSlot
	assignment.ProposerSlots = meta.current.proposalSlots[idx]
	populateCommitteeFields(assignment, meta.current.liteAssignment)

	nextAssignment.ValidatorIndex = idx
	nextAssignment.Status = statusEnum
	nextAssignment.CommitteesAtSlot = meta.next.committeesAtSlot
	populateCommitteeFields(nextAssignment, meta.next.liteAssignment)

	// Sync committee flags
	if coreTime.HigherEqualThanAltairVersionAndEpoch(s, reqEpoch) {
		inSync, err := helpers.IsCurrentPeriodSyncCommittee(s, idx)
		if err != nil {
			return nil, nil, status.Errorf(codes.Internal, "Could not determine current epoch sync committee: %v", err)
		}
		assignment.IsSyncCommittee = inSync
		nextAssignment.IsSyncCommittee = inSync
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
			nextAssignment.IsSyncCommittee = nextInSync
			if nextInSync {
				go func() {
					if err := core.RegisterSyncSubnetNextPeriodProto(s, reqEpoch, pubKey, statusEnum); err != nil {
						log.WithError(err).Warn("Could not register sync subnet next period")
					}
				}()
			}
		}
	}

	return assignment, nextAssignment, nil
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
