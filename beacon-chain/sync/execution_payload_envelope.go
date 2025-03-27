package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/crypto/rand"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	v1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

func (s *Service) validateExecutionPayloadEnvelope(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	if pid == s.cfg.p2p.PeerID() {
		log.Info("Received local execution payload envelope")
		return pubsub.ValidationAccept, nil
	}
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}
	ctx, span := trace.StartSpan(ctx, "sync.validateExecutionPayloadEnvelope")
	defer span.End()
	if msg.Topic == nil {
		return pubsub.ValidationReject, errInvalidTopic
	}
	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}
	signedEnvelope, ok := m.(*v1.SignedExecutionPayloadEnvelope)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}
	e, err := blocks.WrappedROSignedExecutionPayloadEnvelope(signedEnvelope)
	if err != nil {
		log.WithError(err).Error("Failed to create read only signed payload execution envelope")
		return pubsub.ValidationIgnore, err
	}
	v := s.newExecutionPayloadEnvelopeVerifier(e, verification.GossipExecutionPayloadEnvelopeRequirements)

	if err := v.VerifyBlockRootSeen(s.seenBlockRoot); err != nil {
		blockRoot := signedEnvelope.Message.BeaconBlockRoot
		log.WithFields(logrus.Fields{
			"blockRoot": fmt.Sprintf("%#x", blockRoot),
			"blockHash": fmt.Sprintf("%#x", signedEnvelope.Message.Payload.BlockHash),
			"slot":      signedEnvelope.Message.Slot,
		}).Debug("Inserting pending execution payload")
		s.pendingExecutionPayloads.Add(signedEnvelope)
		go func() {
			if err := s.sendBatchRootRequest(context.Background(), [][32]byte{[32]byte(blockRoot)}, rand.NewGenerator()); err != nil {
				log.WithError(err).Error("Failed to send batch root request")
			}
		}()
		return pubsub.ValidationIgnore, err
	}
	res, err := s.validateAfterBlockRootSeen(ctx, signedEnvelope, v)
	if err != nil {
		return res, err
	}
	msg.ValidatorData = signedEnvelope

	log.WithFields(logrus.Fields{
		"blockRoot": fmt.Sprintf("%#x", signedEnvelope.Message.BeaconBlockRoot),
		"slot":      signedEnvelope.Message.Slot,
	}).Debug("Received execution payload envelope")
	return pubsub.ValidationAccept, nil
}

func (s *Service) validateAfterBlockRootSeen(ctx context.Context, signedEnvelope *v1.SignedExecutionPayloadEnvelope, v verification.ExecutionPayloadEnvelopeVerifier) (pubsub.ValidationResult, error) {
	root := [32]byte(signedEnvelope.Message.BeaconBlockRoot)
	_, seen := s.payloadEnvelopeCache.Load(root)
	if seen {
		return pubsub.ValidationIgnore, nil
	}
	if err := v.VerifyBlockRootValid(s.hasBadBlock); err != nil {
		return pubsub.ValidationReject, err
	}
	signedHeader, err := s.cfg.beaconDB.SignedExecutionPayloadHeader(ctx, root)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	res, err := verifyAgainstHeader(v, signedHeader)
	if err != nil {
		return res, err
	}
	st, err := s.cfg.stateGen.StateByRoot(ctx, root)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	if err := v.VerifySignature(st); err != nil {
		return pubsub.ValidationReject, err
	}
	s.payloadEnvelopeCache.Store(root, struct{}{})
	return pubsub.ValidationAccept, nil
}

func verifyAgainstHeader(v verification.ExecutionPayloadEnvelopeVerifier, signed interfaces.ROSignedExecutionPayloadHeader) (pubsub.ValidationResult, error) {
	header, err := signed.Header()
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	if err := v.VerifySlot(header); err != nil {
		return pubsub.ValidationReject, err
	}
	if err := v.VerifyBuilderValid(header); err != nil {
		return pubsub.ValidationReject, err
	}
	if err := v.VerifyPayloadHash(header); err != nil {
		return pubsub.ValidationReject, err
	}
	return pubsub.ValidationAccept, nil
}

func (s *Service) executionPayloadEnvelopeSubscriber(ctx context.Context, msg proto.Message) error {
	e, ok := msg.(*v1.SignedExecutionPayloadEnvelope)
	if !ok {
		return errWrongMessage
	}
	env, err := blocks.WrappedROSignedExecutionPayloadEnvelope(e)
	if err != nil {
		return err
	}
	return s.cfg.chain.ReceiveExecutionPayloadEnvelope(ctx, env, nil)
}

func (s *Service) latePayloadTasks(ctx context.Context) {
	slot, root := s.cfg.chain.HighestReceivedBlockSlotRoot()
	if slots.ToEpoch(slot) < params.BeaconConfig().EPBSForkEpoch {
		return
	}
	if slot < s.cfg.clock.CurrentSlot() {
		return
	}
	hash, err := s.cfg.chain.HashForBlockRoot(ctx, root)
	if err != nil {
		log.WithError(err).Error("Failed to get hash for block root")
		return
	}
	if s.cfg.chain.PayloadBeingSynced(root) || s.cfg.chain.HashInForkchoice([32]byte(hash)) {
		return
	}
	log.WithFields(logrus.Fields{"blockRoot": fmt.Sprintf("%#x", root), "slot": slot}).Debug("Requesting late payload")
	go func() {
		if err := s.sendBatchPayloadRequest(context.Background(), [][32]byte{[32]byte(root)}, rand.NewGenerator()); err != nil {
			log.WithError(err).Error("Failed to send batch payload request")
		}
	}()
}
