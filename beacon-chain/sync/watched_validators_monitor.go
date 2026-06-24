package sync

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

// This file watches a configured set of validator indices (which need not be local to this node) and
// reports whether their expected attestations are later seen included in an aggregate received on the
// beacon_aggregate_and_proof gossip subnet.
//
// Unlike the local self-attestation monitor, it does not depend on observing the validators' unaggregated
// attestations: a validator's committee assignment for an epoch is deterministic public data, so we seed
// the slot/committee it is expected to attest in directly from the head state. This lets a second node in
// a different region independently confirm whether those votes were aggregated network-wide, and it only
// requires the global aggregate topic (no --subscribe-all-subnets needed).

type (
	// watchedDutyRecord records a single watched validator's expected attestation for a slot.
	watchedDutyRecord struct {
		committee primitives.CommitteeIndex // committee the validator is expected to attest in.
		seen      bool                      // true once the validator's vote has been observed in a gossiped aggregate.
	}

	// watchedSlotEntry groups all watched validators expected to attest in the same slot.
	watchedSlotEntry struct {
		validators map[primitives.ValidatorIndex]*watchedDutyRecord
	}
)

// monitorWatchedValidatorDuties seeds the expected attestation duties of the watched validators each
// epoch and periodically reports whether those attestations were seen in a gossiped aggregate. It runs
// until the service context is closed and is a no-op when no validators are watched.
func (s *Service) monitorWatchedValidatorDuties() {
	watched := features.Get().WatchedAttestationValidators
	if len(watched) == 0 {
		return
	}

	indices := make([]primitives.ValidatorIndex, 0, len(watched))
	for idx := range watched {
		indices = append(indices, idx)
	}
	slices.Sort(indices)

	log.WithField("watchedValidators", indices).Info("Watching validators for attestation-aggregate inclusion")

	ticker := time.NewTicker(time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			slot := s.cfg.clock.CurrentSlot()
			s.seedWatchedDuties(s.ctx, slot, indices)
			s.reportWatchedDuties(slot)
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting watched validators monitor")
			return
		}
	}
}

// seedWatchedDuties computes the committee assignments of the watched validators for the current and
// next epoch and records the slot/committee each is expected to attest in. Already-seeded epochs are
// skipped, so this is cheap to call every slot.
func (s *Service) seedWatchedDuties(ctx context.Context, slot primitives.Slot, indices []primitives.ValidatorIndex) {
	currentEpoch := slots.ToEpoch(slot)
	// Seed the current and next epoch: assignments for both are deterministic from the current head
	// state, and seeding ahead means we are ready for aggregates that arrive at an epoch boundary.
	for _, epoch := range []primitives.Epoch{currentEpoch, currentEpoch + 1} {
		s.watchedDutiesLock.Lock()
		seeded := s.watchedSeededEpochs[epoch]
		s.watchedDutiesLock.Unlock()
		if seeded {
			continue
		}

		st, err := s.cfg.chain.HeadState(ctx)
		if err != nil {
			log.WithError(err).Debug("Could not get head state to seed watched validator duties")
			return
		}
		if err := helpers.VerifyAssignmentEpoch(epoch, st); err != nil {
			// Epoch not yet computable from the current head state; retry on a later tick.
			continue
		}

		assignments, err := helpers.CommitteeAssignments(ctx, st, epoch, indices)
		if err != nil {
			log.WithError(err).WithField("epoch", epoch).Debug("Could not compute watched validator duties")
			continue
		}

		s.watchedDutiesLock.Lock()
		for idx, assignment := range assignments {
			e, ok := s.watchedDuties[assignment.AttesterSlot]
			if !ok {
				e = &watchedSlotEntry{validators: make(map[primitives.ValidatorIndex]*watchedDutyRecord)}
				s.watchedDuties[assignment.AttesterSlot] = e
			}
			if _, exists := e.validators[idx]; !exists {
				e.validators[idx] = &watchedDutyRecord{committee: assignment.CommitteeIndex}
			}
		}
		s.watchedSeededEpochs[epoch] = true
		s.watchedDutiesLock.Unlock()
	}
}

