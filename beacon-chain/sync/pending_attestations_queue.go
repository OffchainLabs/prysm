package sync

import (
	"bytes"
	"context"
	"encoding/hex"
	"sync"

	"github.com/OffchainLabs/prysm/v6/async"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/rand"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/sirupsen/logrus"
)

// This defines how often a node cleans up and processes pending attestations in the queue.
var processPendingAttsPeriod = slots.DivideSlotBy(2 /* twice per slot */)
var pendingAttsLimit = 10000

// This processes pending attestation queues on every processPendingAttsPeriod.
func (s *Service) runPendingAttsQueue() {
	// Prevents multiple queue processing goroutines (invoked by RunEvery) from contending for data.
	mutex := new(sync.Mutex)
	async.RunEvery(s.ctx, processPendingAttsPeriod, func() {
		mutex.Lock()
		if err := s.processPendingAtts(s.ctx); err != nil {
			log.WithError(err).Debug("Could not process pending attestation")
		}
		mutex.Unlock()
	})
}

// This defines how pending attestations are processed. It contains features:
// 1. Clean up invalid pending attestations from the queue.
// 2. Check if pending attestations can be processed when the block has arrived.
// 3. Request block from a random peer if unable to proceed step 2.
func (s *Service) processPendingAtts(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "processPendingAtts")
	defer span.End()

	// Before a node processes pending attestations queue, it verifies
	// the attestations in the queue are still valid. Attestations will
	// be deleted from the queue if invalid (i.e. getting stalled from falling too many slots behind).
	s.validatePendingAtts(ctx, s.cfg.clock.CurrentSlot())

	s.pendingAttsLock.RLock()
	roots := make([][32]byte, 0, len(s.blkRootToPendingAtts))
	for br := range s.blkRootToPendingAtts {
		roots = append(roots, br)
	}
	s.pendingAttsLock.RUnlock()

	var pendingRoots [][32]byte
	randGen := rand.NewGenerator()
	for _, bRoot := range roots {
		s.pendingAttsLock.RLock()
		attestations := s.blkRootToPendingAtts[bRoot]
		s.pendingAttsLock.RUnlock()
		// has the pending attestation's missing block arrived and the node processed block yet?
		if s.cfg.beaconDB.HasBlock(ctx, bRoot) && (s.cfg.beaconDB.HasState(ctx, bRoot) || s.cfg.beaconDB.HasStateSummary(ctx, bRoot)) && s.cfg.chain.InForkchoice(bRoot) {
			s.processAttestations(ctx, attestations)
			log.WithFields(logrus.Fields{
				"blockRoot":        hex.EncodeToString(bytesutil.Trunc(bRoot[:])),
				"pendingAttsCount": len(attestations),
			}).Debug("Verified and saved pending attestations to pool")

			// Delete the missing block root key from pending attestation queue so a node will not request for the block again.
			s.pendingAttsLock.Lock()
			delete(s.blkRootToPendingAtts, bRoot)
			s.pendingAttsLock.Unlock()
		} else {
			s.pendingQueueLock.RLock()
			seen := s.seenPendingBlocks[bRoot]
			s.pendingQueueLock.RUnlock()
			if !seen {
				pendingRoots = append(pendingRoots, bRoot)
			}
		}
	}
	return s.sendBatchRootRequest(ctx, pendingRoots, randGen)
}

func (s *Service) processAttestations(ctx context.Context, attestations []any) {
	for _, signedAtt := range attestations {
		// The pending attestations can arrive as both aggregates and attestations,
		// and each form has to be processed differently.
		switch t := signedAtt.(type) {
		case ethpb.Att:
			s.processAtt(ctx, t)
		case ethpb.SignedAggregateAttAndProof:
			s.processAggregate(ctx, t)
		default:
			log.Warnf("Unexpected item of type %T in pending attestation queue. Item will not be processed", t)
		}
	}
}

func (s *Service) processAggregate(ctx context.Context, aggregate ethpb.SignedAggregateAttAndProof) {
	att := aggregate.AggregateAttestationAndProof().AggregateVal()

	// Save the pending aggregated attestation to the pool if it passes the aggregated
	// validation steps.
	valRes, err := s.validateAggregatedAtt(ctx, aggregate)
	if err != nil {
		log.WithError(err).Debug("Pending aggregated attestation failed validation")
	}
	aggValid := pubsub.ValidationAccept == valRes
	if s.validateBlockInAttestation(ctx, aggregate) && aggValid {
		if features.Get().EnableExperimentalAttestationPool {
			if err = s.cfg.attestationCache.Add(att); err != nil {
				log.WithError(err).Debug("Could not save aggregated attestation")
				return
			}
		} else {
			if att.IsAggregated() {
				if err = s.cfg.attPool.SaveAggregatedAttestation(att); err != nil {
					log.WithError(err).Debug("Could not save aggregated attestation")
					return
				}
			} else if err = s.cfg.attPool.SaveUnaggregatedAttestation(att); err != nil {
				log.WithError(err).Debug("Could not save unaggregated attestation")
				return
			}
		}

		s.setAggregatorIndexEpochSeen(att.GetData().Target.Epoch, aggregate.AggregateAttestationAndProof().GetAggregatorIndex())

		// Broadcasting the signed attestation again once a node is able to process it.
		if err := s.cfg.p2p.Broadcast(ctx, aggregate); err != nil {
			log.WithError(err).Debug("Could not broadcast")
		}
	}
}

