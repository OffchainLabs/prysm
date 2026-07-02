package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	payloadattestation "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attestation"
	"github.com/OffchainLabs/prysm/v7/crypto/rand"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

const maxPendingPayloadAttestationRoots = 2

func (s *Service) queuePendingPayloadAttestation(ctx context.Context, v verification.PayloadAttestationMsgVerifier, att *eth.PayloadAttestationMessage) (pubsub.ValidationResult, error) {
	root := bytesutil.ToBytes32(att.Data.BeaconBlockRoot)
	validatorIndex := att.ValidatorIndex

	st, err := s.cfg.chain.HeadStateReadOnly(ctx)
	if err != nil || st == nil {
		return pubsub.ValidationIgnore, err
	}

	s.pendingPayloadAttestationLock.Lock()
	inner, rootExists := s.pendingPayloadAttestations[root]
	if !rootExists && len(s.pendingPayloadAttestations) >= maxPendingPayloadAttestationRoots {
		s.pendingPayloadAttestationLock.Unlock()
		log.Debug("Too many pending payload attestation roots, ignoring new payload attestation")
		return pubsub.ValidationIgnore, nil
	}
	for _, existing := range inner {
		if existing.ValidatorIndex == validatorIndex {
			s.pendingPayloadAttestationLock.Unlock()
			return pubsub.ValidationIgnore, nil
		}
	}
	if err := v.VerifyValidatorInPTC(ctx, st); err != nil {
		s.pendingPayloadAttestationLock.Unlock()
		return pubsub.ValidationIgnore, err
	}
	if err := v.VerifySignature(st); err != nil {
		s.pendingPayloadAttestationLock.Unlock()
		return pubsub.ValidationReject, err
	}
	s.pendingPayloadAttestations[root] = append(inner, att)
	s.pendingPayloadAttestationLock.Unlock()

	if s.cfg.chain.InForkchoice(root) {
		go s.processPendingPayloadAttestation(s.ctx, root)
		return pubsub.ValidationIgnore, nil
	}

	s.pendingQueueLock.RLock()
	inPendingQueue := s.seenPendingBlocks[root]
	s.pendingQueueLock.RUnlock()
	if !rootExists && !inPendingQueue && !s.cfg.chain.BlockBeingSynced(root) {
		go func() {
			if err := s.sendBatchRootRequest(s.ctx, [][32]byte{root}, rand.NewGenerator()); err != nil {
				log.WithError(err).Debug("Could not request beacon block for pending payload attestation")
			}
		}()
	}
	return pubsub.ValidationIgnore, nil
}

func (s *Service) processPendingPayloadAttestation(ctx context.Context, root [32]byte) {
	s.pendingPayloadAttestationLock.Lock()
	atts, ok := s.pendingPayloadAttestations[root]
	if !ok {
		s.pendingPayloadAttestationLock.Unlock()
		return
	}
	delete(s.pendingPayloadAttestations, root)
	s.pendingPayloadAttestationLock.Unlock()

	if len(atts) == 0 {
		return
	}

	blockSlot, err := s.cfg.chain.RecentBlockSlot(root)
	if err != nil {
		log.WithError(err).Debug("Could not get block slot for pending payload attestations")
		return
	}

	for _, att := range atts {
		pa, err := payloadattestation.NewReadOnly(att)
		if err != nil {
			log.WithError(err).Debug("Could not create read only pending payload attestation")
			continue
		}
		if s.payloadAttestationCache.Seen(pa.Slot(), pa.ValidatorIndex()) {
			continue
		}
		if blockSlot != pa.Slot() {
			continue
		}
		if err := s.processPayloadAttestationMessage(ctx, att); err != nil {
			log.WithError(err).Debug("Could not process pending payload attestation")
			continue
		}
		if err := s.cfg.p2p.Broadcast(ctx, att); err != nil {
			log.WithError(err).Warn("Could not broadcast pending payload attestation")
		}
	}
}

func (s *Service) prunePendingPayloadAttestations() {
	s.pendingPayloadAttestationLock.Lock()
	defer s.pendingPayloadAttestationLock.Unlock()
	if len(s.pendingPayloadAttestations) == 0 {
		return
	}
	currentSlot := s.cfg.clock.CurrentSlot()
	for root, atts := range s.pendingPayloadAttestations {
		if len(atts) == 0 || atts[0].Data == nil || atts[0].Data.Slot+1 < currentSlot {
			delete(s.pendingPayloadAttestations, root)
		}
	}
}
