package sync

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/sirupsen/logrus"
)

// This file tracks every unaggregated attestation that this beacon node broadcast on behalf of a
// locally connected validator client, and logs whether that validator's vote is later seen included
// in an aggregate received on the beacon_aggregate_and_proof gossip subnet.

const (
	selfAttRetentionEpochs = 2
	selfAttChannelBuffer   = 512
)

var errSelfAttStateNotCached = errors.New("state not found in cache")

type (
	// selfAttRecord records a single attestation we submitted for a given validator.
	selfAttRecord struct {
		submittedAt time.Time                 //wall-clock time at which we broadcast the attestation.
		committee   primitives.CommitteeIndex //committee the validator attested in (AttestationData index is 0 post-Electra).
		seen        bool                      //true once the validator's vote has been observed in a gossiped aggregate.
	}

	// selfAttEntry groups all of our validators that submitted an attestation for the same AttestationData.
	selfAttEntry struct {
		slot       primitives.Slot
		validators map[primitives.ValidatorIndex]*selfAttRecord
		logged     bool
	}
)

// monitorSelfSubmittedAttestations consumes LocalAttestationSubmitted events and periodically prunes
// stale entries. It runs until the service context is closed.
func (s *Service) monitorSelfSubmittedAttestations() {
	channel := make(chan *feed.Event, selfAttChannelBuffer)
	subscription := s.cfg.attestationNotifier.OperationFeed().Subscribe(channel)
	defer subscription.Unsubscribe()

	ticker := time.NewTicker(time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case e := <-channel:
			if e.Type != operation.LocalAttestationSubmitted {
				continue
			}

			data, ok := e.Data.(*operation.LocalAttestationSubmittedData)
			if !ok {
				log.Error("Event feed data is not of type *operation.LocalAttestationSubmittedData")
				continue
			}

			s.recordSelfSubmittedAttestation(data.Attestation)
		case <-ticker.C:
			s.pruneSelfSubmittedAttestations(s.cfg.clock.CurrentSlot())
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting self attestation monitor")
			return
		case err := <-subscription.Err():
			log.WithError(err).Error("Could not subscribe to operation notifier")
			return
		}
	}
}

// recordSelfSubmittedAttestation registers an attestation that we broadcast on behalf of a local
// validator client.
func (s *Service) recordSelfSubmittedAttestation(att ethpb.Att) {
	data := att.GetData()
	if data == nil {
		return
	}

	root, err := data.HashTreeRoot()
	if err != nil {
		log.WithError(err).Error("Could not compute submitted attestation data root")
		return
	}

	idx, ok := s.unaggregatedValidatorIndex(s.ctx, att)
	if !ok {
		return
	}
	committee := att.GetCommitteeIndex()

	s.selfSubmittedAttsLock.Lock()
	defer s.selfSubmittedAttsLock.Unlock()

	e, ok := s.selfSubmittedAtts[root]
	if !ok {
		s.selfSubmittedAtts[root] = &selfAttEntry{
			slot:       data.Slot,
			validators: map[primitives.ValidatorIndex]*selfAttRecord{idx: {submittedAt: time.Now(), committee: committee}},
		}

		return
	}

	if _, exists := e.validators[idx]; exists {
		return
	}

	e.validators[idx] = &selfAttRecord{submittedAt: time.Now(), committee: committee}
}

