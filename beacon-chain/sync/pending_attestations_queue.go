package sync

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/rand"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/sirupsen/logrus"
)

var pendingAttsLimit = 32768

// This method processes pending attestations as a "known" block as arrived. With validations,
// the valid attestations get saved into the operation mem pool, and the invalid attestations gets deleted
// from the sync pending pool.
func (s *Service) processPendingAttsForBlock(ctx context.Context, bRoot [32]byte) error {
	ctx, span := trace.StartSpan(ctx, "processPendingAttsForBlock")
	defer span.End()

	// Confirm that the pending attestation's missing block arrived and the node processed the block.
	if !s.cfg.beaconDB.HasBlock(ctx, bRoot) || !(s.cfg.beaconDB.HasState(ctx, bRoot) || s.cfg.beaconDB.HasStateSummary(ctx, bRoot)) || !s.cfg.chain.InForkchoice(bRoot) {
		return fmt.Errorf("could not process unknown block root %#x", bRoot)
	}

	// Before a node processes pending attestations queue, it verifies
	// the attestations in the queue are still valid. Attestations will
	// be deleted from the queue if invalid (i.e. getting stalled from falling too many slots behind).
	s.validatePendingAtts(ctx, s.cfg.clock.CurrentSlot())

	s.pendingAttsLock.RLock()
	attestations := s.blkRootToPendingAtts[bRoot]
	s.pendingAttsLock.RUnlock()

	if len(attestations) > 0 {
		start := time.Now()
		s.processAttestations(ctx, attestations)
		duration := time.Since(start)
		log.WithFields(logrus.Fields{
			"blockRoot":        hex.EncodeToString(bytesutil.Trunc(bRoot[:])),
			"pendingAttsCount": len(attestations),
			"duration":         duration,
		}).Debug("Verified and saved pending attestations to pool")
	}

	randGen := rand.NewGenerator()
	// Delete the missing block root key from pending attestation queue so a node will not request for the block again.
	s.pendingAttsLock.Lock()
	delete(s.blkRootToPendingAtts, bRoot)
	pendingRoots := make([][32]byte, 0, len(s.blkRootToPendingAtts))
	s.pendingQueueLock.RLock()
	for r := range s.blkRootToPendingAtts {
		if !s.seenPendingBlocks[r] {
			pendingRoots = append(pendingRoots, r)
		}
	}
	s.pendingQueueLock.RUnlock()
	s.pendingAttsLock.Unlock()

	//  Request the blocks for the pending attestations that could not be processed.
	return s.sendBatchRootRequest(ctx, pendingRoots, randGen)
}

func (s *Service) processAttestations(ctx context.Context, attestations []any) {
	if len(attestations) == 0 {
		return
	}

	atts := make([]ethpb.Att, 0, len(attestations))
	for _, att := range attestations {
		switch v := att.(type) {
		case ethpb.Att:
			atts = append(atts, v)
		case ethpb.SignedAggregateAttAndProof:
			s.processAggregate(ctx, v)
		default:
			log.Warnf("Unexpected attestation type %T, skipping", v)
		}
	}

	for _, bucket := range bucketAttestationsByData(atts) {
		s.processAttestationBucket(ctx, bucket)
	}
}

// attestationBucket groups attestations with the same AttestationData for batch processing.
type attestationBucket struct {
	dataHash     [32]byte
	data         *ethpb.AttestationData
	attestations []ethpb.Att
}

