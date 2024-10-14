package sync

import (
	"context"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	v1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
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
		log.WithError(err).Error("failed to create read only signed payload execution envelope")
		return pubsub.ValidationIgnore, err
	}
	v := s.newExecutionPayloadEnvelopeVerifier(e, verification.GossipExecutionPayloadEnvelopeRequirements)

	if err := v.VerifyBlockRootSeen(s.seenBlockRoot); err != nil {
		return pubsub.ValidationIgnore, err
	}
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
	if err := v.SetSlot(header.Slot()); err != nil {
		return pubsub.ValidationIgnore, err
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
	env, err := blocks.WrappedROExecutionPayloadEnvelope(e.Message)
	if err != nil {
		return err
	}
	return s.cfg.chain.ReceiveExecutionPayloadEnvelope(ctx, env, nil)
}
