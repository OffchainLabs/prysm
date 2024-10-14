package sync

import (
	"context"
	"fmt"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	v1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"google.golang.org/protobuf/proto"
)

func (s *Service) validateExecutionPayloadHeader(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	ctx, span := trace.StartSpan(ctx, "sync.validateExecutionPayloadHeader")
	defer span.End()

	if msg.Topic == nil {
		return pubsub.ValidationReject, errInvalidTopic
	}

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}

	signedHeader, ok := m.(*v1.SignedExecutionPayloadHeader)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}
	shm := signedHeader.Message
	slot := shm.Slot
	builderIndex := shm.BuilderIndex

	if seenBuilderBySlot(slot, builderIndex) {
		return pubsub.ValidationIgnore, fmt.Errorf("builder %d has already been seen in slot %d", builderIndex, slot)
	}

	highestValueHeader := s.executionPayloadHeaderCache.SignedExecutionPayloadHeader(slot, shm.ParentBlockHash, shm.ParentBlockRoot)
	if highestValueHeader != nil && highestValueHeader.Message.Value >= shm.Value {
		return pubsub.ValidationIgnore, fmt.Errorf("received header has lower value than cached header")
	}

	h, err := blocks.WrappedROSignedExecutionPayloadHeader(signedHeader)
	if err != nil {
		log.WithError(err).Error("failed to create read only signed execution payload header")
		return pubsub.ValidationIgnore, err
	}

	roState, err := s.cfg.chain.HeadStateReadOnly(ctx)
	if err != nil {
		log.WithError(err).Error("failed to get head state to validate execution payload header")
		return pubsub.ValidationIgnore, err
	}
	v := s.newExecutionPayloadHeaderVerifier(h, roState, verification.GossipExecutionPayloadHeaderRequirements)

	if err := v.VerifyCurrentOrNextSlot(); err != nil {
		return pubsub.ValidationIgnore, err
	}

	if err := v.VerifyParentBlockRootSeen(s.seenBlockRoot); err != nil {
		return pubsub.ValidationIgnore, err
	}

	if err := v.VerifyParentBlockHashSeen(s.seenBlockHash); err != nil {
		return pubsub.ValidationIgnore, err
	}

	if err := v.VerifySignature(); err != nil {
		return pubsub.ValidationReject, err
	}
	addBuilderBySlot(slot, builderIndex)

	if err := v.VerifyBuilderActiveNotSlashed(); err != nil {
		return pubsub.ValidationReject, err
	}

	if err := v.VerifyBuilderSufficientBalance(); err != nil {
		return pubsub.ValidationReject, err
	}

	return pubsub.ValidationAccept, nil
}

func (s *Service) subscribeExecutionPayloadHeader(ctx context.Context, msg proto.Message) error {
	e, ok := msg.(*v1.SignedExecutionPayloadHeader)
	if !ok {
		return errWrongMessage
	}

	s.executionPayloadHeaderCache.SaveSignedExecutionPayloadHeader(e)

	return nil
}

var (
	// builderBySlot is a map of slots to a set of builder that have been seen in that slot.
	builderBySlot     = make(map[primitives.Slot]map[primitives.ValidatorIndex]struct{})
	builderBySlotLock sync.RWMutex
)

func addBuilderBySlot(slot primitives.Slot, index primitives.ValidatorIndex) {
	builderBySlotLock.Lock()
	defer builderBySlotLock.Unlock()

	// Remove old slots: p2p allows current and next slot, so we allow two slots to be seen
	for k := range builderBySlot {
		if k+1 < slot {
			delete(builderBySlot, k)
		}
	}

	if _, ok := builderBySlot[slot]; !ok {
		builderBySlot[slot] = make(map[primitives.ValidatorIndex]struct{})
	}

	builderBySlot[slot][index] = struct{}{}
}

func seenBuilderBySlot(slot primitives.Slot, index primitives.ValidatorIndex) bool {
	builderBySlotLock.RLock()
	defer builderBySlotLock.RUnlock()

	if _, ok := builderBySlot[slot]; !ok {
		return false
	}

	_, ok := builderBySlot[slot][index]
	return ok
}