// processAttestationBucket processes a bucket of attestations with shared AttestationData.
func (s *Service) processAttestationBucket(ctx context.Context, bucket *attestationBucket) {
	if bucket == nil || len(bucket.attestations) == 0 {
		return
	}

	data := bucket.data

	// Shared validations for the entire bucket.
	if !s.cfg.chain.InForkchoice(bytesutil.ToBytes32(data.BeaconBlockRoot)) {
		log.WithError(blockchain.ErrNotDescendantOfFinalized).WithField("root", fmt.Sprintf("%#x", data.BeaconBlockRoot)).Debug("Failed forkchoice check for bucket")
		return
	}

	preState, err := s.cfg.chain.AttestationTargetState(ctx, data.Target)
	if err != nil {
		log.WithError(err).Debug("Failed to get attestation prestate for bucket")
		return
	}

	if err := s.cfg.chain.VerifyLmdFfgConsistency(ctx, bucket.attestations[0]); err != nil {
		log.WithError(err).Debug("Failed FFG consistency check for bucket")
		return
	}

	// Collect valid attestations for both single and electra formats.
	// Broadcast takes single format but attestation pool and batch signature verification take electra format.
	forBroadcast := make([]ethpb.Att, 0, len(bucket.attestations))
	forPool := make([]ethpb.Att, 0, len(bucket.attestations))

	for _, att := range bucket.attestations {
		committee, err := helpers.BeaconCommitteeFromState(ctx, preState, data.Slot, att.GetCommitteeIndex())
		if err != nil {
			log.WithError(err).Debug("Failed to get committee from state")
			continue
		}

		valid, err := validateAttesterData(ctx, att, committee)
		if err != nil {
			log.WithError(err).Debug("Failed attester data validation")
			continue
		}
		if valid != pubsub.ValidationAccept {
			log.Debug("Pending attestation rejected due to invalid data")
			continue
		}

		var conv ethpb.Att
		if att.Version() >= version.Electra {
			single, ok := att.(*ethpb.SingleAttestation)
			if !ok {
				log.Debugf("Wrong type: expected %T, got %T", &ethpb.SingleAttestation{}, att)
				continue
			}
			conv = single.ToAttestationElectra(committee)
		} else {
			conv = att
		}

		forBroadcast = append(forBroadcast, att)
		forPool = append(forPool, conv)
	}

	if len(forPool) == 0 {
		return
	}

	verified := s.batchVerifyAttestationSignatures(ctx, forPool, preState)
	verifiedSet := make(map[ethpb.Att]struct{}, len(verified))
	for _, att := range verified {
		verifiedSet[att] = struct{}{}
	}

	for i, poolAtt := range forPool {
		if _, ok := verifiedSet[poolAtt]; ok {
			s.processVerifiedAttestation(ctx, forBroadcast[i], poolAtt, preState)
		}
	}
}

// batchVerifyAttestationSignatures attempts batch verification, with individual fallback on failure.
func (s *Service) batchVerifyAttestationSignatures(
	ctx context.Context,
	attestations []ethpb.Att,
	preState state.ReadOnlyBeaconState,
) []ethpb.Att {
	const fallbackMsg = "batch verification failed, using individual checks"

	set, err := blocks.AttestationSignatureBatch(ctx, preState, attestations)
	if err != nil {
		log.WithError(err).Debug(fallbackMsg)
		return s.fallbackToIndividualVerification(ctx, attestations, preState)
	}

	ok, err := set.Verify()
	if err != nil || !ok {
		if err != nil {
			log.WithError(err).Debug(fallbackMsg)
		} else {
			log.Debug(fallbackMsg)
		}
		return s.fallbackToIndividualVerification(ctx, attestations, preState)
	}

	return attestations
}

// fallbackToIndividualVerification verifies each attestation individually if batch verification fails.
func (s *Service) fallbackToIndividualVerification(
	ctx context.Context,
	attestations []ethpb.Att,
	preState state.ReadOnlyBeaconState,
) []ethpb.Att {
	verified := make([]ethpb.Att, 0, len(attestations))

	for _, att := range attestations {
		res, err := s.validateUnaggregatedAttWithState(ctx, att, preState)
		if err != nil {
			log.WithError(err).Debug("Individual signature verification error")
			continue
		}
		if res == pubsub.ValidationAccept {
			verified = append(verified, att)
		}
	}

	return verified
}

// saveAttestation saves an attestation to the appropriate pool.
func (s *Service) saveAttestation(att ethpb.Att) error {
	if features.Get().EnableExperimentalAttestationPool {
		return s.cfg.attestationCache.Add(att)
	}
	if att.IsAggregated() {
		return s.cfg.attPool.SaveAggregatedAttestation(att)
	}
	return s.cfg.attPool.SaveUnaggregatedAttestation(att)
}