// matchSelfSubmittedAttestation marks our validators whose vote the given aggregate includes. Once
// every validator that submitted for the aggregate's data has been seen, it emits a single log line
// reporting them.
func (s *Service) matchSelfSubmittedAttestation(ctx context.Context, aggregate ethpb.Att) {
	ctx, span := trace.StartSpan(ctx, "sync.matchSelfSubmittedAttestation")
	defer span.End()

	if aggregate == nil || aggregate.GetData() == nil {
		return
	}

	root, err := aggregate.GetData().HashTreeRoot()
	if err != nil {
		log.WithError(err).Error("Could not compute aggregate data root")
		return
	}

	s.selfSubmittedAttsLock.Lock()
	defer s.selfSubmittedAttsLock.Unlock()

	e, ok := s.selfSubmittedAtts[root]
	if !ok {
		return
	}

	// Cheap pre-check: only do the committee/state work if at least one of our validators for this
	// data is still waiting to be seen.
	if e.allSeen() {
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

	for idx, sub := range e.validators {
		if !sub.seen && included[uint64(idx)] {
			sub.seen = true
		}
	}

	// Only report once every validator that submitted for this data has been seen in an aggregate.
	// Entries that are never completed are surfaced by pruneSelfSubmittedAttestations as misses.
	if !e.allSeen() {
		return
	}

	// Log at most once per data root, even if a validator was added after the entry first completed.
	if e.logged {
		return
	}
	e.logged = true

	validators := make([]primitives.ValidatorIndex, 0, len(e.validators))
	for idx := range e.validators {
		validators = append(validators, idx)
	}
	slices.Sort(validators)

	// committees is aligned 1:1 with validators: committees[i] is the committee validators[i] attested in.
	committees := make([]primitives.CommitteeIndex, len(validators))
	for i, idx := range validators {
		committees[i] = e.validators[idx].committee
	}

	log.WithFields(logrus.Fields{
		"slot":             aggregate.GetData().Slot,
		"committees":       committees,
		"attDataRoot":      fmt.Sprintf("%#x", bytesutil.Trunc(root[:])),
		"validatorCount":   len(validators),
		"validatorIndices": validators,
	}).Debug("Submitted attestations seen in gossiped aggregate")
}

// pruneSelfSubmittedAttestations removes entries older than the retention window. It emits a single
// warning summarizing how many of the pruned attestations were seen in an aggregate versus never
// seen, listing the indices of the validators whose attestations were never seen.
func (s *Service) pruneSelfSubmittedAttestations(slot primitives.Slot) {
	retention := primitives.Slot(selfAttRetentionEpochs) * params.BeaconConfig().SlotsPerEpoch
	if slot < retention {
		return
	}
	threshold := slot - retention

	s.selfSubmittedAttsLock.Lock()
	defer s.selfSubmittedAttsLock.Unlock()

	seenCount := 0
	notSeen := make([]primitives.ValidatorIndex, 0, len(s.selfSubmittedAtts))
	for root, e := range s.selfSubmittedAtts {
		if e.slot >= threshold {
			continue
		}

		for idx, sub := range e.validators {
			if sub.seen {
				seenCount++
				continue
			}

			notSeen = append(notSeen, idx)
		}

		delete(s.selfSubmittedAtts, root)
	}

	if len(notSeen) == 0 {
		return
	}

	slices.Sort(notSeen)
	log.WithFields(logrus.Fields{
		"seenCount":         seenCount,
		"notSeenCount":      len(notSeen),
		"notSeenValidators": notSeen,
		"prunedBeforeSlot":  threshold,
		"slotInEpoch":       threshold % params.BeaconConfig().SlotsPerEpoch,
	}).Warning("Submitted attestations never seen in a gossiped aggregate")
}

// allSeen reports whether every validator in the entry has already been seen in an aggregate.
func (e *selfAttEntry) allSeen() bool {
	for _, sub := range e.validators {
		if !sub.seen {
			return false
		}
	}

	return true
}

// unaggregatedValidatorIndex extracts the index of the validator whose vote an unaggregated
// attestation carries.
func (s *Service) unaggregatedValidatorIndex(ctx context.Context, att ethpb.Att) (primitives.ValidatorIndex, bool) {
	if single, ok := att.(*ethpb.SingleAttestation); ok {
		return single.AttesterIndex, true
	}

	indices, err := s.attestingIndices(ctx, att)
	if err != nil {
		if !errors.Is(err, errSelfAttStateNotCached) {
			log.WithError(err).Debug("Could not determine attesting index for submitted attestation")
		}

		return 0, false
	}

	if len(indices) != 1 {
		return 0, false
	}

	return primitives.ValidatorIndex(indices[0]), true
}

// attestingIndices returns the validator indices that participated in the given attestation,
// resolving its committee(s) from the cached state referenced by the attestation's head block root.
func (s *Service) attestingIndices(ctx context.Context, att ethpb.Att) ([]uint64, error) {
	root := bytesutil.ToBytes32(att.GetData().BeaconBlockRoot)
	st := s.cfg.stateGen.StateByRootIfCachedNoCopy(root)
	if st == nil {
		return nil, errSelfAttStateNotCached
	}

	committeeBits := att.CommitteeBitsVal().BitIndices()
	committees := make([][]primitives.ValidatorIndex, len(committeeBits))
	for i, ci := range committeeBits {
		committee, err := helpers.BeaconCommitteeFromState(ctx, st, att.GetData().Slot, primitives.CommitteeIndex(ci))
		if err != nil {
			return nil, fmt.Errorf("beacon committee from state: %w", err)
		}

		committees[i] = committee
	}

	attestingIndices, err := attestation.AttestingIndices(att, committees...)
	if err != nil {
		return nil, fmt.Errorf("attesting indices: %w", err)
	}

	return attestingIndices, nil
}
