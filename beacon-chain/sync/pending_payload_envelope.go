package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/async"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// processPendingPayloadEnvelopeQueue sweeps the pending envelope map at
// mid-slot to recover envelopes orphaned by the race between
// queuePendingPayloadEnvelope and beaconBlockSubscriber.
func (s *Service) processPendingPayloadEnvelopeQueue() {
	async.RunEvery(s.ctx, slots.DivideSlotBy(2), func() {
		if !s.chainIsStarted() {
			return
		}
		s.processPendingPayloadEnvelopes(s.ctx)
	})
}

// processPendingPayloadEnvelope retrieves a queued payload envelope for the
// given block root and calls ReceiveExecutionPayloadEnvelope. Signature was
// already verified before queueing; slot, builder and payload hash checks are
// performed inside ReceiveExecutionPayloadEnvelope → ProcessExecutionPayload.
func (s *Service) processPendingPayloadEnvelope(ctx context.Context, root [32]byte) {
	s.pendingEnvelopeLock.Lock()
	signedEnvelope, ok := s.pendingPayloadEnvelopes[root]
	if !ok {
		s.pendingEnvelopeLock.Unlock()
		return
	}
	delete(s.pendingPayloadEnvelopes, root)
	s.pendingEnvelopeLock.Unlock()

	e, err := blocks.WrappedROSignedExecutionPayloadEnvelope(signedEnvelope)
	if err != nil {
		log.WithError(err).Debug("Could not wrap pending signed execution payload envelope")
		return
	}
	env, err := e.Envelope()
	if err != nil {
		log.WithError(err).Debug("Could not get pending execution payload envelope")
		return
	}
	if s.hasSeenPayloadEnvelope(root, env.BuilderIndex()) {
		return
	}
	if s.hasBadBlock(root) {
		s.setSeenPayloadEnvelope(root, env.BuilderIndex())
		return
	}
	s.setSeenPayloadEnvelope(root, env.BuilderIndex())

	if err := s.cfg.chain.ReceiveExecutionPayloadEnvelope(ctx, e); err != nil {
		log.WithError(err).Debug("Could not process pending payload envelope")
	}
}

// processPendingPayloadEnvelopes iterates the pending envelope map and
// processes any entry whose beacon block is now in forkchoice.
func (s *Service) processPendingPayloadEnvelopes(ctx context.Context) {
	s.pendingEnvelopeLock.RLock()
	roots := make([][32]byte, 0, len(s.pendingPayloadEnvelopes))
	for root := range s.pendingPayloadEnvelopes {
		roots = append(roots, root)
	}
	s.pendingEnvelopeLock.RUnlock()

	for _, root := range roots {
		if !s.cfg.chain.InForkchoice(root) {
			continue
		}
		s.processPendingPayloadEnvelope(ctx, root)
	}
}

// prunePendingPayloadEnvelopes removes entries whose slot is at or below the
// finalized epoch start slot, following the same pattern as validatePendingSlots.
func (s *Service) prunePendingPayloadEnvelopes() {
	s.pendingEnvelopeLock.Lock()
	defer s.pendingEnvelopeLock.Unlock()

	finalizedEpoch := s.cfg.chain.FinalizedCheckpt().Epoch
	for root, env := range s.pendingPayloadEnvelopes {
		if slots.ToEpoch(env.Message.Slot) < finalizedEpoch {
			delete(s.pendingPayloadEnvelopes, root)
		}
	}
}