// matchWatchedDuties marks any watched validator whose expected attestation the given aggregate includes.
func (s *Service) matchWatchedDuties(ctx context.Context, aggregate ethpb.Att) {
	if len(features.Get().WatchedAttestationValidators) == 0 {
		return
	}
	if aggregate == nil || aggregate.GetData() == nil {
		return
	}

	ctx, span := trace.StartSpan(ctx, "sync.matchWatchedDuties")
	defer span.End()

	slot := aggregate.GetData().Slot

	s.watchedDutiesLock.Lock()
	defer s.watchedDutiesLock.Unlock()

	e, ok := s.watchedDuties[slot]
	if !ok || e.allSeen() {
		return
	}

	indices, err := s.attestingIndices(ctx, aggregate)
	if err != nil {
		if !errors.Is(err, errSelfAttStateNotCached) {
			log.WithError(err).Debug("Could not get attesting indices for received aggregate")
		}

		return
	}

	included := make(map[uint64]bool, len(indices))
	for _, idx := range indices {
		included[idx] = true
	}

	for idx, rec := range e.validators {
		if !rec.seen && included[uint64(idx)] {
			rec.seen = true
		}
	}
}

// reportWatchedDuties logs the seen/never-seen status of watched validators whose expected attestation
// slot has aged past the retention window, then drops those entries.
func (s *Service) reportWatchedDuties(slot primitives.Slot) {
	retention := primitives.Slot(selfAttRetentionEpochs) * params.BeaconConfig().SlotsPerEpoch
	if slot < retention {
		return
	}
	threshold := slot - retention

	s.watchedDutiesLock.Lock()
	defer s.watchedDutiesLock.Unlock()

	for dutySlot, e := range s.watchedDuties {
		if dutySlot >= threshold {
			continue
		}

		s.logWatchedDuties(dutySlot, e)
		delete(s.watchedDuties, dutySlot)
	}
}

// logWatchedDuties reports the seen/never-seen status of the watched validators expected to attest at the
// slot. The messages say "watched" rather than "submitted": this node does not submit these attestations,
// it only knows (from the validators' public committee assignments) that they were expected to attest.
// "never seen" therefore means the validator was expected to attest at the slot but appeared in no
// aggregate this node received.
func (s *Service) logWatchedDuties(slot primitives.Slot, e *watchedSlotEntry) {
	seen := make([]primitives.ValidatorIndex, 0, len(e.validators))
	notSeen := make([]primitives.ValidatorIndex, 0, len(e.validators))
	for idx, rec := range e.validators {
		if rec.seen {
			seen = append(seen, idx)
			continue
		}

		notSeen = append(notSeen, idx)
	}
	slices.Sort(seen)
	slices.Sort(notSeen)

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

	if len(notSeen) > 0 {
		committees := make([]primitives.CommitteeIndex, len(notSeen))
		for i, idx := range notSeen {
			committees[i] = e.validators[idx].committee
		}

		log.WithFields(logrus.Fields{
			"slot":              slot,
			"epoch":             slot / slotsPerEpoch,
			"slotInEpoch":       slot % slotsPerEpoch,
			"seenCount":         len(seen),
			"notSeenCount":      len(notSeen),
			"notSeenValidators": notSeen,
			"notSeenCommittees": committees,
		}).Warning("Watched attestations never seen in a gossiped aggregate")
	}

	if len(seen) > 0 {
		committees := make([]primitives.CommitteeIndex, len(seen))
		for i, idx := range seen {
			committees[i] = e.validators[idx].committee
		}

		log.WithFields(logrus.Fields{
			"slot":             slot,
			"epoch":            slot / slotsPerEpoch,
			"slotInEpoch":      slot % slotsPerEpoch,
			"committees":       committees,
			"validatorCount":   len(seen),
			"validatorIndices": seen,
		}).Debug("All watched attestations seen in a gossiped aggregate")
	}
}

// allSeen reports whether every watched validator expected at the slot has been seen in an aggregate.
func (e *watchedSlotEntry) allSeen() bool {
	for _, rec := range e.validators {
		if !rec.seen {
			return false
		}
	}

	return true
}