func (s *Service) processAtt(ctx context.Context, att ethpb.Att) {
	data := att.GetData()

	// This is an important validation before retrieving attestation pre state to defend against
	// attestation's target intentionally referencing a checkpoint that's long ago.
	if !s.cfg.chain.InForkchoice(bytesutil.ToBytes32(data.BeaconBlockRoot)) {
		log.WithError(blockchain.ErrNotDescendantOfFinalized).Debug("Could not verify finalized consistency")
		return
	}
	if err := s.cfg.chain.VerifyLmdFfgConsistency(ctx, att); err != nil {
		log.WithError(err).Debug("Could not verify FFG consistency")
		return
	}
	preState, err := s.cfg.chain.AttestationTargetState(ctx, data.Target)
	if err != nil {
		log.WithError(err).Debug("Could not retrieve attestation prestate")
		return
	}
	committee, err := helpers.BeaconCommitteeFromState(ctx, preState, data.Slot, att.GetCommitteeIndex())
	if err != nil {
		log.WithError(err).Debug("Could not retrieve committee from state")
		return
	}
	valid, err := validateAttesterData(ctx, att, committee)
	if err != nil {
		log.WithError(err).Debug("Could not validate attester data")
		return
	} else if valid != pubsub.ValidationAccept {
		log.Debug("Attestation failed attester data validation")
		return
	}

	// Decide if the attestation is an Electra SingleAttestation or a Phase0 unaggregated attestation
	var (
		attForValidation ethpb.Att
		broadcastAtt     ethpb.Att
		eventType        feed.EventType
		eventData        interface{}
	)

	if att.Version() >= version.Electra {
		singleAtt, ok := att.(*ethpb.SingleAttestation)
		if !ok {
			log.Debugf("Attestation has wrong type (expected %T, got %T)", &ethpb.SingleAttestation{}, att)
			return
		}
		// Convert Electra SingleAttestation to unaggregated ElectraAttestation. This is needed because many parts of the codebase assume that attestations have a certain structure and SingleAttestation validates these assumptions.
		attForValidation = singleAtt.ToAttestationElectra(committee)
		broadcastAtt = singleAtt
		eventType = operation.SingleAttReceived
		eventData = &operation.SingleAttReceivedData{
			Attestation: singleAtt,
		}
	} else {
		// Phase0 attestation
		attForValidation = att
		broadcastAtt = att
		eventType = operation.UnaggregatedAttReceived
		eventData = &operation.UnAggregatedAttReceivedData{
			Attestation: att,
		}
	}

	valid, err = s.validateUnaggregatedAttWithState(ctx, attForValidation, preState)
	if err != nil {
		log.WithError(err).Debug("Pending unaggregated attestation failed validation")
		return
	}

	if valid == pubsub.ValidationAccept {
		if features.Get().EnableExperimentalAttestationPool {
			if err = s.cfg.attestationCache.Add(attForValidation); err != nil {
				log.WithError(err).Debug("Could not save unaggregated attestation")
				return
			}
		} else {
			if err := s.cfg.attPool.SaveUnaggregatedAttestation(attForValidation); err != nil {
				log.WithError(err).Debug("Could not save unaggregated attestation")
				return
			}
		}

		s.setSeenUnaggregatedAtt(att)

		valCount, err := helpers.ActiveValidatorCount(ctx, preState, slots.ToEpoch(data.Slot))
		if err != nil {
			log.WithError(err).Debug("Could not retrieve active validator count")
			return
		}

		// Broadcast the final 'broadcastAtt' object
		if err := s.cfg.p2p.BroadcastAttestation(ctx, helpers.ComputeSubnetForAttestation(valCount, broadcastAtt), broadcastAtt); err != nil {
			log.WithError(err).Debug("Could not broadcast")
		}

		// Feed event notification for other services
		s.cfg.attestationNotifier.OperationFeed().Send(&feed.Event{
			Type: eventType,
			Data: eventData,
		})
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
