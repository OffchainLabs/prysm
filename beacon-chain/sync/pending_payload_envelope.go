package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

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

	if s.hasSeenPayloadEnvelope(root, env.BuilderIndex()) {
		return
	}
	finalized := s.cfg.chain.FinalizedCheckpt()
	if finalized == nil {
		return
	}
	if err := v.VerifySlotAboveFinalized(finalized.Epoch); err != nil {
		return
	}
	if err := v.VerifyBlockRootValid(s.hasBadBlock); err != nil {
		return
	}
	if err := v.VerifySlotMatchesBlock(block.Block().Slot()); err != nil {
		return
	}
	signedBid, err := block.Block().Body().SignedExecutionPayloadBid()
	if err != nil {
		return
	}
	wrappedBid, err := blocks.WrappedROSignedExecutionPayloadBid(signedBid)
	if err != nil {
		return
	}
	bid, err := wrappedBid.Bid()
	if err != nil {
		return
	}
	if err := v.VerifyBuilderValid(bid); err != nil {
		return
	}
	if err := v.VerifyPayloadHash(bid); err != nil {
		return
	}
	st, err := s.blockVerifyingState(ctx, block)
	if err != nil {
		log.WithError(err).Debug("Could not get state for pending payload envelope signature verification")
		return
	}
	if err := v.VerifySignature(st); err != nil {
		return
	}
	s.setSeenPayloadEnvelope(root, env.BuilderIndex())

	if err := s.cfg.chain.ReceiveExecutionPayloadEnvelope(ctx, e); err != nil {
		log.WithError(err).Debug("Could not process pending payload envelope")
	}
}

// prunePendingPayloadEnvelopes removes entries whose slot is at or below the
// finalized epoch start slot, following the same pattern as validatePendingSlots.
func (s *Service) prunePendingPayloadEnvelopes() {
	s.pendingEnvelopeLock.Lock()
	defer s.pendingEnvelopeLock.Unlock()

	finalizedEpoch := s.cfg.chain.FinalizedCheckpt().Epoch
	for root, env := range s.pendingPayloadEnvelopes {
		if slots.ToEpoch(env.Message.Slot) <= finalizedEpoch {
			delete(s.pendingPayloadEnvelopes, root)
		}
	}
}