// processVerifiedAttestation handles a signature-verified attestation.
func (s *Service) processVerifiedAttestation(
	ctx context.Context,
	broadcastAtt ethpb.Att,
	poolAtt ethpb.Att,
	preState state.ReadOnlyBeaconState,
) {
	data := broadcastAtt.GetData()

	if err := s.saveAttestation(poolAtt); err != nil {
		log.WithError(err).Debug("Failed to save unaggregated attestation")
		return
	}

	if key, err := generateUnaggregatedAttCacheKey(broadcastAtt); err != nil {
		log.WithError(err).Error("Failed to generate cache key for attestation tracking")
	} else {
		s.setSeenUnaggregatedAtt(key)
	}

	valCount, err := helpers.ActiveValidatorCount(ctx, preState, slots.ToEpoch(data.Slot))
	if err != nil {
		log.WithError(err).Debug("Failed to retrieve active validator count")
		return
	}

	if err := s.cfg.p2p.BroadcastAttestation(ctx, helpers.ComputeSubnetForAttestation(valCount, broadcastAtt), broadcastAtt); err != nil {
		log.WithError(err).Debug("Failed to broadcast attestation")
	}

	var (
		eventType feed.EventType
		eventData any
	)

	switch {
	case broadcastAtt.Version() >= version.Electra:
		if sa, ok := broadcastAtt.(*ethpb.SingleAttestation); ok {
			eventType = operation.SingleAttReceived
			eventData = &operation.SingleAttReceivedData{Attestation: sa}
			break
		}
		fallthrough
	default:
		eventType = operation.UnaggregatedAttReceived
		eventData = &operation.UnAggregatedAttReceivedData{Attestation: broadcastAtt}
	}

	// Send event notification
	s.cfg.attestationNotifier.OperationFeed().Send(&feed.Event{
		Type: eventType,
		Data: eventData,
	})
}

func (s *Service) processAggregate(ctx context.Context, aggregate ethpb.SignedAggregateAttAndProof) {
	res, err := s.validateAggregatedAtt(ctx, aggregate)
	if err != nil {
		log.WithError(err).Debug("Pending aggregated attestation failed validation")
		return
	}
	if res != pubsub.ValidationAccept || !s.validateBlockInAttestation(ctx, aggregate) {
		log.Debug("Pending aggregated attestation failed validation")
		return
	}

	att := aggregate.AggregateAttestationAndProof().AggregateVal()
	if err := s.saveAttestation(att); err != nil {
		log.WithError(err).Debug("Could not save aggregated attestation")
		return
	}

	s.setAggregatorIndexEpochSeen(att.GetData().Target.Epoch, aggregate.AggregateAttestationAndProof().GetAggregatorIndex())

	if err := s.cfg.p2p.Broadcast(ctx, aggregate); err != nil {
		log.WithError(err).Debug("Could not broadcast aggregated attestation")
	}
}

// This defines how pending aggregates are saved in the map. The key is the
// root of the missing block. The value is the list of pending attestations/aggregates
// that voted for that block root. The caller of this function is responsible
// for not sending repeated aggregates to the pending queue.
func (s *Service) savePendingAggregate(agg ethpb.SignedAggregateAttAndProof) {
	root := bytesutil.ToBytes32(agg.AggregateAttestationAndProof().AggregateVal().GetData().BeaconBlockRoot)

	s.savePending(root, agg, func(other any) bool {
		a, ok := other.(ethpb.SignedAggregateAttAndProof)
		return ok && pendingAggregatesAreEqual(agg, a)
	})
}

// This defines how pending attestations are saved in the map. The key is the
// root of the missing block. The value is the list of pending attestations/aggregates
// that voted for that block root. The caller of this function is responsible
// for not sending repeated attestations to the pending queue.
func (s *Service) savePendingAtt(att ethpb.Att) {
	if att.Version() >= version.Electra && !att.IsSingle() {
		log.Debug("Non-single attestation sent to pending attestation pool. Attestation will be ignored")
		return
	}

	root := bytesutil.ToBytes32(att.GetData().BeaconBlockRoot)

	s.savePending(root, att, func(other any) bool {
		a, ok := other.(ethpb.Att)
		return ok && pendingAttsAreEqual(att, a)
	})
}

