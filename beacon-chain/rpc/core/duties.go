package core

import (
	"bytes"
	"context"
	"sort"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	coreTime "github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AttesterDutyResult is a transport-agnostic representation of attester duty.
type AttesterDutyResult struct {
	Pubkey                  [fieldparams.BLSPubkeyLength]byte
	ValidatorIndex          primitives.ValidatorIndex
	CommitteeIndex          primitives.CommitteeIndex
	CommitteeLength         uint64
	CommitteesAtSlot        uint64
	ValidatorCommitteeIndex uint64
	Slot                    primitives.Slot
}

// ProposerDutyResult is a transport-agnostic representation of proposer duty.
type ProposerDutyResult struct {
	Pubkey         [fieldparams.BLSPubkeyLength]byte
	ValidatorIndex primitives.ValidatorIndex
	Slot           primitives.Slot
}

// SyncCommitteeDutyResult is a transport-agnostic representation of sync committee duty.
type SyncCommitteeDutyResult struct {
	Pubkey                        [fieldparams.BLSPubkeyLength]byte
	ValidatorIndex                primitives.ValidatorIndex
	ValidatorSyncCommitteeIndices []uint64
}

// PTCDutyResult is a transport-agnostic representation of a PTC duty.
type PTCDutyResult struct {
	ValidatorIndex primitives.ValidatorIndex
	Slot           primitives.Slot
}

// AttesterDuties computes attester duties for the requested validators at the given epoch.
// The caller is responsible for providing a state that is adequate for the requested epoch.
func (s *Service) AttesterDuties(ctx context.Context, st state.BeaconState, epoch primitives.Epoch, indices []primitives.ValidatorIndex) ([]*AttesterDutyResult, *RpcError) {
	ctx, span := trace.StartSpan(ctx, "coreService.AttesterDuties")
	defer span.End()

	assignments, err := helpers.CommitteeAssignments(ctx, st, epoch, indices)
	if err != nil {
		return nil, &RpcError{Err: errors.Wrap(err, "could not compute committee assignments"), Reason: Internal}
	}
	activeValidatorCount, err := helpers.ActiveValidatorCount(ctx, st, epoch)
	if err != nil {
		return nil, &RpcError{Err: errors.Wrap(err, "could not get active validator count"), Reason: Internal}
	}
	committeesAtSlot := helpers.SlotCommitteeCount(activeValidatorCount)

	duties := make([]*AttesterDutyResult, 0, len(indices))
	for _, index := range indices {
		pubkey := st.PubkeyAtIndex(index)
		var zeroPubkey [fieldparams.BLSPubkeyLength]byte
		if bytes.Equal(pubkey[:], zeroPubkey[:]) {
			return nil, &RpcError{Err: errors.Errorf("Invalid validator index %d", index), Reason: BadRequest}
		}
		committee := assignments[index]
		if committee == nil {
			continue
		}
		duties = append(duties, &AttesterDutyResult{
			Pubkey:                  pubkey,
			ValidatorIndex:          index,
			CommitteeIndex:          committee.CommitteeIndex,
			CommitteeLength:         uint64(len(committee.Committee)),
			CommitteesAtSlot:        committeesAtSlot,
			ValidatorCommitteeIndex: findValidatorIndexInCommittee(committee.Committee, index),
			Slot:                    committee.AttesterSlot,
		})
	}
	return duties, nil
}

// ProposerDuties computes proposer duties for the given epoch.
// Results are sorted by slot.
func (s *Service) ProposerDuties(ctx context.Context, st state.BeaconState, epoch primitives.Epoch) ([]*ProposerDutyResult, *RpcError) {
	ctx, span := trace.StartSpan(ctx, "coreService.ProposerDuties")
	defer span.End()

	assignments, err := helpers.ProposerAssignments(ctx, st, epoch)
	if err != nil {
		return nil, &RpcError{Err: errors.Wrap(err, "could not compute proposer assignments"), Reason: Internal}
	}

	duties := make([]*ProposerDutyResult, 0, params.BeaconConfig().SlotsPerEpoch)
	for index, proposalSlots := range assignments {
		pubkey := st.PubkeyAtIndex(index)
		for _, slot := range proposalSlots {
			duties = append(duties, &ProposerDutyResult{
				Pubkey:         pubkey,
				ValidatorIndex: index,
				Slot:           slot,
			})
		}
	}

	sort.Slice(duties, func(i, j int) bool {
		return duties[i].Slot < duties[j].Slot
	})

	return duties, nil
}

// SyncCommitteeDuties computes sync committee duties for the requested validators.
// It also registers sync subnets for matched validators.
// The caller is responsible for providing a state that is adequate for the requested epoch.
func (s *Service) SyncCommitteeDuties(ctx context.Context, st state.BeaconState, requestedEpoch primitives.Epoch, currentEpoch primitives.Epoch, indices []primitives.ValidatorIndex) ([]*SyncCommitteeDutyResult, *RpcError) {
	_, span := trace.StartSpan(ctx, "coreService.SyncCommitteeDuties")
	defer span.End()

	// Determine which sync committee to use based on the requested epoch.
	startingEpoch := min(requestedEpoch, currentEpoch)
	currentSyncCommitteeFirstEpoch, err := slots.SyncCommitteePeriodStartEpoch(startingEpoch)
	if err != nil {
		return nil, &RpcError{Err: errors.Wrap(err, "could not get sync committee period start epoch"), Reason: Internal}
	}
	nextSyncCommitteeFirstEpoch := currentSyncCommitteeFirstEpoch + params.BeaconConfig().EpochsPerSyncCommitteePeriod
	isCurrentCommittee := requestedEpoch < nextSyncCommitteeFirstEpoch

	var committee [][]byte
	if isCurrentCommittee {
		sc, err := st.CurrentSyncCommittee()
		if err != nil {
			return nil, &RpcError{Err: errors.Wrap(err, "could not get sync committee"), Reason: Internal}
		}
		committee = sc.Pubkeys
	} else {
		sc, err := st.NextSyncCommittee()
		if err != nil {
			return nil, &RpcError{Err: errors.Wrap(err, "could not get sync committee"), Reason: Internal}
		}
		committee = sc.Pubkeys
	}

	// Build pubkey → positions map from committee pubkeys.
	committeePubkeys := make(map[[fieldparams.BLSPubkeyLength]byte][]uint64)
	for j, pk := range committee {
		var pk48 [fieldparams.BLSPubkeyLength]byte
		copy(pk48[:], pk)
		committeePubkeys[pk48] = append(committeePubkeys[pk48], uint64(j))
	}

	duties := make([]*SyncCommitteeDutyResult, 0)
	for _, index := range indices {
		pubkey := st.PubkeyAtIndex(index)
		var zeroPubkey [fieldparams.BLSPubkeyLength]byte
		if bytes.Equal(pubkey[:], zeroPubkey[:]) {
			return nil, &RpcError{Err: errors.Errorf("Invalid validator index %d", index), Reason: BadRequest}
		}
		positions, ok := committeePubkeys[pubkey]
		if !ok {
			continue
		}
		duties = append(duties, &SyncCommitteeDutyResult{
			Pubkey:                        pubkey,
			ValidatorIndex:                index,
			ValidatorSyncCommitteeIndices: positions,
		})

		// Register sync subnets for matched validators.
		if isCurrentCommittee {
			if err := RegisterSyncSubnetCurrentPeriod(st, requestedEpoch, pubkey[:], syncDutyStatus(st, index)); err != nil {
				return nil, &RpcError{Err: errors.Wrapf(err, "could not register sync subnet for validator %d", index), Reason: Internal}
			}
		} else {
			if err := RegisterSyncSubnetNextPeriod(st, requestedEpoch, pubkey[:], syncDutyStatus(st, index)); err != nil {
				return nil, &RpcError{Err: errors.Wrapf(err, "could not register sync subnet for validator %d", index), Reason: Internal}
			}
		}
	}

	return duties, nil
}

// PTCDuties computes payload timeliness committee duties for the requested validators
// at the given epoch. Pre-Gloas epochs return an empty result.
func (s *Service) PTCDuties(ctx context.Context, st state.BeaconState, epoch primitives.Epoch, indices []primitives.ValidatorIndex) ([]*PTCDutyResult, *RpcError) {
	_, span := trace.StartSpan(ctx, "coreService.PTCDuties")
	defer span.End()

	if len(indices) == 0 || epoch < params.BeaconConfig().GloasForkEpoch {
		return []*PTCDutyResult{}, nil
	}

	requested := make(map[primitives.ValidatorIndex]bool, len(indices))
	for _, idx := range indices {
		requested[idx] = true
	}

	startSlot, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, &RpcError{Err: err, Reason: Internal}
	}
	endSlot := startSlot + params.BeaconConfig().SlotsPerEpoch

	duties := make([]*PTCDutyResult, 0, len(indices))
	for slot := startSlot; slot < endSlot; slot++ {
		if ctx.Err() != nil {
			return nil, &RpcError{Err: ctx.Err(), Reason: Internal}
		}

		ptc, err := gloas.PayloadCommittee(ctx, st, slot)
		if err != nil {
			return nil, &RpcError{Err: err, Reason: Internal}
		}

		seen := make(map[primitives.ValidatorIndex]bool)
		for _, idx := range ptc {
			if !requested[idx] || seen[idx] {
				continue
			}
			seen[idx] = true
			duties = append(duties, &PTCDutyResult{
				ValidatorIndex: idx,
				Slot:           slot,
			})
		}
	}

	return duties, nil
}

