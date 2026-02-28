package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/async"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
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
// given block root, runs the full set of gossip validation checks, and if
// valid calls ReceiveExecutionPayloadEnvelope. The caller must pass the
// already-known block so we avoid a redundant DB lookup.
func (s *Service) processPendingPayloadEnvelope(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, root [32]byte) {
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
	v := s.newExecutionPayloadEnvelopeVerifier(e, verification.GossipExecutionPayloadEnvelopeRequirements)
	// Signature was verified before queueing, block root is seen because the block just arrived.
	v.SatisfyRequirement(verification.RequireBuilderSignatureValid)
	v.SatisfyRequirement(verification.RequireBlockRootSeen)

	if s.hasSeenPayloadEnvelope(root, env.BuilderIndex()) {
		return
	}
	finalized := s.cfg.chain.FinalizedCheckpt()
	if finalized == nil {
		return
	}
	if err := v.VerifySlotAboveFinalized(finalized.Epoch); err != nil {
		log.WithError(err).Debug("Pending payload envelope failed slot above finalized check")
		return
	}
	if err := v.VerifyBlockRootValid(s.hasBadBlock); err != nil {
		log.WithError(err).Debug("Pending payload envelope has bad block root")
		return
	}
	if err := v.VerifySlotMatchesBlock(block.Block().Slot()); err != nil {
		log.WithError(err).Debug("Pending payload envelope slot does not match block")
		return
	}
	signedBid, err := block.Block().Body().SignedExecutionPayloadBid()
	if err != nil {
		log.WithError(err).Debug("Could not get signed bid from block for pending payload envelope")
		return
	}
	wrappedBid, err := blocks.WrappedROSignedExecutionPayloadBid(signedBid)
	if err != nil {
		log.WithError(err).Debug("Could not wrap signed bid for pending payload envelope")
		return
	}
	bid, err := wrappedBid.Bid()
	if err != nil {
		log.WithError(err).Debug("Could not get bid for pending payload envelope")
		return
	}
	if err := v.VerifyBuilderValid(bid); err != nil {
		log.WithError(err).Debug("Pending payload envelope has invalid builder")
		return
	}
	if err := v.VerifyPayloadHash(bid); err != nil {
		log.WithError(err).Debug("Pending payload envelope has invalid payload hash")
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
		block, err := s.cfg.beaconDB.Block(ctx, root)
		if err != nil {
			log.WithError(err).Debug("Could not retrieve block for pending payload envelope")
			continue
		}
		s.processPendingPayloadEnvelope(ctx, block, root)
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
