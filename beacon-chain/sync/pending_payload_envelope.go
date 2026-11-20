package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/async"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

const (
	maxPendingPayloadRoots    = 128
	maxPendingBuildersPerRoot = 2
	maxSelfBuildSigFailures   = 3
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

// processPendingPayloadEnvelope retrieves all queued payload envelopes for the
// given block root and calls ReceiveExecutionPayloadEnvelope on each.
func (s *Service) processPendingPayloadEnvelope(ctx context.Context, root [32]byte) {
	s.pendingEnvelopeLock.Lock()
	inner, ok := s.pendingPayloadEnvelopes[root]
	if !ok {
		s.pendingEnvelopeLock.Unlock()
		return
	}
	delete(s.pendingPayloadEnvelopes, root)
	s.pendingEnvelopeLock.Unlock()

	for _, e := range inner {
		env, err := e.Envelope()
		if err != nil {
			log.WithError(err).Debug("Could not get pending execution payload envelope")
			continue
		}
		if s.hasSeenPayloadEnvelope(root, env.BuilderIndex()) {
			continue
		}
		if s.hasBadBlock(root) {
			s.setSeenPayloadEnvelope(root, env.BuilderIndex())
			continue
		}

		receiveCtx, cancel := context.WithTimeout(ctx, params.BeaconConfig().SlotDuration())
		err = s.cfg.chain.ReceiveExecutionPayloadEnvelope(receiveCtx, e)
		cancel()
		if err != nil {
			log.WithError(err).Debug("Could not process pending payload envelope")
			continue
		}
		s.setSeenPayloadEnvelope(root, env.BuilderIndex())
		if err := s.cfg.p2p.Broadcast(ctx, e.Proto()); err != nil {
			log.WithError(err).Warn("Could not broadcast pending payload envelope")
		}
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
	for root, inner := range s.pendingPayloadEnvelopes {
		for _, signed := range inner {
			if env, err := signed.Envelope(); err == nil && slots.ToEpoch(env.Slot()) < finalizedEpoch {
				delete(s.pendingPayloadEnvelopes, root)
			}
			break // only need one envelope per root; admission enforces current-slot
		}
	}
}