// SyncCommitteeDutiesLastValidEpoch returns the last epoch for which sync committee duties can be computed.
func SyncCommitteeDutiesLastValidEpoch(currentEpoch primitives.Epoch) primitives.Epoch {
	currentSyncPeriodIndex := currentEpoch / params.BeaconConfig().EpochsPerSyncCommitteePeriod
	return (currentSyncPeriodIndex+2)*params.BeaconConfig().EpochsPerSyncCommitteePeriod - 1
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

// syncDutyStatus returns a validator.Status suitable for sync subnet registration.
// It returns Active for any active validator and Pending otherwise.
func syncDutyStatus(st state.BeaconState, idx primitives.ValidatorIndex) validator.Status {
	val, err := st.ValidatorAtIndexReadOnly(idx)
	if err != nil || val.IsNil() {
		return validator.Pending
	}
	currentEpoch := coreTime.CurrentEpoch(st)
	if val.ActivationEpoch() <= currentEpoch && currentEpoch < val.ExitEpoch() {
		return validator.Active
	}
	return validator.Pending
}

// AttestationDependentRoot returns the block root at (epoch-1 start - 1),
// which is the dependent root for attester duties at the given epoch.
// Callers must handle epoch <= 1 separately (e.g. using the genesis block root from the DB).
func AttestationDependentRoot(s state.BeaconState, epoch primitives.Epoch) ([]byte, error) {
	if epoch <= 1 {
		return nil, errors.New("epoch <= 1 requires genesis block root from DB")
	}
	prevEpochStartSlot, err := slots.EpochStart(epoch.Sub(1))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not obtain epoch's start slot: %v", err)
	}
	root, err := helpers.BlockRootAtSlot(s, prevEpochStartSlot.Sub(1))
	if err != nil {
		return nil, errors.Wrap(err, "could not get block root")
	}
	return root, nil
}

