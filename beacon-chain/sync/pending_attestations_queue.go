package sync

import (
	"bytes"
	"context"
	"encoding/hex"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/prysmaticlabs/prysm/v5/async"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/rand"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
)

// This defines how often a node cleans up and processes pending attestations in the queue.
var processPendingAttsPeriod = slots.DivideSlotBy(2 /* twice per slot */)
var pendingAttsLimit = 10000

// This processes pending attestation queues on every `processPendingAttsPeriod`.
func (s *Service) processPendingAttsQueue() {
	// Prevents multiple queue processing goroutines (invoked by RunEvery) from contending for data.
	mutex := new(sync.Mutex)
	async.RunEvery(s.ctx, processPendingAttsPeriod, func() {
		mutex.Lock()
		if err := s.processPendingAtts(s.ctx); err != nil {
			log.WithError(err).Debugf("Could not process pending attestation: %v", err)
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
	// be deleted from the queue if invalid (ie. getting staled from falling too many slots behind).
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
		if s.cfg.beaconDB.HasBlock(ctx, bRoot) && (s.cfg.beaconDB.HasState(ctx, bRoot) || s.cfg.beaconDB.HasStateSummary(ctx, bRoot)) {
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

func (s *Service) processAttestations(ctx context.Context, attestations []ethpb.SignedAggregateAttAndProof) {
	for _, signedAtt := range attestations {
		aggregate := signedAtt.AggregateAttestationAndProof().AggregateVal()
		data := aggregate.GetData()
		// The pending attestations can arrive in both aggregated and unaggregated forms,
		// each from has distinct validation steps.
		if aggregate.IsAggregated() {
			// Save the pending aggregated attestation to the pool if it passes the aggregated
			// validation steps.
			valRes, err := s.validateAggregatedAtt(ctx, signedAtt)
			if err != nil {
				log.WithError(err).Debug("Pending aggregated attestation failed validation")
			}
			aggValid := pubsub.ValidationAccept == valRes
			if s.validateBlockInAttestation(ctx, signedAtt) && aggValid {
				if features.Get().EnableExperimentalAttestationPool {
					if err = s.cfg.attestationCache.Add(aggregate); err != nil {
						log.WithError(err).Debug("Could not save aggregate attestation")
						continue
					}
				} else {
					if err := s.cfg.attPool.SaveAggregatedAttestation(aggregate); err != nil {
						log.WithError(err).Debug("Could not save aggregate attestation")
						continue
					}
				}

				s.setAggregatorIndexEpochSeen(data.Target.Epoch, signedAtt.AggregateAttestationAndProof().GetAggregatorIndex())

				// Broadcasting the signed attestation again once a node is able to process it.
				if err := s.cfg.p2p.Broadcast(ctx, signedAtt); err != nil {
					log.WithError(err).Debug("Could not broadcast")
				}
			}
		} else {
			// This is an important validation before retrieving attestation pre state to defend against
			// attestation's target intentionally reference checkpoint that's long ago.
			// Verify current finalized checkpoint is an ancestor of the block defined by the attestation's beacon block root.
			if !s.cfg.chain.InForkchoice(bytesutil.ToBytes32(data.BeaconBlockRoot)) {
				log.WithError(blockchain.ErrNotDescendantOfFinalized).Debug("Could not verify finalized consistency")
				continue
			}
			if err := s.cfg.chain.VerifyLmdFfgConsistency(ctx, aggregate); err != nil {
				log.WithError(err).Debug("Could not verify FFG consistency")
				continue
			}
			preState, err := s.cfg.chain.AttestationTargetState(ctx, data.Target)
			if err != nil {
				log.WithError(err).Debug("Could not retrieve attestation prestate")
				continue
			}

			valid, err := s.validateUnaggregatedAttWithState(ctx, aggregate, preState)
			if err != nil {
				log.WithError(err).Debug("Pending unaggregated attestation failed validation")
				continue
			}
			if valid == pubsub.ValidationAccept {
				if features.Get().EnableExperimentalAttestationPool {
					if err = s.cfg.attestationCache.Add(aggregate); err != nil {
						log.WithError(err).Debug("Could not save unaggregated attestation")
						continue
					}
				} else {
					if err := s.cfg.attPool.SaveUnaggregatedAttestation(aggregate); err != nil {
						log.WithError(err).Debug("Could not save unaggregated attestation")
						continue
					}
				}
				s.setSeenCommitteeIndicesSlot(data.Slot, data.CommitteeIndex, aggregate.GetAggregationBits())

				valCount, err := helpers.ActiveValidatorCount(ctx, preState, slots.ToEpoch(data.Slot))
				if err != nil {
					log.WithError(err).Debug("Could not retrieve active validator count")
					continue
				}
				// Broadcasting the signed attestation again once a node is able to process it.
				if err := s.cfg.p2p.BroadcastAttestation(ctx, helpers.ComputeSubnetForAttestation(valCount, aggregate), aggregate); err != nil {
					log.WithError(err).Debug("Could not broadcast")
				}
			}
		}
	}
}

// This defines how pending attestations is saved in the map. The key is the
// root of the missing block. The value is the list of pending attestations
// that voted for that block root. The caller of this function is responsible
// for not sending repeated attestations to the pending queue.
func (s *Service) savePendingAtt(att ethpb.SignedAggregateAttAndProof) {
	root := bytesutil.ToBytes32(att.AggregateAttestationAndProof().AggregateVal().GetData().BeaconBlockRoot)

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
		s.blkRootToPendingAtts[root] = []ethpb.SignedAggregateAttAndProof{att}
		return
	}
	// Skip if the attestation from the same aggregator already exists in
	// the pending queue.
	for _, a := range s.blkRootToPendingAtts[root] {
		if attsAreEqual(att, a) {
			return
		}
	}
	pendingAttCount.Inc()
	s.blkRootToPendingAtts[root] = append(s.blkRootToPendingAtts[root], att)
}

func attsAreEqual(a, b ethpb.SignedAggregateAttAndProof) bool {
	if a.GetSignature() != nil {
		return b.GetSignature() != nil && a.AggregateAttestationAndProof().GetAggregatorIndex() == b.AggregateAttestationAndProof().GetAggregatorIndex()
	}
	if b.GetSignature() != nil {
		return false
	}

	aAggregate := a.AggregateAttestationAndProof().AggregateVal()
	bAggregate := b.AggregateAttestationAndProof().AggregateVal()
	aData := aAggregate.GetData()
	bData := bAggregate.GetData()

	if aData.Slot != bData.Slot {
		return false
	}

	if a.Version() >= version.Electra {
		if !bytes.Equal(aAggregate.CommitteeBitsVal().Bytes(), bAggregate.CommitteeBitsVal().Bytes()) {
			return false
		}
	} else if aData.CommitteeIndex != bData.CommitteeIndex {
		return false
	}

	return bytes.Equal(aAggregate.GetAggregationBits(), bAggregate.GetAggregationBits())
}

// This validates the pending attestations in the queue are still valid.
// If not valid, a node will remove it in the queue in place. The validity
// check specifies the pending attestation could not fall one epoch behind
// of the current slot.
func (s *Service) validatePendingAtts(ctx context.Context, slot primitives.Slot) {
	_, span := trace.StartSpan(ctx, "validatePendingAtts")
	defer span.End()

	s.pendingAttsLock.Lock()
	defer s.pendingAttsLock.Unlock()

	for bRoot, atts := range s.blkRootToPendingAtts {
		for i := len(atts) - 1; i >= 0; i-- {
			if slot >= atts[i].AggregateAttestationAndProof().AggregateVal().GetData().Slot+params.BeaconConfig().SlotsPerEpoch {
				// Remove the pending attestation from the list in place.
				atts = append(atts[:i], atts[i+1:]...)
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
