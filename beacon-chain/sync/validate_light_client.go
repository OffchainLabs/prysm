package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

func (s *Service) validateLightClientOptimisticUpdate(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	// Validation runs on publish (not just subscriptions), so we should approve any message from
	// ourselves.
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	// Ignore updates while syncing
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	_, span := trace.StartSpan(ctx, "sync.validateLightClientOptimisticUpdate")
	defer span.End()

	currentUpdate := s.lcStore.LastOptimisticUpdate()
	if currentUpdate == nil {
		log.Debug("No existing optimistic update to compare against. Ignoring.")
		return pubsub.ValidationIgnore, nil
	}

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}

	newUpdate, ok := m.(interfaces.LightClientOptimisticUpdate)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}

	attestedHeaderRoot, err := newUpdate.AttestedHeader().Beacon().HashTreeRoot()
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	// validate that enough time has passed since the start of the slot so the block has had enough time to propagate
	slotStart, err := slots.StartTime(s.cfg.clock.GenesisTime(), newUpdate.SignatureSlot())
	if err != nil {
		log.WithError(err).Debug("Peer sent a slot that would overflow slot start time")
		return pubsub.ValidationReject, nil
	}
	earliestValidTime := slotStart.
		Add(slots.ComponentDuration(primitives.BP(params.BeaconConfig().SyncMessageDueBPS))).
		Add(-params.BeaconConfig().MaximumGossipClockDisparityDuration())
	if s.cfg.clock.Now().Before(earliestValidTime) {
		log.Debug("Newly received light client optimistic update ignored. not enough time passed for block to propagate")
		return pubsub.ValidationIgnore, nil
	}

	if !proto.Equal(newUpdate.Proto(), currentUpdate.Proto()) {
		log.WithFields(logrus.Fields{
			"attestedSlot":       fmt.Sprintf("%d", newUpdate.AttestedHeader().Beacon().Slot),
			"signatureSlot":      fmt.Sprintf("%d", newUpdate.SignatureSlot()),
			"attestedHeaderRoot": fmt.Sprintf("%x", attestedHeaderRoot),
		}).Debug("Received light client optimistic update is different from the local one. Ignoring.")
		return pubsub.ValidationIgnore, nil
	}

	log.WithFields(logrus.Fields{
		"attestedSlot":       fmt.Sprintf("%d", newUpdate.AttestedHeader().Beacon().Slot),
		"signatureSlot":      fmt.Sprintf("%d", newUpdate.SignatureSlot()),
		"attestedHeaderRoot": fmt.Sprintf("%x", attestedHeaderRoot),
	}).Debug("New gossiped light client optimistic update validated.")

	msg.ValidatorData = newUpdate.Proto()
	return pubsub.ValidationAccept, nil
}

func (s *Service) validateLightClientFinalityUpdate(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	// Validation runs on publish (not just subscriptions), so we should approve any message from
	// ourselves.
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	// Ignore updates while syncing
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	_, span := trace.StartSpan(ctx, "sync.validateLightClientFinalityUpdate")
	defer span.End()

	currentUpdate := s.lcStore.LastFinalityUpdate()
	if currentUpdate == nil {
		log.Debug("No existing finality update to compare against. Ignoring.")
		return pubsub.ValidationIgnore, nil
	}

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}

	newUpdate, ok := m.(interfaces.LightClientFinalityUpdate)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}

	attestedHeaderRoot, err := newUpdate.AttestedHeader().Beacon().HashTreeRoot()
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	// validate that enough time has passed since the start of the slot so the block has had enough time to propagate
	slotStart, err := slots.StartTime(s.cfg.clock.GenesisTime(), newUpdate.SignatureSlot())
	if err != nil {
		log.WithError(err).Debug("Peer sent a slot that would overflow slot start time")
		return pubsub.ValidationReject, nil
	}
	earliestValidTime := slotStart.
		Add(slots.ComponentDuration(primitives.BP(params.BeaconConfig().SyncMessageDueBPS))).
		Add(-params.BeaconConfig().MaximumGossipClockDisparityDuration())
	if s.cfg.clock.Now().Before(earliestValidTime) {
		log.Debug("Newly received light client finality update ignored. not enough time passed for block to propagate")
		return pubsub.ValidationIgnore, nil
	}

	if !proto.Equal(newUpdate.Proto(), currentUpdate.Proto()) {
		log.WithFields(logrus.Fields{
			"attestedSlot":       fmt.Sprintf("%d", newUpdate.AttestedHeader().Beacon().Slot),
			"signatureSlot":      fmt.Sprintf("%d", newUpdate.SignatureSlot()),
			"attestedHeaderRoot": fmt.Sprintf("%x", attestedHeaderRoot),
		}).Debug("Received light client finality update is different from the local one. ignoring.")
		return pubsub.ValidationIgnore, nil
	}

	log.WithFields(logrus.Fields{
		"attestedSlot":       fmt.Sprintf("%d", newUpdate.AttestedHeader().Beacon().Slot),
		"signatureSlot":      fmt.Sprintf("%d", newUpdate.SignatureSlot()),
		"attestedHeaderRoot": fmt.Sprintf("%x", attestedHeaderRoot),
	}).Debug("New gossiped light client finality update validated.")

	msg.ValidatorData = newUpdate.Proto()
	return pubsub.ValidationAccept, nil
}