// ProposalDependentRoot returns the block root at (epoch start - 1),
// which is the dependent root for proposer duties at the given epoch.
// This is the pre-Fulu (v1) calculation used by the REST /eth/v1 endpoint.
// Callers must handle epoch 0 separately (e.g. using the genesis block root from the DB).
func ProposalDependentRoot(s state.BeaconState, epoch primitives.Epoch) ([]byte, error) {
	if epoch == 0 {
		return nil, errors.New("epoch 0 requires genesis block root from DB")
	}
	epochStartSlot, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not obtain epoch's start slot: %v", err)
	}
	root, err := helpers.BlockRootAtSlot(s, epochStartSlot.Sub(1))
	if err != nil {
		return nil, errors.Wrap(err, "could not get block root")
	}
	return root, nil
}

// ProposalDependentRootV2 returns the dependent root for proposer duties.
func ProposalDependentRootV2(s state.BeaconState, epoch primitives.Epoch) ([]byte, error) {
	if s.Version() >= version.Fulu {
		// Post-Fulu (EIP-7917) the proposer schedule is deterministic from the
		// previous epoch's state, so the dependent root is (prev_epoch_start - 1),
		// matching AttestationDependentRoot. Pre-Fulu it falls back to (epoch_start - 1).
		// See https://github.com/ethereum/beacon-APIs/pull/563.
		return AttestationDependentRoot(s, epoch)
	}
	return ProposalDependentRoot(s, epoch)
}
