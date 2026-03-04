package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/rand"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func (s *Service) validateExecutionPayloadEnvelope(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	ctx, span := trace.StartSpan(ctx, "sync.validateExecutionPayloadEnvelope")
	defer span.End()

	if msg.Topic == nil {
		return pubsub.ValidationReject, p2p.ErrInvalidTopic
	}

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}

	signedEnvelope, ok := m.(*ethpb.SignedExecutionPayloadEnvelope)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}
	e, err := blocks.WrappedROSignedExecutionPayloadEnvelope(signedEnvelope)
	if err != nil {
		log.WithError(err).Error("failed to create read only signed payload execution envelope")
		return pubsub.ValidationIgnore, err
	}
	v := s.newExecutionPayloadEnvelopeVerifier(e, verification.GossipExecutionPayloadEnvelopeRequirements)

	env, err := e.Envelope()
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	// [IGNORE] The envelope's block root envelope.block_root has been seen (via gossip or non-gossip sources)
	// (a client MAY queue payload for processing once the block is retrieved).
	if err := v.VerifyBlockRootSeen(func(root [32]byte) bool { return s.cfg.chain.HasBlock(ctx, root) }); err != nil {
		return s.queuePendingPayloadEnvelope(ctx, v, env, signedEnvelope)
	}
	root := env.BeaconBlockRoot()
	// [IGNORE] The node has not seen another valid SignedExecutionPayloadEnvelope for this block root from this builder.
	if s.hasSeenPayloadEnvelope(root, env.BuilderIndex()) {
		return pubsub.ValidationIgnore, nil
	}
	finalized := s.cfg.chain.FinalizedCheckpt()
	if finalized == nil {
		return pubsub.ValidationIgnore, errors.New("nil finalized checkpoint")
	}
	// [IGNORE] The envelope is from a slot greater than or equal to the latest finalized slot --
	// i.e. validate that envelope.slot >= compute_start_slot_at_epoch(store.finalized_checkpoint.epoch).
	if err := v.VerifySlotAboveFinalized(finalized.Epoch); err != nil {
		return pubsub.ValidationIgnore, err
	}
	// [REJECT] block passes validation.
	if err := v.VerifyBlockRootValid(s.hasBadBlock); err != nil {
		return pubsub.ValidationReject, err
	}

	// Let block be the block with envelope.beacon_block_root.
	block, err := s.cfg.beaconDB.Block(ctx, root)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	// [REJECT] block.slot equals envelope.slot.
	if err := v.VerifySlotMatchesBlock(block.Block().Slot()); err != nil {
		return pubsub.ValidationReject, err
	}

	// Let bid alias block.body.signed_execution_payload_bid.message
	// (notice that this can be obtained from the state.latest_execution_payload_bid).
	signedBid, err := block.Block().Body().SignedExecutionPayloadBid()
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	wrappedBid, err := blocks.WrappedROSignedExecutionPayloadBid(signedBid)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	bid, err := wrappedBid.Bid()
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	// [REJECT] envelope.builder_index == bid.builder_index.
	if err := v.VerifyBuilderValid(bid); err != nil {
		return pubsub.ValidationReject, err
	}
	// [REJECT] payload.block_hash == bid.block_hash.
	if err := v.VerifyPayloadHash(bid); err != nil {
		return pubsub.ValidationReject, err
	}

	// For self-build, the state is retrived via how we retrieve for beacon block optimization
	// For builder index, the state is retrived via head state read only
	st, err := s.blockVerifyingState(ctx, block)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	// [REJECT] signed_execution_payload_envelope.signature is valid with respect to the builder's public key.
	if err := v.VerifySignature(st); err != nil {
		return pubsub.ValidationReject, err
	}
	s.setSeenPayloadEnvelope(root, env.BuilderIndex())
	return pubsub.ValidationAccept, nil
}