// We want to avoid saving duplicate items, which is the purpose of the passed-in closure.
// It is the responsibility of the caller to provide a function that correctly determines quality
// in the context of the pending queue.
func (s *Service) savePending(root [32]byte, pending any, isEqual func(other any) bool) {
	s.pendingAttsLock.Lock()
	defer s.pendingAttsLock.Unlock()

	numOfPendingAtts := 0
	for _, v := range s.blkRootToPendingAtts {
		numOfPendingAtts += len(v)
	}
	// Exit early if we exceed the pending attestations limit.
	if numOfPendingAtts >= pendingAttsLimit {
		return
	}

	_, ok := s.blkRootToPendingAtts[root]
	if !ok {
		pendingAttCount.Inc()
		s.blkRootToPendingAtts[root] = []any{pending}
		return
	}

	// Skip if the attestation/aggregate from the same validator already exists in
	// the pending queue.
	for _, a := range s.blkRootToPendingAtts[root] {
		if isEqual(a) {
			return
		}
	}

	pendingAttCount.Inc()
	s.blkRootToPendingAtts[root] = append(s.blkRootToPendingAtts[root], pending)
}

func pendingAggregatesAreEqual(a, b ethpb.SignedAggregateAttAndProof) bool {
	if a.Version() != b.Version() {
		return false
	}
	if a.AggregateAttestationAndProof().GetAggregatorIndex() != b.AggregateAttestationAndProof().GetAggregatorIndex() {
		return false
	}
	aAtt := a.AggregateAttestationAndProof().AggregateVal()
	bAtt := b.AggregateAttestationAndProof().AggregateVal()
	if aAtt.GetData().Slot != bAtt.GetData().Slot {
		return false
	}
	if aAtt.GetCommitteeIndex() != bAtt.GetCommitteeIndex() {
		return false
	}
	return bytes.Equal(aAtt.GetAggregationBits(), bAtt.GetAggregationBits())
}

func pendingAttsAreEqual(a, b ethpb.Att) bool {
	if a.Version() != b.Version() {
		return false
	}
	if a.GetData().Slot != b.GetData().Slot {
		return false
	}
	if a.Version() >= version.Electra {
		return a.GetAttestingIndex() == b.GetAttestingIndex()
	}
	if a.GetCommitteeIndex() != b.GetCommitteeIndex() {
		return false
	}
	return bytes.Equal(a.GetAggregationBits(), b.GetAggregationBits())
}

// This validates the pending attestations in the queue are still valid.
// If not valid, a node will remove it from the queue in place. The validity
// check specifies the pending attestation cannot fall one epoch behind
// the current slot.
func (s *Service) validatePendingAtts(ctx context.Context, slot primitives.Slot) {
	_, span := trace.StartSpan(ctx, "validatePendingAtts")
	defer span.End()

	s.pendingAttsLock.Lock()
	defer s.pendingAttsLock.Unlock()

	for bRoot, atts := range s.blkRootToPendingAtts {
		for i := len(atts) - 1; i >= 0; i-- {
			var attSlot primitives.Slot
			switch t := atts[i].(type) {
			case ethpb.Att:
				attSlot = t.GetData().Slot
			case ethpb.SignedAggregateAttAndProof:
				attSlot = t.AggregateAttestationAndProof().AggregateVal().GetData().Slot
			default:
				log.Debugf("Unexpected item of type %T in pending attestation queue. Item will be removed", t)
				// Remove the pending attestation from the map in place.
				atts[i] = atts[len(atts)-1]
				atts = atts[:len(atts)-1]
				continue
			}
			if slot >= attSlot+params.BeaconConfig().SlotsPerEpoch {
				// Remove the pending attestation from the map in place.
				atts[i] = atts[len(atts)-1]
				atts = atts[:len(atts)-1]
			}
		}
		s.blkRootToPendingAtts[bRoot] = atts

		// If the pending attestations list of a given block root is empty,
		// a node will remove the key from the map to avoid dangling keys.
		if len(s.blkRootToPendingAtts[bRoot]) == 0 {
			delete(s.blkRootToPendingAtts, bRoot)
		}
	}
}

// bucketAttestationsByData groups attestations by their AttestationData hash.
func bucketAttestationsByData(attestations []ethpb.Att) map[[32]byte]*attestationBucket {
	bucketMap := make(map[[32]byte]*attestationBucket)

	for _, att := range attestations {
		data := att.GetData()
		dataHash, err := data.HashTreeRoot()
		if err != nil {
			log.WithError(err).Debug("Failed to hash attestation data, skipping attestation")
			continue
		}

		if bucket, ok := bucketMap[dataHash]; ok {
			bucket.attestations = append(bucket.attestations, att)
		} else {
			bucketMap[dataHash] = &attestationBucket{
				dataHash:     dataHash,
				data:         data,
				attestations: []ethpb.Att{att},
			}
		}
	}

	return bucketMap
}
