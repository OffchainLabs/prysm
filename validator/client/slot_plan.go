package client

import (
	"context"
	"slices"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// dutyAssignment is a plan-owned duty copy; never re-read the store after planning.
type dutyAssignment struct {
	publicKey               pubkey
	validatorIndex          primitives.ValidatorIndex
	committeeIndex          primitives.CommitteeIndex
	committeeLength         uint64
	validatorCommitteeIndex uint64
}

type attestationWork struct {
	duty      dutyAssignment
	aggregate bool
}

type syncCommitteeWork struct {
	duty      dutyAssignment
	aggregate bool
}

func newDutyAssignment(pk pubkey, duty *ethpb.ValidatorDuty) dutyAssignment {
	return dutyAssignment{
		publicKey:               pk,
		validatorIndex:          duty.ValidatorIndex,
		committeeIndex:          duty.CommitteeIndex,
		committeeLength:         duty.CommitteeLength,
		validatorCommitteeIndex: duty.ValidatorCommitteeIndex,
	}
}

// slotPlan is all decided work for one slot, from a single duty snapshot.
type slotPlan struct {
	slot                primitives.Slot
	dutyRevision        uint64
	proposals           []pubkey
	attestations        []attestationWork
	syncCommittee       []syncCommitteeWork
	payloadAttestations []dutyAssignment
}

// planSlot builds all work for slot; the plan stays valid across duty refreshes.
func (v *validator) planSlot(ctx context.Context, slot primitives.Slot) (slotPlan, error) {
	ctx, span := trace.StartSpan(ctx, "validator.planSlot")
	defer span.End()

	snap := v.duties.snapshot()
	if !snap.isInitialized() {
		return slotPlan{}, errors.New("validator duties are not initialized")
	}

	plan := slotPlan{slot: slot, dutyRevision: snap.revision}
	var syncCommitteePubkeys []pubkey

	for pk, duty := range snap.currentDuties() {
		if duty == nil {
			continue
		}
		// The store may predate a reload; quarantined keys get no work at all.
		if v.isDoppelGangerPending(pk) {
			continue
		}

		assignment := newDutyAssignment(pk, duty)

		for _, proposerSlot := range duty.ProposerSlots {
			if proposerSlot != 0 && proposerSlot == slot {
				plan.proposals = append(plan.proposals, pk)
				break
			}
		}

		if duty.AttesterSlot == slot {
			work := attestationWork{duty: assignment}
			aggregator, err := v.isAggregator(ctx, duty.CommitteeLength, slot, pk)
			if err != nil {
				log.WithError(err).Errorf("Could not check if validator %#x is an aggregator", bytesutil.Trunc(duty.PublicKey))
			} else {
				work.aggregate = aggregator
			}
			plan.attestations = append(plan.attestations, work)
		}

		// Sync duty at slot signs for slot-1; at epoch end, use the next committee.
		inSyncCommittee := duty.IsSyncCommittee
		if slots.IsEpochEnd(slot) {
			inSyncCommittee = snap.isNextSyncCommittee(duty.ValidatorIndex)
		}
		if inSyncCommittee {
			plan.syncCommittee = append(plan.syncCommittee, syncCommitteeWork{duty: assignment})
			syncCommitteePubkeys = append(syncCommitteePubkeys, pk)
		}

		if slices.Contains(snap.ptcSlots(duty.ValidatorIndex), slot) {
			plan.payloadAttestations = append(plan.payloadAttestations, assignment)
		}
	}

	aggPubkeys, err := v.aggSelector.SyncCommitteeAggregators(ctx, slot, syncCommitteePubkeys)
	if err != nil {
		log.WithError(err).Error("Could not check if any validator is a sync committee aggregator")
		return plan, nil
	}
	aggregators := make(map[pubkey]bool, len(aggPubkeys))
	for _, pk := range aggPubkeys {
		aggregators[pk] = true
	}
	for i := range plan.syncCommittee {
		plan.syncCommittee[i].aggregate = aggregators[plan.syncCommittee[i].duty.publicKey]
	}

	return plan, nil
}