// queuePendingPayloadEnvelope verifies the builder signature and queues the
// envelope for processing once the corresponding block arrives.
func (s *Service) queuePendingPayloadEnvelope(
	ctx context.Context,
	v verification.ExecutionPayloadEnvelopeVerifier,
	env interfaces.ROExecutionPayloadEnvelope,
	signedEnvelope *ethpb.SignedExecutionPayloadEnvelope,
) (pubsub.ValidationResult, error) {
	currentSlot := s.cfg.clock.CurrentSlot()
	if env.Slot() != currentSlot {
		return pubsub.ValidationIgnore, nil
	}
	st, err := s.cfg.chain.HeadStateReadOnly(ctx)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	currentEpoch := slots.ToEpoch(currentSlot)
	stateEpoch := slots.ToEpoch(st.Slot())
	proposerInLookahead := (stateEpoch == currentEpoch || stateEpoch+1 == currentEpoch)
	builderIdx := uint64(env.BuilderIndex())
	isSelfBuild := builderIdx == uint64(params.BeaconConfig().BuilderIndexSelfBuild)
	if !isSelfBuild || proposerInLookahead {
		if err := v.VerifySignature(st); err != nil {
			return pubsub.ValidationReject, err
		}
	} else {
		log.Debug("Ignoring payload envelope from self-build outside of the Lookahead window")
		return pubsub.ValidationIgnore, nil
	}
	root := env.BeaconBlockRoot()
	s.pendingEnvelopeLock.Lock()
	inner, rootExists := s.pendingPayloadEnvelopes[root]
	if !rootExists {
		if !isSelfBuild && len(s.pendingPayloadEnvelopes) >= maxPendingPayloadRoots {
			s.pendingEnvelopeLock.Unlock()
			return pubsub.ValidationIgnore, nil
		}
		inner = make(map[uint64]*ethpb.SignedExecutionPayloadEnvelope)
		s.pendingPayloadEnvelopes[root] = inner
	} else {
		for _, existing := range inner {
			if existing.Message.Slot != signedEnvelope.Message.Slot {
				s.pendingEnvelopeLock.Unlock()
				return pubsub.ValidationIgnore, nil
			}
			break
		}
	}
	if _, exists := inner[builderIdx]; exists {
		s.pendingEnvelopeLock.Unlock()
		return pubsub.ValidationIgnore, nil
	}
	if !isSelfBuild && len(inner) >= maxPendingBuildersPerRoot {
		s.pendingEnvelopeLock.Unlock()
		return pubsub.ValidationIgnore, nil
	}
	inner[builderIdx] = signedEnvelope
	s.pendingEnvelopeLock.Unlock()

	if !rootExists {
		go func() {
			if err := s.sendBatchRootRequest(s.ctx, [][32]byte{root}, rand.NewGenerator()); err != nil {
				log.WithError(err).Debug("Could not request beacon block for pending payload envelope")
			}
		}()
	}
	return pubsub.ValidationIgnore, nil
}

func (s *Service) executionPayloadEnvelopeSubscriber(ctx context.Context, msg proto.Message) error {
	e, ok := msg.(*ethpb.SignedExecutionPayloadEnvelope)
	if !ok {
		return errWrongMessage
	}
	env, err := blocks.WrappedROSignedExecutionPayloadEnvelope(e)
	if err != nil {
		return errors.Wrap(err, "could not wrap signed execution payload envelope")
	}
	if err := s.cfg.chain.ReceiveExecutionPayloadEnvelope(ctx, env); err != nil {
		if blockchain.IsInvalidBlock(err) {
			envelope, envErr := env.Envelope()
			if envErr == nil {
				s.setBadPayload(ctx, envelope.BeaconBlockRoot())
			} else {
				log.WithError(envErr).Error("failed to get envelope from signed execution payload envelope")
			}
		}
		return err
	}
	return nil
}

func (s *Service) hasSeenPayloadEnvelope(root [32]byte, builderIdx primitives.BuilderIndex) bool {
	if s.seenPayloadEnvelopeCache == nil {
		return false
	}

	b := append(bytesutil.Bytes32(uint64(builderIdx)), root[:]...)
	_, seen := s.seenPayloadEnvelopeCache.Get(string(b))
	return seen
}

func (s *Service) setSeenPayloadEnvelope(root [32]byte, builderIdx primitives.BuilderIndex) {
	if s.seenPayloadEnvelopeCache == nil {
		return
	}

	b := append(bytesutil.Bytes32(uint64(builderIdx)), root[:]...)
	s.seenPayloadEnvelopeCache.Add(string(b), true)
}
